package repo

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/attachment"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entity"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/group"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
)

const (
	testFieldTypeNumber  = "number"
	testFieldTypeBoolean = "boolean"
)

func containerFactory() EntityCreate {
	return EntityCreate{
		Name:        fk.Str(10),
		Description: fk.Str(100),
	}
}

func entityFactory() EntityCreate {
	return EntityCreate{
		Name:        fk.Str(10),
		Description: fk.Str(100),
	}
}

// useContainerEntityType creates or gets a default location entity type for the test group.
func useContainerEntityType(t *testing.T) EntityTypeSummary {
	t.Helper()
	et, err := tRepos.EntityTypes.GetDefault(context.Background(), tGroup.ID, true)
	require.NoError(t, err)
	return et
}

// useItemEntityType creates or gets a default item entity type for the test group.
func useItemEntityType(t *testing.T) EntityTypeSummary {
	t.Helper()
	et, err := tRepos.EntityTypes.GetDefault(context.Background(), tGroup.ID, false)
	require.NoError(t, err)
	return et
}

func useEntities(t *testing.T, count int) []EntityOut {
	t.Helper()

	containerET := useContainerEntityType(t)
	itemET := useItemEntityType(t)

	// Create a container entity
	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)

	entities := make([]EntityOut, count)
	for i := 0; i < count; i++ {
		itm := entityFactory()
		itm.ParentID = container.ID
		itm.EntityTypeID = itemET.ID

		e, err := tRepos.Entities.Create(context.Background(), tGroup.ID, itm)
		require.NoError(t, err)
		entities[i] = e
	}

	t.Cleanup(func() {
		for _, e := range entities {
			_ = tRepos.Entities.Delete(context.Background(), e.ID)
		}
		_ = tRepos.Entities.Delete(context.Background(), container.ID)
	})

	return entities
}

func TestEntityRepository_RecursiveRelationships(t *testing.T) {
	parent := useEntities(t, 1)[0]

	children := useEntities(t, 3)

	for _, child := range children {
		update := EntityUpdate{
			ID:          child.ID,
			ParentID:    parent.ID,
			Name:        "note-important",
			Description: "This is a note",
		}
		if child.EntityType != nil {
			update.EntityTypeID = child.EntityType.ID
		}

		// Append Parent ID
		_, err := tRepos.Entities.UpdateByGroup(context.Background(), tGroup.ID, update)
		require.NoError(t, err)

		// Check Parent ID
		updated, err := tRepos.Entities.GetOne(context.Background(), child.ID)
		require.NoError(t, err)
		assert.Equal(t, parent.ID, updated.Parent.ID)

		// Remove Parent ID
		update.ParentID = uuid.Nil
		_, err = tRepos.Entities.UpdateByGroup(context.Background(), tGroup.ID, update)
		require.NoError(t, err)

		// Check Parent ID
		updated, err = tRepos.Entities.GetOne(context.Background(), child.ID)
		require.NoError(t, err)
		assert.Nil(t, updated.Parent)
	}
}

func TestEntityRepository_GetOne(t *testing.T) {
	entities := useEntities(t, 3)

	for _, e := range entities {
		result, err := tRepos.Entities.GetOne(context.Background(), e.ID)
		require.NoError(t, err)
		assert.Equal(t, e.ID, result.ID)
	}
}

func TestEntityRepository_GetAll(t *testing.T) {
	length := 10
	expected := useEntities(t, length)

	results, err := tRepos.Entities.GetAll(context.Background(), tGroup.ID)
	require.NoError(t, err)

	// Results include the container + the items
	assert.GreaterOrEqual(t, len(results), length)

	for _, e := range expected {
		found := false
		for _, r := range results {
			if e.ID == r.ID {
				found = true
				assert.Equal(t, e.Name, r.Name)
				assert.Equal(t, e.Description, r.Description)
			}
		}
		assert.True(t, found, "expected entity not found in results")
	}
}

func TestEntityRepository_Create(t *testing.T) {
	containerET := useContainerEntityType(t)
	itemET := useItemEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)

	itm := entityFactory()
	itm.ParentID = container.ID
	itm.EntityTypeID = itemET.ID

	result, err := tRepos.Entities.Create(context.Background(), tGroup.ID, itm)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)

	// Cleanup
	err = tRepos.Entities.Delete(context.Background(), result.ID)
	require.NoError(t, err)
	err = tRepos.Entities.Delete(context.Background(), container.ID)
	require.NoError(t, err)
}

func TestEntityRepository_Create_WithFractionalQuantity(t *testing.T) {
	containerET := useContainerEntityType(t)
	itemET := useItemEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)

	itm := entityFactory()
	itm.ParentID = container.ID
	itm.EntityTypeID = itemET.ID
	itm.Quantity = 1.25

	result, err := tRepos.Entities.Create(context.Background(), tGroup.ID, itm)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)
	assert.InDelta(t, 1.25, result.Quantity, 0.000001)

	fetched, err := tRepos.Entities.GetOne(context.Background(), result.ID)
	require.NoError(t, err)
	assert.InDelta(t, 1.25, fetched.Quantity, 0.000001)

	// Cleanup
	err = tRepos.Entities.Delete(context.Background(), result.ID)
	require.NoError(t, err)
	err = tRepos.Entities.Delete(context.Background(), container.ID)
	require.NoError(t, err)
}

func TestEntityRepository_QueryByGroup_FilteredInventoryValueIgnoresPagination(t *testing.T) {
	ctx := context.Background()
	itemType := useItemEntityType(t)
	prefix := "inventory-filter-" + uuid.NewString()
	createdIDs := make([]uuid.UUID, 0, 6)
	t.Cleanup(func() {
		for _, id := range createdIDs {
			_ = tRepos.Entities.Delete(ctx, id)
		}
	})

	createPriced := func(suffix string, quantity, price float64) EntityOut {
		t.Helper()
		out, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
			Name:         prefix + "-" + suffix,
			Quantity:     quantity,
			EntityTypeID: itemType.ID,
		})
		require.NoError(t, err)
		createdIDs = append(createdIDs, out.ID)
		require.NoError(t, tClient.Entity.UpdateOneID(out.ID).SetPurchasePrice(price).Exec(ctx))
		return out
	}

	createPriced("active-a", 2, 10)
	createPriced("active-b", 3, 5)

	sold := createPriced("sold", 4, 100)
	require.NoError(t, tClient.Entity.UpdateOneID(sold.ID).SetSoldDate(time.Now()).Exec(ctx))

	archived := createPriced("archived", 1, 999)
	require.NoError(t, tClient.Entity.UpdateOneID(archived.ID).SetArchived(true).Exec(ctx))

	// A matching entity with a legacy cross-tenant type edge must be excluded
	// from both the visible query and its aggregate.
	_, _, foreignTypeID := makeForeignGroup(t)
	corrupt := createPriced("foreign-type", 1, 777)
	require.NoError(t, tClient.Entity.UpdateOneID(corrupt.ID).SetEntityTypeID(foreignTypeID).Exec(ctx))

	createPriced("other-search", 1, 123)

	result, err := tRepos.Entities.QueryByGroup(ctx, tGroup.ID, EntityQuery{
		Page:                  1,
		PageSize:              1,
		Search:                prefix + "-active",
		IncludeInventoryValue: true,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, 2, result.Total)
	assert.InDelta(t, 35, result.FilteredInventoryValue, 0.001,
		"the total must include every filtered row, not only the current page")

	result, err = tRepos.Entities.QueryByGroup(ctx, tGroup.ID, EntityQuery{
		Page:                  1,
		PageSize:              1,
		Search:                prefix,
		IncludeArchived:       true,
		IncludeInventoryValue: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 5, result.Total, "active, sold, archived, and unrelated-suffix rows use the search filter; corrupt type is excluded")
	assert.InDelta(t, 158, result.FilteredInventoryValue, 0.001,
		"archived and sold rows remain excluded even when the list includes archived entities")
}

func TestEntityRepository_PaginationReturnsFullFilteredCount(t *testing.T) {
	ctx := context.Background()
	prefix := "pagination-" + uuid.NewString()
	itemType := useItemEntityType(t)
	created := make([]EntityOut, 0, 3)

	for _, suffix := range []string{"a", "b", "c"} {
		out, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
			Name:         prefix + "-" + suffix,
			EntityTypeID: itemType.ID,
		})
		require.NoError(t, err)
		created = append(created, out)
	}
	t.Cleanup(func() {
		for _, out := range created {
			_ = tRepos.Entities.Delete(context.Background(), out.ID)
		}
	})

	first, err := tRepos.Entities.QueryByGroup(ctx, tGroup.ID, EntityQuery{
		Page:     1,
		PageSize: 2,
		Search:   prefix,
	})
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	assert.Equal(t, 3, first.Total)

	second, err := tRepos.Entities.QueryByGroup(ctx, tGroup.ID, EntityQuery{
		Page:     2,
		PageSize: 2,
		Search:   prefix,
	})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, 3, second.Total)

	all, err := tRepos.Entities.QueryByGroup(ctx, tGroup.ID, EntityQuery{
		Page:     -1,
		PageSize: -1,
		Search:   prefix,
	})
	require.NoError(t, err)
	require.Len(t, all.Items, 3)
	assert.Equal(t, 3, all.Total)

	assetID := AssetID(time.Now().UnixNano())
	for _, out := range created {
		require.NoError(t, tClient.Entity.UpdateOneID(out.ID).SetAssetID(int64(assetID)).Exec(ctx))
	}

	assetPage, err := tRepos.Entities.QueryByAssetID(ctx, tGroup.ID, assetID, 1, 2)
	require.NoError(t, err)
	require.Len(t, assetPage.Items, 2)
	assert.Equal(t, 3, assetPage.Total)
}

func TestEntityRepository_Create_RejectsNonFiniteQuantity(t *testing.T) {
	containerET := useContainerEntityType(t)
	itemET := useItemEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)

	itm := entityFactory()
	itm.ParentID = container.ID
	itm.EntityTypeID = itemET.ID
	itm.Quantity = math.NaN()

	_, err = tRepos.Entities.Create(context.Background(), tGroup.ID, itm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid quantity: must be a finite number")

	// Cleanup
	err = tRepos.Entities.Delete(context.Background(), container.ID)
	require.NoError(t, err)
}

func TestEntityRepository_Update_RollsBackEntityAndTagsWhenFieldWriteFails(t *testing.T) {
	ctx := context.Background()
	itemET := useItemEntityType(t)
	tags := useTags(t, 1)

	created, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "atomic-update-original",
		Quantity:     1,
		EntityTypeID: itemET.ID,
		TagIDs:       []uuid.UUID{tags[0].ID},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), created.ID) })

	_, err = tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, EntityUpdate{
		ID:           created.ID,
		Name:         "must-roll-back",
		Quantity:     2,
		EntityTypeID: itemET.ID,
		TagIDs:       []uuid.UUID{},
		Fields: []EntityFieldData{{
			Type: "not-a-valid-field-type",
			Name: "invalid",
		}},
	})
	require.Error(t, err)

	got, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "atomic-update-original", got.Name)
	assert.InDelta(t, 1, got.Quantity, 0.000001)
	require.Len(t, got.Tags, 1)
	assert.Equal(t, tags[0].ID, got.Tags[0].ID)
	assert.Empty(t, got.Fields)
}

func TestEntityRepository_Duplicate_PreservesTypedCustomFields(t *testing.T) {
	ctx := context.Background()
	itemType := useItemEntityType(t)
	source, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "typed-duplicate-source",
		Quantity:     1,
		EntityTypeID: itemType.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(ctx, source.ID) })

	timeValue := time.Date(2026, time.June, 7, 8, 9, 10, 0, time.UTC)
	source, err = tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, EntityUpdate{
		ID:           source.ID,
		Name:         source.Name,
		Quantity:     1,
		EntityTypeID: itemType.ID,
		Fields: []EntityFieldData{
			{Type: "text", Name: "text", TextValue: "duplicate-me"},
			{Type: testFieldTypeNumber, Name: "number", NumberValue: 91},
			{Type: testFieldTypeBoolean, Name: "boolean", BooleanValue: true},
			{Type: "time", Name: "time", TimeValue: timeValue},
		},
	})
	require.NoError(t, err)

	duplicate, err := tRepos.Entities.Duplicate(ctx, tGroup.ID, source.ID, DuplicateOptions{
		CopyCustomFields: true,
		CopyPrefix:       "typed-copy-",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(ctx, duplicate.ID) })
	require.Len(t, duplicate.Fields, 4)

	byName := lo.SliceToMap(duplicate.Fields, func(field EntityFieldData) (string, EntityFieldData) {
		return field.Name, field
	})
	assert.Equal(t, "duplicate-me", byName["text"].TextValue)
	assert.Equal(t, 91, byName["number"].NumberValue)
	assert.True(t, byName["boolean"].BooleanValue)
	assert.WithinDuration(t, timeValue, byName["time"].TimeValue, time.Second)
}

func TestEntityRepository_Duplicate_RollsBackWhenRequestedCopyFails(t *testing.T) {
	ctx := context.Background()
	itemType := useItemEntityType(t)
	source, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "duplicate-rollback-source",
		Quantity:     1,
		EntityTypeID: itemType.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(ctx, source.ID) })

	_, err = tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, EntityUpdate{
		ID:           source.ID,
		Name:         source.Name,
		Quantity:     1,
		EntityTypeID: itemType.ID,
		Fields: []EntityFieldData{{
			Type:      "text",
			Name:      "duplicate-rollback-sentinel",
			TextValue: "source survives",
		}},
	})
	require.NoError(t, err)

	_, err = tClient.Sql().ExecContext(ctx, `
		CREATE TRIGGER duplicate_field_failure
		BEFORE INSERT ON entity_fields
		WHEN NEW.name = 'duplicate-rollback-sentinel'
		BEGIN
			SELECT RAISE(ABORT, 'forced duplicate field failure');
		END`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = tClient.Sql().ExecContext(context.Background(), `DROP TRIGGER IF EXISTS duplicate_field_failure`)
	})

	copyPrefix := "duplicate-must-not-persist-" + uuid.NewString()
	_, err = tRepos.Entities.Duplicate(ctx, tGroup.ID, source.ID, DuplicateOptions{
		CopyCustomFields: true,
		CopyPrefix:       copyPrefix,
	})
	require.Error(t, err)

	count, err := tClient.Entity.Query().
		Where(
			entity.HasGroupWith(group.ID(tGroup.ID)),
			entity.NameHasPrefix(copyPrefix),
		).
		Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count, "the duplicate entity must roll back when a requested child copy fails")

	got, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, source.ID)
	require.NoError(t, err)
	require.Len(t, got.Fields, 1)
	assert.Equal(t, "source survives", got.Fields[0].TextValue)
}

func TestEntityRepository_RejectsHierarchyCycles(t *testing.T) {
	ctx := context.Background()
	itemET := useItemEntityType(t)
	containerET := useContainerEntityType(t)

	create := func(name string, entityTypeID, parentID uuid.UUID) EntityOut {
		t.Helper()
		out, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
			Name:         name,
			Quantity:     1,
			EntityTypeID: entityTypeID,
			ParentID:     parentID,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), out.ID) })
		return out
	}

	root := create("cycle-root", itemET.ID, uuid.Nil)
	child := create("cycle-child", itemET.ID, root.ID)
	grandchild := create("cycle-grandchild", itemET.ID, child.ID)

	_, err := tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, EntityUpdate{
		ID:           root.ID,
		Name:         root.Name,
		Quantity:     1,
		EntityTypeID: itemET.ID,
		ParentID:     grandchild.ID,
	})
	require.ErrorContains(t, err, "hierarchy cycle")

	err = tRepos.Entities.Patch(ctx, tGroup.ID, child.ID, EntityPatch{
		ParentID: child.ID,
	})
	require.ErrorContains(t, err, "hierarchy cycle")

	rootContainer := create("container-cycle-root", containerET.ID, uuid.Nil)
	childContainer := create("container-cycle-child", containerET.ID, rootContainer.ID)
	_, err = tRepos.Entities.UpdateContainer(ctx, tGroup.ID, rootContainer.ID, EntityUpdate{
		Name:     rootContainer.Name,
		ParentID: childContainer.ID,
	})
	require.ErrorContains(t, err, "hierarchy cycle")

	gotRoot, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, root.ID)
	require.NoError(t, err)
	assert.Nil(t, gotRoot.Parent)
	gotChild, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, child.ID)
	require.NoError(t, err)
	require.NotNil(t, gotChild.Parent)
	assert.Equal(t, root.ID, gotChild.Parent.ID)
	gotContainerRoot, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, rootContainer.ID)
	require.NoError(t, err)
	assert.Nil(t, gotContainerRoot.Parent)
}

func TestEntityRepository_Create_WithParent(t *testing.T) {
	containerET := useContainerEntityType(t)
	itemET := useItemEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)
	assert.NotEmpty(t, container.ID)

	itm := entityFactory()
	itm.ParentID = container.ID
	itm.EntityTypeID = itemET.ID

	// Create Resource
	result, err := tRepos.Entities.Create(context.Background(), tGroup.ID, itm)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)

	// Get Resource
	foundEntity, err := tRepos.Entities.GetOne(context.Background(), result.ID)
	require.NoError(t, err)
	assert.Equal(t, result.ID, foundEntity.ID)
	assert.NotNil(t, foundEntity.Parent)
	assert.Equal(t, container.ID, foundEntity.Parent.ID)

	// Cleanup
	err = tRepos.Entities.Delete(context.Background(), result.ID)
	require.NoError(t, err)
	err = tRepos.Entities.Delete(context.Background(), container.ID)
	require.NoError(t, err)
}

func TestEntityRepository_Delete(t *testing.T) {
	entities := useEntities(t, 3)

	for _, e := range entities {
		err := tRepos.Entities.Delete(context.Background(), e.ID)
		require.NoError(t, err)
	}

	results, err := tRepos.Entities.GetAll(context.Background(), tGroup.ID)
	require.NoError(t, err)
	// After deleting items, only container(s) remain
	for _, e := range entities {
		for _, r := range results {
			assert.NotEqual(t, e.ID, r.ID)
		}
	}
}

func TestEntityRepository_Update_Tags(t *testing.T) {
	e := useEntities(t, 1)[0]
	tags := useTags(t, 3)

	tagsIDs := []uuid.UUID{tags[0].ID, tags[1].ID, tags[2].ID}

	type args struct {
		tagIds []uuid.UUID
	}

	tests := []struct {
		name string
		args args
		want []uuid.UUID
	}{
		{
			name: "add all tags",
			args: args{
				tagIds: tagsIDs,
			},
			want: tagsIDs,
		},
		{
			name: "update with one tag",
			args: args{
				tagIds: tagsIDs[:1],
			},
			want: tagsIDs[:1],
		},
		{
			name: "add one new tag to existing single tag",
			args: args{
				tagIds: tagsIDs[1:],
			},
			want: tagsIDs[1:],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateData := EntityUpdate{
				ID:     e.ID,
				Name:   e.Name,
				TagIDs: tt.args.tagIds,
			}
			if e.EntityType != nil {
				updateData.EntityTypeID = e.EntityType.ID
			}

			updated, err := tRepos.Entities.UpdateByGroup(context.Background(), tGroup.ID, updateData)
			require.NoError(t, err)
			assert.Len(t, tt.want, len(updated.Tags))

			for _, tag := range updated.Tags {
				assert.Contains(t, tt.want, tag.ID)
			}
		})
	}
}

func TestEntityRepository_Update(t *testing.T) {
	entities := useEntities(t, 3)

	e := entities[0]

	updateData := EntityUpdate{
		ID:               e.ID,
		Name:             e.Name,
		SerialNumber:     fk.Str(10),
		TagIDs:           nil,
		ModelNumber:      fk.Str(10),
		Manufacturer:     fk.Str(10),
		PurchaseDate:     types.DateFromTime(time.Now()),
		PurchaseFrom:     fk.Str(10),
		PurchasePrice:    300.99,
		SoldDate:         types.DateFromTime(time.Now()),
		SoldTo:           fk.Str(10),
		SoldPrice:        300.99,
		SoldNotes:        fk.Str(10),
		Notes:            fk.Str(10),
		WarrantyExpires:  types.DateFromTime(time.Now()),
		WarrantyDetails:  fk.Str(10),
		LifetimeWarranty: true,
	}
	if e.EntityType != nil {
		updateData.EntityTypeID = e.EntityType.ID
	}

	updatedEntity, err := tRepos.Entities.UpdateByGroup(context.Background(), tGroup.ID, updateData)
	require.NoError(t, err)

	got, err := tRepos.Entities.GetOne(context.Background(), updatedEntity.ID)
	require.NoError(t, err)

	assert.Equal(t, updateData.ID, got.ID)
	assert.Equal(t, updateData.Name, got.Name)
	assert.Equal(t, updateData.SerialNumber, got.SerialNumber)
	assert.Equal(t, updateData.ModelNumber, got.ModelNumber)
	assert.Equal(t, updateData.Manufacturer, got.Manufacturer)
	assert.Equal(t, updateData.PurchaseFrom, got.PurchaseFrom)
	assert.InDelta(t, updateData.PurchasePrice, got.PurchasePrice, 0.01)
	assert.Equal(t, updateData.SoldTo, got.SoldTo)
	assert.InDelta(t, updateData.SoldPrice, got.SoldPrice, 0.01)
	assert.Equal(t, updateData.SoldNotes, got.SoldNotes)
	assert.Equal(t, updateData.Notes, got.Notes)
	assert.Equal(t, updateData.WarrantyDetails, got.WarrantyDetails)
	assert.Equal(t, updateData.LifetimeWarranty, got.LifetimeWarranty)
}

func TestEntityRepository_Update_WithFractionalQuantity(t *testing.T) {
	e := useEntities(t, 1)[0]

	updateData := EntityUpdate{
		ID:       e.ID,
		Name:     e.Name,
		Quantity: 2.75,
	}
	if e.EntityType != nil {
		updateData.EntityTypeID = e.EntityType.ID
	}

	updatedEntity, err := tRepos.Entities.UpdateByGroup(context.Background(), tGroup.ID, updateData)
	require.NoError(t, err)

	got, err := tRepos.Entities.GetOne(context.Background(), updatedEntity.ID)
	require.NoError(t, err)

	assert.InDelta(t, 2.75, got.Quantity, 0.000001)
}

func TestEntityRepository_Update_RejectsNonFiniteQuantity(t *testing.T) {
	e := useEntities(t, 1)[0]

	updateData := EntityUpdate{
		ID:       e.ID,
		Name:     e.Name,
		Quantity: math.Inf(1),
	}

	_, err := tRepos.Entities.UpdateByGroup(context.Background(), tGroup.ID, updateData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid quantity: must be a finite number")
}

func TestEntityRepository_Patch_RejectsNonFiniteQuantity(t *testing.T) {
	e := useEntities(t, 1)[0]

	quantity := math.Inf(-1)
	err := tRepos.Entities.Patch(context.Background(), tGroup.ID, e.ID, EntityPatch{Quantity: &quantity})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid quantity: must be a finite number")
}

func TestEntityRepository_CreateFromTemplate_RejectsNonFiniteQuantity(t *testing.T) {
	containerET := useContainerEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)

	_, err = tRepos.Entities.CreateFromTemplate(context.Background(), tGroup.ID, EntityCreateFromTemplate{
		Name:        fk.Str(10),
		Description: fk.Str(20),
		Quantity:    math.NaN(),
		ParentID:    container.ID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid quantity: must be a finite number")

	// Cleanup
	err = tRepos.Entities.Delete(context.Background(), container.ID)
	require.NoError(t, err)
}

// TestEntityRepository_CreateFromTemplate_DefaultsEntityType verifies that
// creating an entity from a template without an explicit entity type still
// succeeds and resolves the group's default item type, rather than failing the
// required entity_type edge (regression test for #1548).
func TestEntityRepository_CreateFromTemplate_DefaultsEntityType(t *testing.T) {
	containerET := useContainerEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)

	out, err := tRepos.Entities.CreateFromTemplate(context.Background(), tGroup.ID, EntityCreateFromTemplate{
		Name:        fk.Str(10),
		Description: fk.Str(20),
		Quantity:    1,
		ParentID:    container.ID,
		// EntityTypeID intentionally left zero to exercise the fallback.
	})
	require.NoError(t, err)
	require.NotNil(t, out.EntityType)
	assert.False(t, out.EntityType.IsLocation)

	// Cleanup
	err = tRepos.Entities.Delete(context.Background(), out.ID)
	require.NoError(t, err)
	err = tRepos.Entities.Delete(context.Background(), container.ID)
	require.NoError(t, err)
}

// TestEntityRepository_CreateFromTemplate_UsesSelectedEntityType verifies that
// the user-selected entity type takes precedence over the default fallback.
func TestEntityRepository_CreateFromTemplate_UsesSelectedEntityType(t *testing.T) {
	containerET := useContainerEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)

	selectedET, err := tRepos.EntityTypes.Create(context.Background(), tGroup.ID, EntityTypeCreate{
		Name: fk.Str(10),
	})
	require.NoError(t, err)

	out, err := tRepos.Entities.CreateFromTemplate(context.Background(), tGroup.ID, EntityCreateFromTemplate{
		Name:         fk.Str(10),
		Description:  fk.Str(20),
		Quantity:     1,
		ParentID:     container.ID,
		EntityTypeID: selectedET.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, out.EntityType)
	assert.Equal(t, selectedET.ID, out.EntityType.ID)

	// Cleanup
	err = tRepos.Entities.Delete(context.Background(), out.ID)
	require.NoError(t, err)
	err = tRepos.Entities.Delete(context.Background(), container.ID)
	require.NoError(t, err)
}

func TestEntityRepository_GetAllCustomFields(t *testing.T) {
	const FieldsCount = 5

	e := useEntities(t, 1)[0]

	fields := make([]EntityFieldData, FieldsCount)
	names := make([]string, FieldsCount)
	values := make([]string, FieldsCount)

	for i := 0; i < FieldsCount; i++ {
		name := fk.Str(10)
		fields[i] = EntityFieldData{
			Name:      name,
			Type:      "text",
			TextValue: fk.Str(10),
		}
		names[i] = name
		values[i] = fields[i].TextValue
	}

	updateData := EntityUpdate{
		ID:     e.ID,
		Name:   e.Name,
		Fields: fields,
	}
	if e.EntityType != nil {
		updateData.EntityTypeID = e.EntityType.ID
	}

	_, err := tRepos.Entities.UpdateByGroup(context.Background(), tGroup.ID, updateData)
	require.NoError(t, err)

	// Test getting all fields
	{
		results, err := tRepos.Entities.GetAllCustomFieldNames(context.Background(), tGroup.ID)
		require.NoError(t, err)
		assert.ElementsMatch(t, names, results)
	}

	// Test getting all values from field
	{
		results, err := tRepos.Entities.GetAllCustomFieldValues(context.Background(), tUser.DefaultGroupID, names[0])

		require.NoError(t, err)
		assert.ElementsMatch(t, values[:1], results)
	}
}

func TestEntityRepository_DeleteWithAttachments(t *testing.T) {
	// Create an entity with an attachment
	e := useEntities(t, 1)[0]

	// Add an attachment to the entity
	att, err := tRepos.Attachments.Create(
		context.Background(),
		e.ID,
		ItemCreateAttachment{
			Title:   "test-attachment.txt",
			Content: strings.NewReader("test content for attachment deletion"),
		},
		attachment.TypePhoto,
		true,
	)
	require.NoError(t, err)
	assert.NotNil(t, att)

	// Verify the attachment exists
	retrievedAttachment, err := tRepos.Attachments.Get(context.Background(), tGroup.ID, att.ID)
	require.NoError(t, err)
	assert.Equal(t, att.ID, retrievedAttachment.ID)

	// Verify the attachment is linked to the entity
	entityWithAttachments, err := tRepos.Entities.GetOne(context.Background(), e.ID)
	require.NoError(t, err)
	assert.Len(t, entityWithAttachments.Attachments, 1)
	assert.Equal(t, att.ID, entityWithAttachments.Attachments[0].ID)

	// Delete the entity
	err = tRepos.Entities.Delete(context.Background(), e.ID)
	require.NoError(t, err)

	// Verify the entity is deleted
	_, err = tRepos.Entities.GetOne(context.Background(), e.ID)
	require.Error(t, err)

	// Verify the attachment is also deleted
	_, err = tRepos.Attachments.Get(context.Background(), tGroup.ID, att.ID)
	require.Error(t, err)
}

func TestEntityRepository_DeleteByGroupWithAttachments(t *testing.T) {
	// Create an entity with an attachment
	e := useEntities(t, 1)[0]

	// Add an attachment to the entity
	att, err := tRepos.Attachments.Create(
		context.Background(),
		e.ID,
		ItemCreateAttachment{
			Title:   "test-attachment-by-group.txt",
			Content: strings.NewReader("test content for attachment deletion by group"),
		},
		attachment.TypePhoto,
		true,
	)
	require.NoError(t, err)
	assert.NotNil(t, att)

	// Verify the attachment exists
	retrievedAttachment, err := tRepos.Attachments.Get(context.Background(), tGroup.ID, att.ID)
	require.NoError(t, err)
	assert.Equal(t, att.ID, retrievedAttachment.ID)

	// Delete the entity using DeleteByGroup
	err = tRepos.Entities.DeleteByGroup(context.Background(), tGroup.ID, e.ID)
	require.NoError(t, err)

	// Verify the entity is deleted
	_, err = tRepos.Entities.GetOneByGroup(context.Background(), tGroup.ID, e.ID)
	require.Error(t, err)

	// Verify the attachment is also deleted
	_, err = tRepos.Attachments.Get(context.Background(), tGroup.ID, att.ID)
	require.Error(t, err)
}

func TestEntityRepository_WipeInventory(t *testing.T) {
	containerET := useContainerEntityType(t)
	itemET := useItemEntityType(t)

	// Create containers
	c1f := containerFactory()
	c1f.EntityTypeID = containerET.ID
	c1f.Name = "Test Container 1"
	c1f.Description = "Test container for wipe test"
	container1, err := tRepos.Entities.Create(context.Background(), tGroup.ID, c1f)
	require.NoError(t, err)

	c2f := containerFactory()
	c2f.EntityTypeID = containerET.ID
	c2f.Name = "Test Container 2"
	c2f.Description = "Another test container"
	container2, err := tRepos.Entities.Create(context.Background(), tGroup.ID, c2f)
	require.NoError(t, err)

	// Create tags
	tag1, err := tRepos.Tags.Create(context.Background(), tGroup.ID, TagCreate{
		Name:        "Test Tag 1",
		Description: "Test tag for wipe test",
	})
	require.NoError(t, err)

	tag2, err := tRepos.Tags.Create(context.Background(), tGroup.ID, TagCreate{
		Name:        "Test Tag 2",
		Description: "Another test tag",
	})
	require.NoError(t, err)

	// Create items
	i1f := entityFactory()
	i1f.ParentID = container1.ID
	i1f.EntityTypeID = itemET.ID
	i1f.Name = "Test Item 1"
	i1f.Description = "Test item for wipe test"
	i1f.TagIDs = []uuid.UUID{tag1.ID}
	entity1, err := tRepos.Entities.Create(context.Background(), tGroup.ID, i1f)
	require.NoError(t, err)

	i2f := entityFactory()
	i2f.ParentID = container2.ID
	i2f.EntityTypeID = itemET.ID
	i2f.Name = "Test Item 2"
	i2f.Description = "Another test item"
	i2f.TagIDs = []uuid.UUID{tag2.ID}
	entity2, err := tRepos.Entities.Create(context.Background(), tGroup.ID, i2f)
	require.NoError(t, err)

	// Create maintenance entries
	_, err = tRepos.MaintEntry.Create(context.Background(), tGroup.ID, entity1.ID, MaintenanceEntryCreate{
		CompletedDate: types.DateFromTime(time.Now()),
		Name:          "Test Maintenance 1",
		Description:   "Test maintenance entry",
		Cost:          100.0,
	})
	require.NoError(t, err)

	_, err = tRepos.MaintEntry.Create(context.Background(), tGroup.ID, entity2.ID, MaintenanceEntryCreate{
		CompletedDate: types.DateFromTime(time.Now()),
		Name:          "Test Maintenance 2",
		Description:   "Another test maintenance entry",
		Cost:          200.0,
	})
	require.NoError(t, err)

	// Test: Wipe inventory with all options enabled
	t.Run("wipe all including tags, containers, and maintenance", func(t *testing.T) {
		deleted, err := tRepos.Entities.WipeInventory(context.Background(), tGroup.ID, true, true, true)
		require.NoError(t, err)
		assert.Positive(t, deleted, "Should have deleted at least some entities")

		// Verify items are deleted
		_, err = tRepos.Entities.GetOneByGroup(context.Background(), tGroup.ID, entity1.ID)
		require.Error(t, err, "Entity 1 should be deleted")

		_, err = tRepos.Entities.GetOneByGroup(context.Background(), tGroup.ID, entity2.ID)
		require.Error(t, err, "Entity 2 should be deleted")

		// Verify maintenance entries are deleted
		maint1List, err := tRepos.MaintEntry.GetMaintenanceByItemID(context.Background(), tGroup.ID, entity1.ID, MaintenanceFilters{})
		require.NoError(t, err)
		assert.Empty(t, maint1List, "Maintenance entry 1 should be deleted")

		maint2List, err := tRepos.MaintEntry.GetMaintenanceByItemID(context.Background(), tGroup.ID, entity2.ID, MaintenanceFilters{})
		require.NoError(t, err)
		assert.Empty(t, maint2List, "Maintenance entry 2 should be deleted")

		// Verify tags are deleted
		_, err = tRepos.Tags.GetOneByGroup(context.Background(), tGroup.ID, tag1.ID)
		require.Error(t, err, "Tag 1 should be deleted")

		_, err = tRepos.Tags.GetOneByGroup(context.Background(), tGroup.ID, tag2.ID)
		require.Error(t, err, "Tag 2 should be deleted")
	})
}

func TestEntityRepository_WipeInventory_OnlyItems(t *testing.T) {
	containerET := useContainerEntityType(t)
	itemET := useItemEntityType(t)

	// Create test data
	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	cf.Name = "Test Container"
	cf.Description = "Test container for wipe test"
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)

	tagObj, err := tRepos.Tags.Create(context.Background(), tGroup.ID, TagCreate{
		Name:        "Test Tag",
		Description: "Test tag for wipe test",
	})
	require.NoError(t, err)

	ef := entityFactory()
	ef.ParentID = container.ID
	ef.EntityTypeID = itemET.ID
	ef.Name = "Test Item"
	ef.Description = "Test item for wipe test"
	ef.TagIDs = []uuid.UUID{tagObj.ID}
	e, err := tRepos.Entities.Create(context.Background(), tGroup.ID, ef)
	require.NoError(t, err)

	_, err = tRepos.MaintEntry.Create(context.Background(), tGroup.ID, e.ID, MaintenanceEntryCreate{
		CompletedDate: types.DateFromTime(time.Now()),
		Name:          "Test Maintenance",
		Description:   "Test maintenance entry",
		Cost:          100.0,
	})
	require.NoError(t, err)

	// Test: Wipe inventory with only items (no tags, containers, or maintenance)
	deleted, err := tRepos.Entities.WipeInventory(context.Background(), tGroup.ID, false, false, false)
	require.NoError(t, err)
	assert.Positive(t, deleted, "Should have deleted at least the entity")

	// Verify item entity is deleted
	_, err = tRepos.Entities.GetOneByGroup(context.Background(), tGroup.ID, e.ID)
	require.Error(t, err, "Entity should be deleted")

	// Verify maintenance entry is deleted due to cascade
	maintList, err := tRepos.MaintEntry.GetMaintenanceByItemID(context.Background(), tGroup.ID, e.ID, MaintenanceFilters{})
	require.NoError(t, err)
	assert.Empty(t, maintList, "Maintenance entry should be cascade deleted with entity")

	// Verify tag still exists
	_, err = tRepos.Tags.GetOneByGroup(context.Background(), tGroup.ID, tagObj.ID)
	require.NoError(t, err, "Tag should still exist")

	// Containers are retained unless wipeContainers is explicitly requested.
	_, err = tRepos.Entities.GetOneByGroup(context.Background(), tGroup.ID, container.ID)
	require.NoError(t, err, "Container should still exist")

	// Cleanup
	_ = tRepos.Entities.Delete(context.Background(), container.ID)
	_ = tRepos.Tags.DeleteByGroup(context.Background(), tGroup.ID, tagObj.ID)
}

func TestEntityRepository_Create_PersistsIdentifications(t *testing.T) {
	itemET := useItemEntityType(t)

	itm := entityFactory()
	itm.EntityTypeID = itemET.ID
	itm.Manufacturer = "DeWalt"
	itm.ModelNumber = "DCD771"

	result, err := tRepos.Entities.Create(context.Background(), tGroup.ID, itm)
	require.NoError(t, err)
	assert.Equal(t, "DeWalt", result.Manufacturer)
	assert.Equal(t, "DCD771", result.ModelNumber)

	// Cleanup
	err = tRepos.Entities.Delete(context.Background(), result.ID)
	require.NoError(t, err)
}

func TestEntityRepository_IconPersistsThroughCreateAndUpdate(t *testing.T) {
	itemET := useItemEntityType(t)

	itm := entityFactory()
	itm.EntityTypeID = itemET.ID
	itm.Icon = "basket-outline"

	result, err := tRepos.Entities.Create(context.Background(), tGroup.ID, itm)
	require.NoError(t, err)
	assert.Equal(t, "basket-outline", result.Icon)

	// Update changes it
	upd := EntityUpdate{
		ID:           result.ID,
		Name:         result.Name,
		Description:  result.Description,
		Quantity:     result.Quantity,
		EntityTypeID: itemET.ID,
		Icon:         "toolbox-outline",
	}
	updated, err := tRepos.Entities.UpdateByGroup(context.Background(), tGroup.ID, upd)
	require.NoError(t, err)
	assert.Equal(t, "toolbox-outline", updated.Icon)

	// Cleanup
	err = tRepos.Entities.Delete(context.Background(), result.ID)
	require.NoError(t, err)
}

func TestEntityRepository_Tree_CarriesIconFields(t *testing.T) {
	ctx := context.Background()

	et, err := tRepos.EntityTypes.Create(ctx, tGroup.ID, EntityTypeCreate{
		Name: "Iconic Tote", IsLocation: true, IsContainer: true, Icon: "basket-outline",
	})
	require.NoError(t, err)

	withOverride := entityFactory()
	withOverride.EntityTypeID = et.ID
	withOverride.Icon = "treasure-chest"
	a, err := tRepos.Entities.Create(ctx, tGroup.ID, withOverride)
	require.NoError(t, err)

	noOverride := entityFactory()
	noOverride.EntityTypeID = et.ID
	b, err := tRepos.Entities.Create(ctx, tGroup.ID, noOverride)
	require.NoError(t, err)

	tree, err := tRepos.Entities.Tree(ctx, tGroup.ID, TreeQuery{WithItems: false})
	require.NoError(t, err)

	found := map[uuid.UUID]TreeItem{}
	var walk func(items []*TreeItem)
	walk = func(items []*TreeItem) {
		for _, it := range items {
			found[it.ID] = *it
			walk(it.Children)
		}
	}
	// Tree() returns []TreeItem.
	for i := range tree {
		found[tree[i].ID] = tree[i]
		walk(tree[i].Children)
	}

	require.Contains(t, found, a.ID)
	assert.Equal(t, "treasure-chest", found[a.ID].Icon)
	assert.Equal(t, "basket-outline", found[a.ID].TypeIcon)
	assert.True(t, found[a.ID].IsContainer)

	require.Contains(t, found, b.ID)
	assert.Empty(t, found[b.ID].Icon)
	assert.Equal(t, "basket-outline", found[b.ID].TypeIcon)
	assert.True(t, found[b.ID].IsContainer)

	// WithItems=true path: a non-location item nested under location `a` must also
	// carry its own icon fields through the item_tree UNION arm.
	itemET, err := tRepos.EntityTypes.Create(ctx, tGroup.ID, EntityTypeCreate{
		Name: "Iconic Widget", IsLocation: false, IsContainer: false, Icon: "widget-outline",
	})
	require.NoError(t, err)

	nestedItem := entityFactory()
	nestedItem.EntityTypeID = itemET.ID
	nestedItem.ParentID = a.ID
	nestedItem.Icon = "gem-outline"
	c, err := tRepos.Entities.Create(ctx, tGroup.ID, nestedItem)
	require.NoError(t, err)

	treeWithItems, err := tRepos.Entities.Tree(ctx, tGroup.ID, TreeQuery{WithItems: true})
	require.NoError(t, err)

	foundWithItems := map[uuid.UUID]TreeItem{}
	var walkWithItems func(items []*TreeItem)
	walkWithItems = func(items []*TreeItem) {
		for _, it := range items {
			foundWithItems[it.ID] = *it
			walkWithItems(it.Children)
		}
	}
	for i := range treeWithItems {
		foundWithItems[treeWithItems[i].ID] = treeWithItems[i]
		walkWithItems(treeWithItems[i].Children)
	}

	require.Contains(t, foundWithItems, c.ID)
	assert.Equal(t, "gem-outline", foundWithItems[c.ID].Icon)
	assert.Equal(t, "widget-outline", foundWithItems[c.ID].TypeIcon)
	assert.False(t, foundWithItems[c.ID].IsContainer)
	// The parent location's fields must still be intact in the WithItems tree too.
	require.Contains(t, foundWithItems, a.ID)
	assert.Equal(t, "treasure-chest", foundWithItems[a.ID].Icon)
	assert.Equal(t, "basket-outline", foundWithItems[a.ID].TypeIcon)
	assert.True(t, foundWithItems[a.ID].IsContainer)

	// Cleanup
	require.NoError(t, tRepos.Entities.Delete(ctx, c.ID))
	require.NoError(t, tRepos.Entities.Delete(ctx, a.ID))
	require.NoError(t, tRepos.Entities.Delete(ctx, b.ID))
}

func TestEntityRepository_PathForEntity_CarriesIconFields(t *testing.T) {
	ctx := context.Background()

	et, err := tRepos.EntityTypes.Create(ctx, tGroup.ID, EntityTypeCreate{
		Name: "Iconic Shelf", IsLocation: true, IsContainer: true, Icon: "bookshelf",
	})
	require.NoError(t, err)

	parent := entityFactory()
	parent.EntityTypeID = et.ID
	parent.Icon = "safe"
	p, err := tRepos.Entities.Create(ctx, tGroup.ID, parent)
	require.NoError(t, err)

	child := entityFactory()
	child.EntityTypeID = et.ID
	child.ParentID = p.ID
	c, err := tRepos.Entities.Create(ctx, tGroup.ID, child)
	require.NoError(t, err)

	path, err := tRepos.Entities.PathForEntity(ctx, tGroup.ID, c.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(path), 2)

	// First element is the root ancestor (the parent), last is the entity itself.
	assert.Equal(t, "safe", path[0].Icon)
	assert.Equal(t, "bookshelf", path[0].TypeIcon)
	assert.True(t, path[0].IsContainer)

	// Cleanup
	require.NoError(t, tRepos.Entities.Delete(ctx, c.ID))
	require.NoError(t, tRepos.Entities.Delete(ctx, p.ID))
}

func TestEntityRepository_GetOne_DerivesNearestLocationAncestor(t *testing.T) {
	ctx := context.Background()
	locationET := useContainerEntityType(t)
	itemET := useItemEntityType(t)

	create := func(name string, entityTypeID, parentID uuid.UUID) EntityOut {
		t.Helper()
		out, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
			Name:         name,
			Quantity:     1,
			EntityTypeID: entityTypeID,
			ParentID:     parentID,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), out.ID) })
		return out
	}

	location := create("nearest-location", locationET.ID, uuid.Nil)
	box := create("nearest-location-box", itemET.ID, location.ID)
	nested := create("nearest-location-item", itemET.ID, box.ID)
	direct := create("direct-location-item", itemET.ID, location.ID)

	got, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, nested.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Parent)
	assert.Equal(t, box.ID, got.Parent.ID)
	require.NotNil(t, got.Location)
	assert.Equal(t, location.ID, got.Location.ID)

	got, err = tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, direct.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Location)
	assert.Equal(t, location.ID, got.Location.ID)

	got, err = tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, location.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Location)
}

func TestEntityRepository_GetOne_DoesNotFollowLegacyForeignParent(t *testing.T) {
	ctx := context.Background()
	itemET := useItemEntityType(t)
	own, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "legacy-cross-group-child",
		Quantity:     1,
		EntityTypeID: itemET.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), own.ID) })

	foreignGID, _, _ := makeForeignGroup(t)
	foreignLocationET, err := tRepos.EntityTypes.GetDefault(ctx, foreignGID, true)
	require.NoError(t, err)
	foreignLocation, err := tRepos.Entities.Create(ctx, foreignGID, EntityCreate{
		Name:         "foreign-location-must-not-leak",
		Quantity:     1,
		EntityTypeID: foreignLocationET.ID,
	})
	require.NoError(t, err)

	_, err = tClient.Entity.UpdateOneID(own.ID).SetParentID(foreignLocation.ID).Save(ctx)
	require.NoError(t, err)

	got, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, own.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Parent, "foreign direct parent must be filtered")
	assert.Nil(t, got.Location, "foreign location ancestor must be filtered")
}

func TestEntityRepository_MovingEntityKeepsChildrenAttached(t *testing.T) {
	ctx := context.Background()
	locationET := useContainerEntityType(t)
	itemET := useItemEntityType(t)

	create := func(name string, entityTypeID, parentID uuid.UUID) EntityOut {
		t.Helper()
		out, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
			Name:         name,
			Quantity:     1,
			EntityTypeID: entityTypeID,
			ParentID:     parentID,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), out.ID) })
		return out
	}

	locationA := create("move-location-a", locationET.ID, uuid.Nil)
	locationB := create("move-location-b", locationET.ID, uuid.Nil)
	box := create("move-box", itemET.ID, locationA.ID)
	child := create("move-child", itemET.ID, box.ID)

	_, err := tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, EntityUpdate{
		ID:                       box.ID,
		Name:                     box.Name,
		Quantity:                 1,
		EntityTypeID:             itemET.ID,
		ParentID:                 locationB.ID,
		SyncChildEntityLocations: true,
	})
	require.NoError(t, err)

	got, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, child.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Parent)
	assert.Equal(t, box.ID, got.Parent.ID, "moving a box must not flatten its children")
	require.NotNil(t, got.Location)
	assert.Equal(t, locationB.ID, got.Location.ID)

	err = tRepos.Entities.Patch(ctx, tGroup.ID, box.ID, EntityPatch{ParentID: locationA.ID})
	require.NoError(t, err)
	got, err = tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, child.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Parent)
	assert.Equal(t, box.ID, got.Parent.ID)
}

func TestEntityRepository_RejectsNegativeQuantity(t *testing.T) {
	ctx := context.Background()
	itemET := useItemEntityType(t)

	_, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "negative-create",
		Quantity:     -1,
		EntityTypeID: itemET.ID,
	})
	require.ErrorContains(t, err, "must not be negative")

	created, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "negative-guard",
		Quantity:     1,
		EntityTypeID: itemET.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), created.ID) })

	_, err = tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, EntityUpdate{
		ID:           created.ID,
		Name:         created.Name,
		Quantity:     -2,
		EntityTypeID: itemET.ID,
	})
	require.ErrorContains(t, err, "must not be negative")

	negative := -3.0
	err = tRepos.Entities.Patch(ctx, tGroup.ID, created.ID, EntityPatch{Quantity: &negative})
	require.ErrorContains(t, err, "must not be negative")
}
