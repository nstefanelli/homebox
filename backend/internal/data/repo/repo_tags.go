package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services/reporting/eventbus"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/group"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/predicate"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/tag"
)

// TagRepository provides data access operations for tag entities.
// It supports hierarchical tag structures with parent-child relationships
// and enforces multi-tenant isolation through group membership.
type TagRepository struct {
	db  *ent.Client
	bus *eventbus.EventBus
}

const (
	maxTagDepth          = 5
	maxTagTraversalDepth = 256
)

type (
	// TagCreate represents the input data required to create a new tag.
	// Tags can optionally have a parent to form hierarchical structures
	// with a maximum depth of 5 levels.
	TagCreate struct {
		Name        string    `json:"name"        validate:"required,min=1,max=255"`
		ParentID    uuid.UUID `json:"parentId"    extensions:"x-nullable"`
		Description string    `json:"description" validate:"max=1000"`
		Color       string    `json:"color"`
		Icon        string    `json:"icon"        validate:"max=255"`
	}

	// TagUpdate represents the input data for updating an existing tag.
	// All fields can be modified, including moving the tag to a different
	// parent (while maintaining hierarchy constraints and preventing cycles).
	TagUpdate struct {
		ID          uuid.UUID `json:"id"`
		ParentID    uuid.UUID `json:"parentId"    extensions:"x-nullable"`
		Name        string    `json:"name"        validate:"required,min=1,max=255"`
		Description string    `json:"description" validate:"max=1000"`
		Color       string    `json:"color"`
		Icon        string    `json:"icon"        validate:"max=255"`
	}

	// TagSummary provides a lightweight representation of a tag without
	// its relationships. Used in lists and as nested references.
	TagSummary struct {
		ID          uuid.UUID `json:"id"`
		ParentID    uuid.UUID `json:"parentId"    extensions:"x-nullable"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Color       string    `json:"color"`
		Icon        string    `json:"icon"`
		CreatedAt   time.Time `json:"createdAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}

	// TagOut represents a complete tag with its parent and children relationships.
	// The Parent field is nil for root-level tags. Children is always initialized
	// (empty slice if the tag has no children).
	TagOut struct {
		TagSummary
		Parent   *TagSummary  `json:"parent,omitempty" extensions:"x-nullable"`
		Children []TagSummary `json:"children"`
	}
)

func mapTagSummary(tag *ent.Tag) TagSummary {
	parentID := uuid.Nil
	if tag.Edges.Parent != nil {
		parentID = tag.Edges.Parent.ID
	}

	return TagSummary{
		ID:          tag.ID,
		ParentID:    parentID,
		Name:        tag.Name,
		Description: tag.Description,
		Color:       tag.Color,
		Icon:        tag.Icon,
		CreatedAt:   tag.CreatedAt,
		UpdatedAt:   tag.UpdatedAt,
	}
}

var (
	mapTagOutErr = mapTErrFunc(mapTagOut)
	mapTagsOut   = mapTEachErrFunc(mapTagSummary)
)

func mapTagOut(tag *ent.Tag) TagOut {
	parent := lo.TernaryF(
		tag.Edges.Parent != nil,
		func() *TagSummary {
			p := mapTagSummary(tag.Edges.Parent)
			return &p
		},
		func() *TagSummary { return nil },
	)

	children := []TagSummary{}
	if tag.Edges.Children != nil {
		children = lo.Map(tag.Edges.Children, func(c *ent.Tag, _ int) TagSummary {
			summary := mapTagSummary(c)
			summary.ParentID = tag.ID
			return summary
		})
	}

	return TagOut{
		TagSummary: mapTagSummary(tag),
		Parent:     parent,
		Children:   children,
	}
}

func (r *TagRepository) publishMutationEvent(gid uuid.UUID) {
	if r.bus != nil {
		r.bus.Publish(eventbus.EventTagMutation, eventbus.GroupMutationEvent{GID: gid})
	}
}

func getOneTag(
	ctx context.Context,
	client *ent.Client,
	gid uuid.UUID,
	where ...predicate.Tag,
) (TagOut, error) {
	return mapTagOutErr(client.Tag.Query().
		Where(where...).
		WithGroup().
		WithParent(func(parentQuery *ent.TagQuery) {
			parentQuery.Where(tag.HasGroupWith(group.ID(gid)))
		}).
		WithChildren(func(childrenQuery *ent.TagQuery) {
			childrenQuery.Where(tag.HasGroupWith(group.ID(gid)))
		}).
		Only(ctx),
	)
}

// GetOne retrieves a single tag by ID, ensuring it belongs to the specified group.
// Returns the tag with its parent and children relationships fully populated.
// Returns an error if the tag doesn't exist or doesn't belong to the group.
func (r *TagRepository) GetOne(ctx context.Context, gid uuid.UUID, id uuid.UUID) (TagOut, error) {
	return getOneTag(ctx, r.db, gid, tag.ID(id), tag.HasGroupWith(group.ID(gid)))
}

// GetOneByGroup retrieves a single tag by ID with group validation.
// This is an alias for GetOne, maintained for API consistency with other repositories.
func (r *TagRepository) GetOneByGroup(ctx context.Context, gid, id uuid.UUID) (TagOut, error) {
	return getOneTag(ctx, r.db, gid, tag.ID(id), tag.HasGroupWith(group.ID(gid)))
}

// GetAll retrieves all tags belonging to the specified group, ordered by name.
// Tags are returned as summaries (without parent/children relationships loaded).
// Parent edges are loaded to populate ParentID fields in the summaries.
func (r *TagRepository) GetAll(ctx context.Context, groupID uuid.UUID) ([]TagSummary, error) {
	return mapTagsOut(r.db.Tag.Query().
		Where(tag.HasGroupWith(group.ID(groupID))).
		Order(ent.Asc(tag.FieldName)).
		WithGroup().
		WithParent(func(parentQuery *ent.TagQuery) {
			parentQuery.Where(tag.HasGroupWith(group.ID(groupID)))
		}).
		All(ctx),
	)
}

// GetDescendantTagIDs retrieves all descendant tag IDs for the given parent tag IDs.
// Returns all tags that are direct or indirect children of any of the provided tag IDs.
// Uses recursive in-memory traversal since Ent doesn't support recursive CTEs directly.
func (r *TagRepository) GetDescendantTagIDs(ctx context.Context, gid uuid.UUID, tagIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(tagIDs) == 0 {
		return []uuid.UUID{}, nil
	}
	if err := assertTagsInGroup(ctx, r.db.Tag, gid, tagIDs); err != nil {
		return nil, err
	}

	result := make(map[uuid.UUID]bool, len(tagIDs))
	for _, id := range tagIDs {
		result[id] = true
	}

	frontier := append([]uuid.UUID(nil), tagIDs...)
	for depth := 0; len(frontier) > 0 && depth < maxTagTraversalDepth; depth++ {
		children, err := r.db.Tag.Query().
			Where(
				tag.HasGroupWith(group.ID(gid)),
				tag.HasParentWith(
					tag.IDIn(frontier...),
					tag.HasGroupWith(group.ID(gid)),
				),
			).
			All(ctx)
		if err != nil {
			return nil, err
		}

		next := make([]uuid.UUID, 0, len(children))
		for _, child := range children {
			if !result[child.ID] {
				result[child.ID] = true
				next = append(next, child.ID)
			}
		}
		frontier = next
	}
	if len(frontier) > 0 {
		return nil, fmt.Errorf("tag hierarchy exceeds maximum traversal depth of %d", maxTagTraversalDepth)
	}

	descendantIDs := make([]uuid.UUID, 0, len(result))
	for id := range result {
		descendantIDs = append(descendantIDs, id)
	}

	return descendantIDs, nil
}

// getSubtreeDepth calculates the maximum depth of the subtree rooted at the given tag ID.
// Uses a recursive CTE to traverse the entire subtree and find the deepest level.
// Returns 1 for a tag with no children, and increases by 1 for each level.
func getTagSubtreeDepth(ctx context.Context, client *ent.Client, gid, id uuid.UUID) (int, error) {
	query := `
		WITH RECURSIVE tag_tree(id, depth) AS (
			SELECT id, 1 as depth
			FROM tags
			WHERE id = $1
			  AND group_tags = $2
			UNION ALL
			SELECT t.id, tt.depth + 1
			FROM tags t
			JOIN tag_tree tt ON t.tag_children = tt.id
			WHERE t.group_tags = $2
			  AND tt.depth < $3
		)
		SELECT COALESCE(MAX(depth), 0) FROM tag_tree;
	`
	rows, err := client.RawQueryContext(ctx, query, id, gid, maxTagTraversalDepth)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	if rows.Next() {
		var maxDepth int
		if err := rows.Scan(&maxDepth); err != nil {
			return 0, err
		}
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return maxDepth, nil
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return 0, nil
}

// checkDepth calculates how many levels deep the given parent tag is from the root.
// Uses a recursive CTE to traverse up the tree from the parent to the root.
// Returns 0 for root-level tags (parentID is uuid.Nil).
// Returns the number of levels from root to the given parent tag.
func getTagAncestorDepth(ctx context.Context, client *ent.Client, gid, parentID uuid.UUID) (int, error) {
	if parentID == uuid.Nil {
		return 0, nil
	}

	query := `
		WITH RECURSIVE tag_parents(id, parent_id, depth) AS (
			SELECT id, tag_children, 1
			FROM tags
			WHERE id = $1
			  AND group_tags = $2
			UNION ALL
			SELECT t.id, t.tag_children, tp.depth + 1
			FROM tags t
			JOIN tag_parents tp ON t.id = tp.parent_id
			WHERE t.group_tags = $2
			  AND tp.depth < $3
		)
		SELECT COALESCE(MAX(depth), 0) FROM tag_parents;
	`
	rows, err := client.RawQueryContext(ctx, query, parentID, gid, maxTagTraversalDepth)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	if rows.Next() {
		var depth int
		if err := rows.Scan(&depth); err != nil {
			return 0, err
		}
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return depth, nil
	}

	if err := rows.Err(); err != nil {
		return 0, err
	}
	return 0, nil
}

// checkCycle checks if setting movingID's parent to proposedParentID would create a cycle.
// Returns true if proposedParentID is a descendant of movingID (or if they are the same tag).
// Uses a recursive CTE to traverse all descendants of movingID.
// This prevents circular parent-child relationships in the tag hierarchy.
func tagMoveCreatesCycle(
	ctx context.Context,
	client *ent.Client,
	gid, movingID, proposedParentID uuid.UUID,
) (bool, error) {
	if movingID == proposedParentID {
		return true, nil
	}

	query := `
		WITH RECURSIVE ancestors(id, parent_id, depth) AS (
			SELECT id, tag_children, 1
			FROM tags
			WHERE id = $1
			  AND group_tags = $2
			UNION ALL
			SELECT t.id, t.tag_children, a.depth + 1
			FROM tags t
			JOIN ancestors a ON t.id = a.parent_id
			WHERE t.group_tags = $2
			  AND a.depth < $3
		)
		SELECT 1 FROM ancestors WHERE id = $4 LIMIT 1;
	`

	rows, err := client.RawQueryContext(
		ctx,
		query,
		proposedParentID,
		gid,
		maxTagTraversalDepth,
		movingID,
	)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	if rows.Next() {
		return true, nil
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

// Create creates a new tag in the specified group.
// If ParentID is provided, validates that:
//   - The parent tag exists and belongs to the same group
//   - Adding this tag would not exceed the maximum depth of 5 levels
//
// Returns the created tag with all relationships fully populated.
// Publishes a tag mutation event on successful creation.
func (r *TagRepository) Create(ctx context.Context, groupID uuid.UUID, data TagCreate) (TagOut, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return TagOut{}, err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Warn().Err(rollbackErr).Msg("failed to rollback tag creation")
			}
		}
	}()

	if data.ParentID != uuid.Nil {
		_, err := tx.Tag.Query().
			Where(
				tag.ID(data.ParentID),
				tag.HasGroupWith(group.ID(groupID)),
			).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return TagOut{}, fmt.Errorf("parent tag not found or does not belong to this group")
			}
			return TagOut{}, err
		}

		parentDepth, err := getTagAncestorDepth(ctx, tx.Client(), groupID, data.ParentID)
		if err != nil {
			return TagOut{}, err
		}
		if parentDepth+1 > maxTagDepth {
			return TagOut{}, fmt.Errorf("max depth of %d exceeded", maxTagDepth)
		}
	}

	q := tx.Tag.Create().
		SetName(data.Name).
		SetDescription(data.Description).
		SetColor(data.Color).
		SetIcon(data.Icon).
		SetGroupID(groupID)

	if data.ParentID != uuid.Nil {
		q.SetParentID(data.ParentID)
	}

	createdTag, err := q.Save(ctx)
	if err != nil {
		return TagOut{}, err
	}

	freshTag, err := getOneTag(
		ctx,
		tx.Client(),
		groupID,
		tag.ID(createdTag.ID),
		tag.HasGroupWith(group.ID(groupID)),
	)
	if err != nil {
		return TagOut{}, err
	}
	if err := tx.Commit(); err != nil {
		return TagOut{}, err
	}
	committed = true

	r.publishMutationEvent(groupID)
	return freshTag, nil
}

func updateTag(
	ctx context.Context,
	client *ent.Client,
	groupID uuid.UUID,
	data TagUpdate,
	where ...predicate.Tag,
) (int, error) {
	if len(where) == 0 {
		panic("empty where not supported empty")
	}

	if data.ParentID != uuid.Nil {
		_, err := client.Tag.Query().
			Where(
				tag.ID(data.ParentID),
				tag.HasGroupWith(group.ID(groupID)),
			).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return 0, fmt.Errorf("parent tag not found or does not belong to this group")
			}
			return 0, err
		}

		isCycle, err := tagMoveCreatesCycle(ctx, client, groupID, data.ID, data.ParentID)
		if err != nil {
			return 0, err
		}
		if isCycle {
			return 0, fmt.Errorf("cycle detected")
		}

		parentDepth, err := getTagAncestorDepth(ctx, client, groupID, data.ParentID)
		if err != nil {
			return 0, err
		}

		mySubtreeDepth, err := getTagSubtreeDepth(ctx, client, groupID, data.ID)
		if err != nil {
			return 0, err
		}

		if parentDepth+mySubtreeDepth > maxTagDepth {
			return 0, fmt.Errorf("max depth of %d exceeded", maxTagDepth)
		}
	}

	q := client.Tag.Update().
		Where(where...).
		SetName(data.Name).
		SetDescription(data.Description).
		SetColor(data.Color).
		SetIcon(data.Icon)

	if data.ParentID != uuid.Nil {
		q.SetParentID(data.ParentID)
	} else {
		q.ClearParent()
	}

	return q.Save(ctx)
}

// UpdateByGroup updates an existing tag within the specified group.
// Validates that the tag exists and belongs to the group before updating.
// If ParentID is changed, additionally validates:
//   - The new parent tag exists and belongs to the same group
//   - The change would not create a cycle in the hierarchy
//   - The resulting tree would not exceed the maximum depth of 5 levels
//
// Returns an error if the tag is not found, belongs to a different group,
// or if the update would violate hierarchy constraints.
// Publishes a tag mutation event on successful update.
func (r *TagRepository) UpdateByGroup(ctx context.Context, gid uuid.UUID, data TagUpdate) (TagOut, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return TagOut{}, err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Warn().Err(rollbackErr).Msg("failed to rollback tag update")
			}
		}
	}()

	if _, err := tx.Tag.Query().
		Where(
			tag.ID(data.ID),
			tag.HasGroupWith(group.ID(gid)),
		).
		Only(ctx); err != nil {
		return TagOut{}, err
	}

	affected, err := updateTag(
		ctx,
		tx.Client(),
		gid,
		data,
		tag.ID(data.ID),
		tag.HasGroupWith(group.ID(gid)),
	)
	if err != nil {
		return TagOut{}, err
	}

	if affected == 0 {
		return TagOut{}, &ent.NotFoundError{}
	}

	out, err := getOneTag(
		ctx,
		tx.Client(),
		gid,
		tag.ID(data.ID),
		tag.HasGroupWith(group.ID(gid)),
	)
	if err != nil {
		return TagOut{}, err
	}
	if err := tx.Commit(); err != nil {
		return TagOut{}, err
	}
	committed = true

	r.publishMutationEvent(gid)
	return out, nil
}

// delete removes the tag from the database. This should only be used when
// the tag's ownership is already confirmed/validated.
func (r *TagRepository) delete(ctx context.Context, id uuid.UUID) error {
	return r.db.Tag.DeleteOneID(id).Exec(ctx)
}

// DeleteByGroup deletes a tag from the specified group.
// Only deletes the tag if it exists and belongs to the group.
// Note: Child tags are not automatically deleted - they become root-level tags
// if their parent is deleted (depending on database cascade settings).
// Publishes a tag mutation event on successful deletion.
func (r *TagRepository) DeleteByGroup(ctx context.Context, gid, id uuid.UUID) error {
	deleted, err := r.db.Tag.Delete().
		Where(
			tag.ID(id),
			tag.HasGroupWith(group.ID(gid)),
		).Exec(ctx)
	if err != nil {
		return err
	}
	if deleted != 1 {
		return &ent.NotFoundError{}
	}

	r.publishMutationEvent(gid)

	return nil
}
