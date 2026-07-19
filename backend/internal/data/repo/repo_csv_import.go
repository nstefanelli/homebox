package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entity"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entityfield"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entitytype"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/group"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/tag"
)

// CSVImportRow is the repository-level representation of one parsed CSV row.
// Entity contains the scalar values and custom fields to apply. ParentID,
// TagIDs, and ID are resolved transactionally from the other row properties.
type CSVImportRow struct {
	ImportRef       string
	ParentImportRef string
	Location        []string
	TagNames        []string
	Entity          EntityUpdate
}

// ImportCSV applies a parsed CSV import as one serializable transaction. Any
// error, including a parent-reference error discovered after all rows have
// been written, rolls back entities, locations, tags, fields, and type rows
// created by the import.
func (r *EntityRepository) ImportCSV(
	ctx context.Context,
	gid uuid.UUID,
	rows []CSVImportRow,
	autoIncrementAssetID bool,
) (int, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Warn().Err(rollbackErr).Msg("failed to rollback CSV import")
			}
		}
	}()

	finished, err := r.importCSVTx(ctx, tx, gid, rows, autoIncrementAssetID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true

	if finished > 0 {
		r.publishMutationEvent(gid)
		(&TagRepository{db: r.db, bus: r.bus}).publishMutationEvent(gid)
	}
	return finished, nil
}

func (r *EntityRepository) importCSVTx(
	ctx context.Context,
	tx *ent.Tx,
	gid uuid.UUID,
	rows []CSVImportRow,
	autoIncrementAssetID bool,
) (int, error) {
	tagMap, err := loadCSVTagMap(ctx, tx, gid)
	if err != nil {
		return 0, err
	}
	locationMap, err := loadCSVLocationMap(ctx, tx, gid)
	if err != nil {
		return 0, err
	}

	highestAssetID := AssetID(-1)
	if autoIncrementAssetID {
		highestAssetID, err = r.GetHighestAssetIDTx(ctx, tx, gid)
		if err != nil {
			return 0, err
		}
	}

	var itemTypeID uuid.UUID
	var locationTypeID uuid.UUID
	finished := 0

	for i := range rows {
		row := rows[i]
		if err := validateQuantity("CSV import", row.Entity.Quantity); err != nil {
			return 0, err
		}

		tagIDs := make([]uuid.UUID, len(row.TagNames))
		for j, name := range row.TagNames {
			id, ok := tagMap[name]
			if !ok {
				created, err := tx.Tag.Create().
					SetName(name).
					SetGroupID(gid).
					Save(ctx)
				if err != nil {
					return 0, err
				}
				id = created.ID
				tagMap[name] = id
			}
			tagIDs[j] = id
		}

		locationID, ok := locationMap[strings.Join(row.Location, "/")]
		if !ok {
			pathParts := make([]string, 0, len(row.Location))
			for _, name := range row.Location {
				pathParts = append(pathParts, name)
				path := strings.Join(pathParts, "/")
				if existingID, exists := locationMap[path]; exists {
					locationID = existingID
					continue
				}

				if locationTypeID == uuid.Nil {
					locationTypeID, err = resolveCSVDefaultEntityType(ctx, tx, gid, true)
					if err != nil {
						return 0, err
					}
				}

				create := tx.Entity.Create().
					SetName(name).
					SetGroupID(gid).
					SetEntityTypeID(locationTypeID)
				if len(pathParts) > 1 {
					parentPath := strings.Join(pathParts[:len(pathParts)-1], "/")
					parentID, exists := locationMap[parentPath]
					if !exists {
						return 0, fmt.Errorf("failed to resolve parent location %q", parentPath)
					}
					create.SetParentID(parentID)
				}

				created, err := create.Save(ctx)
				if err != nil {
					return 0, err
				}
				locationID = created.ID
				locationMap[path] = locationID
			}

			if locationID == uuid.Nil {
				return 0, errors.New("failed to create location")
			}
		}

		effectiveAssetID := row.Entity.AssetID
		if autoIncrementAssetID && effectiveAssetID.Nil() {
			highestAssetID++
			effectiveAssetID = highestAssetID
		}

		entityID, exists, err := csvEntityIDByRef(ctx, tx, gid, row.ImportRef)
		if err != nil {
			return 0, fmt.Errorf("error checking for existing entity with ref %q: %w", row.ImportRef, err)
		}
		if !exists {
			if itemTypeID == uuid.Nil {
				itemTypeID, err = resolveCSVDefaultEntityType(ctx, tx, gid, false)
				if err != nil {
					return 0, err
				}
			}
			created, err := tx.Entity.Create().
				SetImportRef(row.ImportRef).
				SetName(row.Entity.Name).
				SetGroupID(gid).
				SetEntityTypeID(itemTypeID).
				Save(ctx)
			if err != nil {
				return 0, err
			}
			entityID = created.ID
		}

		update := row.Entity
		update.ID = entityID
		update.ParentID = locationID
		update.TagIDs = tagIDs
		update.AssetID = effectiveAssetID
		if err := applyCSVEntityUpdate(ctx, tx, gid, update); err != nil {
			return 0, err
		}

		finished++
	}

	if err := patchCSVParentRefsTx(ctx, tx, gid, rows); err != nil {
		return 0, err
	}
	return finished, nil
}

func loadCSVTagMap(ctx context.Context, tx *ent.Tx, gid uuid.UUID) (map[string]uuid.UUID, error) {
	tags, err := tx.Tag.Query().
		Where(tag.HasGroupWith(group.ID(gid))).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[string]uuid.UUID, len(tags))
	for _, existing := range tags {
		out[existing.Name] = existing.ID
	}
	return out, nil
}

func loadCSVLocationMap(ctx context.Context, tx *ent.Tx, gid uuid.UUID) (map[string]uuid.UUID, error) {
	locations, err := tx.Entity.Query().
		Where(
			entity.HasGroupWith(group.ID(gid)),
			entity.HasEntityTypeWith(
				entitytype.IsLocation(true),
				entitytype.HasGroupWith(group.ID(gid)),
			),
		).
		WithParent(func(parentQuery *ent.EntityQuery) {
			parentQuery.Where(entity.HasGroupWith(group.ID(gid)))
		}).
		All(ctx)
	if err != nil {
		return nil, err
	}

	byID := make(map[uuid.UUID]*ent.Entity, len(locations))
	for _, location := range locations {
		byID[location.ID] = location
	}

	pathsByID := make(map[uuid.UUID]string, len(locations))
	visiting := make(map[uuid.UUID]bool, len(locations))
	var resolvePath func(uuid.UUID, int) (string, error)
	resolvePath = func(id uuid.UUID, depth int) (string, error) {
		if path, ok := pathsByID[id]; ok {
			return path, nil
		}
		if depth >= maxHierarchyDepth {
			return "", fmt.Errorf("location hierarchy exceeds maximum depth of %d", maxHierarchyDepth)
		}
		if visiting[id] {
			return "", fmt.Errorf("location hierarchy cycle detected at %s", id)
		}
		location, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("location %s not found", id)
		}

		visiting[id] = true
		path := location.Name
		if parent := location.Edges.Parent; parent != nil {
			if _, parentIsLocation := byID[parent.ID]; parentIsLocation {
				parentPath, err := resolvePath(parent.ID, depth+1)
				if err != nil {
					return "", err
				}
				path = parentPath + "/" + path
			}
		}
		delete(visiting, id)
		pathsByID[id] = path
		return path, nil
	}

	out := make(map[string]uuid.UUID, len(locations))
	for _, location := range locations {
		path, err := resolvePath(location.ID, 0)
		if err != nil {
			return nil, err
		}
		out[path] = location.ID
	}
	return out, nil
}

func resolveCSVDefaultEntityType(
	ctx context.Context,
	tx *ent.Tx,
	gid uuid.UUID,
	isLocation bool,
) (uuid.UUID, error) {
	existing, err := tx.EntityType.Query().
		Where(
			entitytype.HasGroupWith(group.ID(gid)),
			entitytype.IsLocation(isLocation),
		).
		Order(entitytype.ByCreatedAt()).
		First(ctx)
	if err == nil {
		return existing.ID, nil
	}
	if !ent.IsNotFound(err) {
		return uuid.Nil, err
	}

	name := "Item"
	if isLocation {
		name = "Location"
	}
	created, err := tx.EntityType.Create().
		SetName(name).
		SetDescription("").
		SetIsLocation(isLocation).
		SetGroupID(gid).
		Save(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return created.ID, nil
}

func csvEntityIDByRef(
	ctx context.Context,
	tx *ent.Tx,
	gid uuid.UUID,
	importRef string,
) (uuid.UUID, bool, error) {
	if importRef == "" {
		return uuid.Nil, false, nil
	}
	id, err := tx.Entity.Query().
		Where(
			entity.ImportRef(importRef),
			entity.HasGroupWith(group.ID(gid)),
		).
		OnlyID(ctx)
	if ent.IsNotFound(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

func applyCSVEntityUpdate(
	ctx context.Context,
	tx *ent.Tx,
	gid uuid.UUID,
	data EntityUpdate,
) error {
	if err := assertEntityInGroup(ctx, tx.Entity, gid, data.ID); err != nil {
		return err
	}
	if err := assertEntityInGroup(ctx, tx.Entity, gid, data.ParentID); err != nil {
		return err
	}
	if err := assertEntityTypeInGroup(ctx, tx.EntityType, gid, data.EntityTypeID); err != nil {
		return err
	}
	if err := assertTagsInGroup(ctx, tx.Tag, gid, data.TagIDs); err != nil {
		return err
	}
	if err := assertValidEntityParent(ctx, tx.Entity, gid, data.ID, data.ParentID); err != nil {
		return err
	}

	update := tx.Entity.Update().
		Where(entity.ID(data.ID), entity.HasGroupWith(group.ID(gid))).
		SetName(data.Name).
		SetDescription(data.Description).
		SetSerialNumber(data.SerialNumber).
		SetModelNumber(data.ModelNumber).
		SetManufacturer(data.Manufacturer).
		SetIcon(data.Icon).
		SetArchived(data.Archived).
		SetPurchaseFrom(data.PurchaseFrom).
		SetPurchasePrice(data.PurchasePrice).
		SetSoldTo(data.SoldTo).
		SetSoldPrice(data.SoldPrice).
		SetSoldNotes(data.SoldNotes).
		SetNotes(data.Notes).
		SetLifetimeWarranty(data.LifetimeWarranty).
		SetInsured(data.Insured).
		SetWarrantyDetails(data.WarrantyDetails).
		SetQuantity(data.Quantity).
		SetAssetID(int64(data.AssetID)).
		SetSyncChildEntityLocations(data.SyncChildEntityLocations)
	applyEntityUpdateDates(update, data)

	if data.EntityTypeID != uuid.Nil {
		update.SetEntityTypeID(data.EntityTypeID)
	}
	if data.ParentID == uuid.Nil {
		update.ClearParent()
	} else {
		update.SetParentID(data.ParentID)
	}

	currentTags, err := tx.Entity.Query().
		Where(entity.ID(data.ID), entity.HasGroupWith(group.ID(gid))).
		QueryTag().
		All(ctx)
	if err != nil {
		return err
	}
	currentTagIDs := newIDSet(currentTags)
	for _, id := range data.TagIDs {
		if currentTagIDs.Contains(id) {
			currentTagIDs.Remove(id)
		} else {
			update.AddTagIDs(id)
		}
	}
	if currentTagIDs.Len() > 0 {
		update.RemoveTagIDs(currentTagIDs.Slice()...)
	}
	if err := update.Exec(ctx); err != nil {
		return err
	}

	if _, err := tx.EntityField.Delete().
		Where(entityfield.HasEntityWith(
			entity.ID(data.ID),
			entity.HasGroupWith(group.ID(gid)),
		)).
		Exec(ctx); err != nil {
		return err
	}
	for _, field := range data.Fields {
		create := tx.EntityField.Create().
			SetEntityID(data.ID).
			SetType(entityfield.Type(field.Type)).
			SetName(field.Name).
			SetTextValue(field.TextValue).
			SetNumberValue(field.NumberValue).
			SetBooleanValue(field.BooleanValue)
		if !field.TimeValue.IsZero() {
			create.SetTimeValue(field.TimeValue)
		}
		if _, err := create.Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func patchCSVParentRefsTx(
	ctx context.Context,
	tx *ent.Tx,
	gid uuid.UUID,
	rows []CSVImportRow,
) error {
	for i := range rows {
		row := rows[i]
		if row.ImportRef == "" || row.ParentImportRef == "" {
			continue
		}

		childID, childExists, err := csvEntityIDByRef(ctx, tx, gid, row.ImportRef)
		if err != nil || !childExists {
			if err == nil {
				err = &ent.NotFoundError{}
			}
			return fmt.Errorf("error resolving child entity with ref %q: %w", row.ImportRef, err)
		}
		parentID, parentExists, err := csvEntityIDByRef(ctx, tx, gid, row.ParentImportRef)
		if err != nil || !parentExists {
			if err == nil {
				err = &ent.NotFoundError{}
			}
			return fmt.Errorf("error resolving parent entity with ref %q: %w", row.ParentImportRef, err)
		}
		if childID == parentID {
			return fmt.Errorf(
				"invalid parent relationship: entity %q cannot be its own parent",
				row.ImportRef,
			)
		}
		if err := assertValidEntityParent(ctx, tx.Entity, gid, childID, parentID); err != nil {
			return err
		}
		if err := tx.Entity.Update().
			Where(entity.ID(childID), entity.HasGroupWith(group.ID(gid))).
			SetParentID(parentID).
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
