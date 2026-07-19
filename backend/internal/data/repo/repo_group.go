package repo

import (
	"context"
	stdsql "database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entity"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entitytemplate"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/group"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/groupinvitationtoken"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/notifier"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/user"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/usergroup"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
)

var (
	ErrInvitationExpired   = errors.New("invitation expired")
	ErrInvitationExhausted = errors.New("invitation used up")
	ErrAlreadyGroupMember  = errors.New("user already a member of this group")
)

type GroupRepository struct {
	db               *ent.Client
	groupMapper      MapFunc[*ent.Group, Group]
	invitationMapper MapFunc[*ent.GroupInvitationToken, GroupInvitation]
	attachments      *AttachmentRepo
}

func NewGroupRepository(db *ent.Client) *GroupRepository {
	gmap := func(g *ent.Group) Group {
		return Group{
			ID:        g.ID,
			Name:      g.Name,
			CreatedAt: g.CreatedAt,
			UpdatedAt: g.UpdatedAt,
			Currency:  strings.ToUpper(g.Currency),
		}
	}

	imap := func(i *ent.GroupInvitationToken) GroupInvitation {
		return GroupInvitation{
			ID:        i.ID,
			ExpiresAt: i.ExpiresAt,
			Uses:      i.Uses,
			Group:     gmap(i.Edges.Group),
		}
	}

	return &GroupRepository{
		db:               db,
		groupMapper:      gmap,
		invitationMapper: imap,
	}
}

type (
	Group struct {
		ID        uuid.UUID `json:"id,omitempty"`
		Name      string    `json:"name,omitempty"`
		CreatedAt time.Time `json:"createdAt,omitempty"`
		UpdatedAt time.Time `json:"updatedAt,omitempty"`
		Currency  string    `json:"currency,omitempty"`
	}

	GroupUpdate struct {
		Name     string `json:"name"     validate:"required,min=1,max=255"`
		Currency string `json:"currency" validate:"required,min=1,max=16"`
	}

	GroupInvitationCreate struct {
		Token     []byte    `json:"-"`
		ExpiresAt time.Time `json:"expiresAt"`
		Uses      int       `json:"uses"`
	}

	GroupInvitation struct {
		ID        uuid.UUID `json:"id"`
		ExpiresAt time.Time `json:"expiresAt"`
		Uses      int       `json:"uses"`
		Group     Group     `json:"group"`
	}

	GroupStatistics struct {
		TotalUsers        int     `json:"totalUsers"`
		TotalItems        int     `json:"totalItems"`
		TotalLocations    int     `json:"totalLocations"`
		TotalTags         int     `json:"totalTags"`
		TotalItemPrice    float64 `json:"totalItemPrice"`
		TotalWithWarranty int     `json:"totalWithWarranty"`
	}

	ValueOverTimeEntry struct {
		Date  time.Time `json:"date"`
		Value float64   `json:"value"`
		Name  string    `json:"name"`
	}

	ValueOverTime struct {
		PriceAtStart float64              `json:"valueAtStart"`
		PriceAtEnd   float64              `json:"valueAtEnd"`
		Start        time.Time            `json:"start"`
		End          time.Time            `json:"end"`
		Entries      []ValueOverTimeEntry `json:"entries"`
	}

	TotalsByOrganizer struct {
		ID    uuid.UUID `json:"id"`
		Name  string    `json:"name"`
		Total float64   `json:"total"`
	}
)

func (r *GroupRepository) GetAllGroups(ctx context.Context, userID uuid.UUID) ([]Group, error) {
	q := r.db.Group.Query()
	if userID != uuid.Nil {
		q.Where(group.HasUsersWith(user.ID(userID)))
	}
	return r.groupMapper.MapEachErr(q.All(ctx))
}

func (r *GroupRepository) StatsLocationsByPurchasePrice(ctx context.Context, gid uuid.UUID) ([]TotalsByOrganizer, error) {
	var v []TotalsByOrganizer

	// Attribute every qualifying descendant item to each location in its
	// ancestor chain. DISTINCT makes legacy cycles non-inflating and the depth
	// guard keeps the recursive walk bounded.
	q := `
		WITH RECURSIVE location_descendants(root_id, entity_id, depth) AS (
			SELECT root.id, child.id, 1
			FROM entities root
			JOIN entity_types root_type ON root_type.id = root.entity_type_entities
			JOIN entities child ON child.entity_children = root.id
			WHERE root.group_entities = $1
			  AND root_type.group_entity_types = $1
			  AND root_type.is_location = true
			  AND child.group_entities = $1

			UNION ALL

			SELECT descendants.root_id, child.id, descendants.depth + 1
			FROM location_descendants descendants
			JOIN entities child ON child.entity_children = descendants.entity_id
			WHERE child.group_entities = $1
			  AND descendants.depth < $2
		),
		unique_descendants AS (
			SELECT DISTINCT root_id, entity_id
			FROM location_descendants
		)
		SELECT root.id, root.name,
			COALESCE(SUM(item.purchase_price * item.quantity), 0) AS total
		FROM entities root
		JOIN entity_types root_type ON root_type.id = root.entity_type_entities
		JOIN unique_descendants descendants ON descendants.root_id = root.id
		JOIN entities item ON item.id = descendants.entity_id
		JOIN entity_types item_type ON item_type.id = item.entity_type_entities
		WHERE root.group_entities = $1
		  AND root_type.group_entity_types = $1
		  AND root_type.is_location = true
		  AND item.group_entities = $1
		  AND item_type.group_entity_types = $1
		  AND item_type.is_location = false
		  AND item.archived = false
		  AND item.sold_date IS NULL
		GROUP BY root.id, root.name
		HAVING COALESCE(SUM(item.purchase_price * item.quantity), 0) <> 0
		ORDER BY lower(root.name), root.id
	`

	rows, err := r.db.Sql().QueryContext(ctx, q, gid, maxHierarchyDepth)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var item TotalsByOrganizer
		if err := rows.Scan(&item.ID, &item.Name, &item.Total); err != nil {
			return nil, err
		}
		v = append(v, item)
	}

	return v, rows.Err()
}

func (r *GroupRepository) StatsTagsByPurchasePrice(ctx context.Context, gid uuid.UUID) ([]TotalsByOrganizer, error) {
	var v []TotalsByOrganizer

	q := `
		SELECT t.id, t.name,
			COALESCE(SUM(e.purchase_price * e.quantity), 0) AS total
		FROM tags t
		JOIN tag_entities te ON te.tag_id = t.id
		JOIN entities e ON e.id = te.entity_id
		JOIN entity_types et ON et.id = e.entity_type_entities
		WHERE t.group_tags = $1
		  AND e.group_entities = $1
		  AND et.group_entity_types = $1
		  AND et.is_location = false
		  AND e.archived = false
		  AND e.sold_date IS NULL
		GROUP BY t.id, t.name
		HAVING COALESCE(SUM(e.purchase_price * e.quantity), 0) <> 0
		ORDER BY lower(t.name), t.id`

	rows, err := r.db.Sql().QueryContext(ctx, q, gid)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var item TotalsByOrganizer
		if err := rows.Scan(&item.ID, &item.Name, &item.Total); err != nil {
			return nil, err
		}
		v = append(v, item)
	}
	return v, rows.Err()
}

func (r *GroupRepository) StatsPurchasePrice(ctx context.Context, gid uuid.UUID, start, end time.Time) (*ValueOverTime, error) {
	// Get the Totals for the Start and End of the Given Time Period
	q := `
	SELECT
		COALESCE(SUM(CASE WHEN e.created_at < $1 THEN e.purchase_price * e.quantity ELSE 0 END), 0) AS price_at_start,
		COALESCE(SUM(CASE WHEN e.created_at < $2 THEN e.purchase_price * e.quantity ELSE 0 END), 0) AS price_at_end
	FROM entities e
	JOIN entity_types et ON et.id = e.entity_type_entities
	WHERE e.group_entities = $3
	  AND et.group_entity_types = $3
	  AND e.archived = false
	  AND e.sold_date IS NULL
	  AND et.is_location = false
`
	stats := ValueOverTime{
		Start: start,
		End:   end,
	}

	row := r.db.Sql().QueryRowContext(ctx, q, sqliteDateFormat(start), sqliteDateFormat(end), gid)
	err := row.Scan(&stats.PriceAtStart, &stats.PriceAtEnd)
	if err != nil {
		return nil, err
	}

	type itemPriceEntry struct {
		Name          string    `json:"name"`
		CreatedAt     time.Time `json:"created_at"`
		PurchasePrice float64   `json:"purchase_price"`
		Quantity      float64   `json:"quantity"`
	}

	var v []itemPriceEntry

	// Get Created Date and Price of all entities between start and end
	predicates := append(
		inventoryValuePredicates(gid),
		entity.CreatedAtGTE(start),
		entity.CreatedAtLTE(end),
	)
	err = r.db.Entity.Query().
		Where(predicates...).
		Select(
			entity.FieldName,
			entity.FieldCreatedAt,
			entity.FieldPurchasePrice,
			entity.FieldQuantity,
		).
		Scan(ctx, &v)

	if err != nil {
		return nil, err
	}

	stats.Entries = lo.Map(v, func(vv itemPriceEntry, _ int) ValueOverTimeEntry {
		return ValueOverTimeEntry{
			Date:  vv.CreatedAt,
			Value: vv.PurchasePrice * vv.Quantity,
			Name:  vv.Name,
		}
	})

	return &stats, nil
}

func (r *GroupRepository) StatsGroup(ctx context.Context, gid uuid.UUID) (GroupStatistics, error) {
	q := `
		SELECT
            (SELECT COUNT(*) FROM user_groups WHERE group_id = $2) AS total_users,
            (SELECT COUNT(*) FROM entities e JOIN entity_types et ON et.id = e.entity_type_entities WHERE e.group_entities = $2 AND et.group_entity_types = $2 AND e.archived = false AND et.is_location = false) AS total_items,
            (SELECT COUNT(*) FROM entities e JOIN entity_types et ON et.id = e.entity_type_entities WHERE e.group_entities = $2 AND et.group_entity_types = $2 AND et.is_location = true) AS total_locations,
            (SELECT COUNT(*) FROM tags WHERE group_tags = $2) AS total_tags,
            (SELECT SUM(e.purchase_price * e.quantity) FROM entities e JOIN entity_types et ON et.id = e.entity_type_entities WHERE e.group_entities = $2 AND et.group_entity_types = $2 AND e.archived = false AND e.sold_date IS NULL AND et.is_location = false) AS total_item_price,
            (SELECT COUNT(*)
                FROM entities e
                JOIN entity_types et ON et.id = e.entity_type_entities
                    WHERE e.group_entities = $2
                    AND et.group_entity_types = $2
                    AND e.archived = false
                    AND et.is_location = false
                    AND (e.lifetime_warranty = true OR e.warranty_expires > $1)
                ) AS total_with_warranty;
`
	var stats GroupStatistics
	row := r.db.Sql().QueryRowContext(ctx, q, sqliteDateFormat(time.Now()), gid)

	var maybeTotalItemPrice *float64
	var maybeTotalWithWarranty *int

	err := row.Scan(&stats.TotalUsers, &stats.TotalItems, &stats.TotalLocations, &stats.TotalTags, &maybeTotalItemPrice, &maybeTotalWithWarranty)
	if err != nil {
		return GroupStatistics{}, err
	}

	stats.TotalItemPrice = orDefault(maybeTotalItemPrice, 0)
	stats.TotalWithWarranty = orDefault(maybeTotalWithWarranty, 0)

	return stats, nil
}

func (r *GroupRepository) GroupCreate(ctx context.Context, name string, userID uuid.UUID) (Group, error) {
	if userID == uuid.Nil {
		return r.groupMapper.MapErr(r.db.Group.Create().SetName(name).Save(ctx))
	}

	tx, err := r.db.Tx(ctx)
	if err != nil {
		return Group{}, err
	}

	g, err := tx.Group.Create().SetName(name).Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return Group{}, err
	}

	// The user creating a group is its owner. This is the only place a fresh
	// owner membership comes from outside registration.
	if _, err := tx.UserGroup.Create().
		SetUserID(userID).
		SetGroupID(g.ID).
		SetRole(usergroup.RoleOwner).
		Save(ctx); err != nil {
		_ = tx.Rollback()
		return Group{}, err
	}

	if err := tx.Commit(); err != nil {
		return Group{}, err
	}
	return r.groupMapper.Map(g), nil
}

func (r *GroupRepository) GroupUpdate(ctx context.Context, id uuid.UUID, data GroupUpdate) (Group, error) {
	entity, err := r.db.Group.UpdateOneID(id).
		SetName(data.Name).
		SetCurrency(strings.ToLower(data.Currency)).
		Save(ctx)

	return r.groupMapper.MapErr(entity, err)
}

func (r *GroupRepository) GroupByID(ctx context.Context, id uuid.UUID) (Group, error) {
	return r.groupMapper.MapErr(r.db.Group.Get(ctx, id))
}

func (r *GroupRepository) GroupDelete(ctx context.Context, id uuid.UUID) error {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	entities, err := tx.Entity.Query().
		Where(entity.HasGroupWith(group.ID(id))).
		WithAttachments(func(aq *ent.AttachmentQuery) {
			aq.WithThumbnail()
		}).
		All(ctx)
	if err != nil {
		return err
	}

	attachments := make([]*ent.Attachment, 0)
	for _, e := range entities {
		attachments = append(attachments, e.Edges.Attachments...)
	}
	candidates, err := deleteAttachmentThumbnailsTx(ctx, tx, attachments)
	if err != nil {
		return err
	}
	templates, err := tx.EntityTemplate.Query().
		Where(entitytemplate.HasGroupWith(group.ID(id))).
		All(ctx)
	if err != nil {
		return err
	}
	for _, template := range templates {
		if template.PhotoPath == "" {
			continue
		}
		candidates = append(candidates, attachmentBlobCandidate{
			Path:     template.PhotoPath,
			MimeType: template.PhotoMimeType,
		})
	}

	// Every user whose default points at the deleted group must be reassigned
	// atomically. Prefer another current membership; otherwise clear it.
	defaultUsers, err := tx.User.Query().
		Where(user.DefaultGroupID(id)).
		All(ctx)
	if err != nil {
		return err
	}
	for _, member := range defaultUsers {
		replacement, err := tx.Group.Query().
			Where(
				group.IDNEQ(id),
				group.HasUsersWith(user.ID(member.ID)),
			).
			Order(ent.Asc(group.FieldCreatedAt)).
			First(ctx)
		switch {
		case err == nil:
			if err := tx.User.UpdateOneID(member.ID).SetDefaultGroupID(replacement.ID).Exec(ctx); err != nil {
				return err
			}
		case ent.IsNotFound(err):
			if err := tx.User.UpdateOneID(member.ID).ClearDefaultGroupID().Exec(ctx); err != nil {
				return err
			}
		default:
			return err
		}
	}

	if _, err := tx.Entity.Delete().
		Where(entity.HasGroupWith(group.ID(id))).
		Exec(ctx); err != nil {
		return err
	}

	if _, err := tx.Notifier.Delete().
		Where(notifier.HasGroupWith(group.ID(id))).
		Exec(ctx); err != nil {
		return err
	}

	if err := tx.Group.DeleteOneID(id).Exec(ctx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	if r.attachments != nil {
		r.attachments.cleanupUnreferencedBlobs(ctx, candidates)
	}
	return nil
}

func (r *GroupRepository) InvitationGet(ctx context.Context, token []byte) (GroupInvitation, error) {
	return r.invitationMapper.MapErr(r.db.GroupInvitationToken.Query().
		Where(groupinvitationtoken.Token(token)).
		WithGroup().
		Only(ctx))
}

func (r *GroupRepository) InvitationGetAll(ctx context.Context, groupID uuid.UUID) ([]GroupInvitation, error) {
	invitations, err := r.db.GroupInvitationToken.Query().
		Where(groupinvitationtoken.HasGroupWith(group.ID(groupID))).
		WithGroup().
		All(ctx)
	if err != nil {
		return nil, err
	}

	return r.invitationMapper.MapEach(invitations), nil
}

func (r *GroupRepository) InvitationCreate(ctx context.Context, groupID uuid.UUID, invite GroupInvitationCreate) (GroupInvitation, error) {
	entity, err := r.db.GroupInvitationToken.Create().
		SetGroupID(groupID).
		SetToken(invite.Token).
		SetExpiresAt(invite.ExpiresAt).
		SetUses(invite.Uses).
		Save(ctx)
	if err != nil {
		return GroupInvitation{}, err
	}

	return r.InvitationGet(ctx, entity.Token)
}

func (r *GroupRepository) InvitationUpdate(ctx context.Context, id uuid.UUID, uses int) error {
	_, err := r.db.GroupInvitationToken.UpdateOneID(id).SetUses(uses).Save(ctx)
	return err
}

func (r *GroupRepository) InvitationDelete(ctx context.Context, groupID uuid.UUID, id uuid.UUID) error {
	n, err := r.db.GroupInvitationToken.Delete().
		Where(
			groupinvitationtoken.ID(id),
			groupinvitationtoken.HasGroupWith(group.ID(groupID)),
		).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return &ent.NotFoundError{}
	}
	return nil
}

// InvitationPurge removes all expired invitations or those that have been used up.
// It returns the number of deleted invitations.
func (r *GroupRepository) InvitationPurge(ctx context.Context) (amount int, err error) {
	q := r.db.GroupInvitationToken.Delete()
	q.Where(groupinvitationtoken.Or(
		groupinvitationtoken.ExpiresAtLT(time.Now()),
		groupinvitationtoken.UsesLTE(0),
	))

	return q.Exec(ctx)
}

func (r *GroupRepository) IsMember(ctx context.Context, groupID, userID uuid.UUID) (bool, error) {
	return r.db.Group.Query().
		Where(group.ID(groupID), group.HasUsersWith(user.ID(userID))).
		Exist(ctx)
}

// IsOwnerOf reports whether userID has role=owner on groupID. This is the
// authoritative per-group ownership check and the only one authorization
// code should consult — there is intentionally no global owner flag.
func (r *GroupRepository) IsOwnerOf(ctx context.Context, userID, groupID uuid.UUID) (bool, error) {
	return r.db.UserGroup.Query().
		Where(
			usergroup.UserID(userID),
			usergroup.GroupID(groupID),
			usergroup.RoleEQ(usergroup.RoleOwner),
		).
		Exist(ctx)
}

// IntegrationsGet returns the group's stored integration settings. Groups
// with no settings row yet return the zero value (all fields "", meaning
// "inherit env" for every field).
func (r *GroupRepository) IntegrationsGet(ctx context.Context, gid uuid.UUID) (types.GroupIntegrations, error) {
	g, err := r.db.Group.Get(ctx, gid)
	if err != nil {
		return types.GroupIntegrations{}, err
	}
	return g.Integrations, nil
}

// IntegrationsSet overwrites the group's stored integration settings.
func (r *GroupRepository) IntegrationsSet(ctx context.Context, gid uuid.UUID, data types.GroupIntegrations) error {
	return r.db.Group.UpdateOneID(gid).SetIntegrations(data).Exec(ctx)
}

func (r *GroupRepository) RemoveMember(ctx context.Context, groupID, userID uuid.UUID) error {
	return r.db.Group.UpdateOneID(groupID).RemoveUserIDs(userID).Exec(ctx)
}

// RemoveMemberAndReassignDefault removes one membership and, when necessary,
// points the user's default at their oldest remaining group. Both mutations
// commit together so a failed reassignment cannot strand a user with a
// default group they no longer belong to.
func (r *GroupRepository) RemoveMemberAndReassignDefault(ctx context.Context, userID, removedGroupID uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, &stdsql.TxOptions{Isolation: stdsql.LevelSerializable})
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Warn().Err(rollbackErr).Msg("failed to rollback member removal")
			}
		}
	}()

	member, err := tx.User.Get(ctx, userID)
	if err != nil {
		return err
	}

	deleted, err := tx.UserGroup.Delete().
		Where(
			usergroup.UserID(userID),
			usergroup.GroupID(removedGroupID),
		).
		Exec(ctx)
	if err != nil {
		return err
	}
	if deleted != 1 {
		return &ent.NotFoundError{}
	}

	if member.DefaultGroupID != nil && *member.DefaultGroupID == removedGroupID {
		replacement, err := tx.Group.Query().
			Where(group.HasUsersWith(user.ID(userID))).
			Order(
				ent.Asc(group.FieldCreatedAt),
				ent.Asc(group.FieldID),
			).
			First(ctx)
		switch {
		case err == nil:
			if err := tx.User.UpdateOneID(userID).SetDefaultGroupID(replacement.ID).Exec(ctx); err != nil {
				return err
			}
		case ent.IsNotFound(err):
			if err := tx.User.UpdateOneID(userID).ClearDefaultGroupID().Exec(ctx); err != nil {
				return err
			}
		default:
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *GroupRepository) InvitationDecrement(ctx context.Context, id uuid.UUID) error {
	n, err := r.db.GroupInvitationToken.Update().
		Where(
			groupinvitationtoken.ID(id),
			groupinvitationtoken.UsesGT(0),
		).
		AddUses(-1).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvitationExhausted
	}
	return nil
}

func (r *GroupRepository) InvitationAccept(ctx context.Context, token []byte, userID uuid.UUID) (Group, error) {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return Group{}, err
	}

	// 1. Get invitation
	invitation, err := tx.GroupInvitationToken.Query().
		Where(groupinvitationtoken.Token(token)).
		WithGroup().
		Only(ctx)
	if err != nil {
		if err := tx.Rollback(); err != nil {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
		return Group{}, err
	}

	// 2. Checks
	if invitation.ExpiresAt.Before(time.Now()) {
		if err := tx.Rollback(); err != nil {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
		return Group{}, ErrInvitationExpired
	}
	if invitation.Uses <= 0 {
		if err := tx.Rollback(); err != nil {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
		return Group{}, ErrInvitationExhausted
	}

	// 3. Check membership
	isMember, err := tx.Group.Query().
		Where(group.ID(invitation.Edges.Group.ID), group.HasUsersWith(user.ID(userID))).
		Exist(ctx)
	if err != nil {
		if err := tx.Rollback(); err != nil {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
		return Group{}, err
	}
	if isMember {
		if err := tx.Rollback(); err != nil {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
		return Group{}, ErrAlreadyGroupMember
	}

	// 4. Add member with role=user; ownership is reserved for whoever created the group.
	if _, err := tx.UserGroup.Create().
		SetUserID(userID).
		SetGroupID(invitation.Edges.Group.ID).
		SetRole(usergroup.RoleUser).
		Save(ctx); err != nil {
		if err := tx.Rollback(); err != nil {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
		return Group{}, err
	}

	// 5. Decrement uses atomically
	n, err := tx.GroupInvitationToken.Update().
		Where(
			groupinvitationtoken.ID(invitation.ID),
			groupinvitationtoken.UsesGT(0),
		).
		AddUses(-1).
		Save(ctx)
	if err != nil {
		if err := tx.Rollback(); err != nil {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
		return Group{}, err
	}
	if n == 0 {
		if err := tx.Rollback(); err != nil {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
		return Group{}, ErrInvitationExhausted
	}

	if err := tx.Commit(); err != nil {
		return Group{}, err
	}

	return r.groupMapper.Map(invitation.Edges.Group), nil
}
