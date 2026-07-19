package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
)

// useToteEntityType creates a dedicated container-flagged entity type for tests.
func useToteEntityType(t *testing.T) EntityTypeSummary {
	t.Helper()
	et, err := tRepos.EntityTypes.Create(context.Background(), tGroup.ID, EntityTypeCreate{
		Name:        fk.Str(10),
		IsLocation:  true,
		IsContainer: true,
		Icon:        "mdi-package-variant",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.EntityTypes.Delete(context.Background(), tGroup.ID, et.ID)
	})
	return et
}

// usePlainLocationEntityType creates a dedicated non-container location entity
// type for tests. We can't rely on useContainerEntityType/GetDefault here:
// GetDefault returns the group's *first-created* IsLocation type, and since
// this file's tests may run before other fixtures in the package establish
// one, that "default" could end up being a container type created earlier in
// the same test. Creating our own type keeps this test's result deterministic
// regardless of package-wide test ordering.
func usePlainLocationEntityType(t *testing.T) EntityTypeSummary {
	t.Helper()
	et, err := tRepos.EntityTypes.Create(context.Background(), tGroup.ID, EntityTypeCreate{
		Name:        fk.Str(10),
		IsLocation:  true,
		IsContainer: false,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.EntityTypes.Delete(context.Background(), tGroup.ID, et.ID)
	})
	return et
}

// TestEntityRepository_Create_ContainerDoesNotForceSync verifies that creating
// a container-flagged entity does NOT default SyncChildEntityLocations to
// true. That flag, when true, makes UpdateByGroup reparent ALL of the
// entity's children onto whatever new parent the entity itself is given —
// i.e. it flattens the container's contents out into the destination
// location on every move (and even on a no-op save, since a PUT from the
// edit page always carries the current parent). Contents-follow-container
// is already inherent in the tree: children keep the container as their
// parent, and only the container's own parent changes when it moves. So
// containers must NOT default this flag — doing so is data-destroying.
func TestEntityRepository_Create_ContainerDoesNotForceSync(t *testing.T) {
	tote := useToteEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = tote.ID
	out, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), out.ID) })

	assert.False(t, out.SyncChildEntityLocations,
		"container-type entities must NOT default to syncing child locations — the sync flag flattens children onto the new parent on move, and tree nesting alone already implements contents-follow-container")
}

// TestEntityRepository_CreateFromTemplate_ContainerDoesNotForceSync mirrors
// TestEntityRepository_Create_ContainerDoesNotForceSync for the
// CreateFromTemplate path (and, by extension, CreateFromTemplateBatch, which
// shares the same createFromTemplateTx helper).
func TestEntityRepository_CreateFromTemplate_ContainerDoesNotForceSync(t *testing.T) {
	tote := useToteEntityType(t)

	parent, err := tRepos.Entities.Create(context.Background(), tGroup.ID, containerFactory())
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), parent.ID) })

	out, err := tRepos.Entities.CreateFromTemplate(context.Background(), tGroup.ID, EntityCreateFromTemplate{
		Name:         fk.Str(10),
		Quantity:     1,
		ParentID:     parent.ID,
		EntityTypeID: tote.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), out.ID) })

	assert.False(t, out.SyncChildEntityLocations,
		"entities created from a template with a container type must NOT default to syncing child locations — same rationale as TestEntityRepository_Create_ContainerDoesNotForceSync")
}

func TestEntityRepository_Query_IsContainerFilter(t *testing.T) {
	tote := useToteEntityType(t)
	plainLocation := usePlainLocationEntityType(t) // dedicated location type, NOT a container

	cf := containerFactory()
	cf.EntityTypeID = tote.ID
	toteEntity, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), toteEntity.ID) })

	lf := containerFactory()
	lf.EntityTypeID = plainLocation.ID
	shelf, err := tRepos.Entities.Create(context.Background(), tGroup.ID, lf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), shelf.ID) })

	res, err := tRepos.Entities.QueryByGroup(context.Background(), tGroup.ID, EntityQuery{
		IsLocation:  lo.ToPtr(true),
		IsContainer: lo.ToPtr(true),
	})
	require.NoError(t, err)

	ids := lo.Map(res.Items, func(e EntitySummary, _ int) string { return e.ID.String() })
	assert.Contains(t, ids, toteEntity.ID.String())
	assert.NotContains(t, ids, shelf.ID.String())
}

func TestEntityRepository_GetOneByGroup_ContainerTotalPrice(t *testing.T) {
	ctx := context.Background()
	tote := useToteEntityType(t)
	itemType := useItemEntityType(t)

	root, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "priced-root",
		EntityTypeID: tote.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(ctx, root.ID) })

	nested, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "priced-nested",
		ParentID:     root.ID,
		EntityTypeID: tote.ID,
	})
	require.NoError(t, err)

	createPricedItem := func(name string, parentID uuid.UUID, quantity, price float64) EntityOut {
		t.Helper()
		out, createErr := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
			Name:         name,
			ParentID:     parentID,
			EntityTypeID: itemType.ID,
			Quantity:     quantity,
		})
		require.NoError(t, createErr)
		require.NoError(t, tClient.Entity.UpdateOneID(out.ID).SetPurchasePrice(price).Exec(ctx))
		return out
	}

	createPricedItem("direct", root.ID, 2, 10.5)
	createPricedItem("nested", nested.ID, 3, 4)

	sold := createPricedItem("sold", nested.ID, 5, 100)
	require.NoError(t, tClient.Entity.UpdateOneID(sold.ID).SetSoldDate(time.Now()).Exec(ctx))

	archived := createPricedItem("archived", root.ID, 7, 200)
	require.NoError(t, tClient.Entity.UpdateOneID(archived.ID).SetArchived(true).Exec(ctx))

	// A legacy corrupt cross-tenant edge must not leak into this group's total.
	foreignGID, _, foreignTypeID := makeForeignGroup(t)
	foreign, err := tRepos.Entities.Create(ctx, foreignGID, EntityCreate{
		Name:         "foreign-priced",
		EntityTypeID: foreignTypeID,
		Quantity:     10,
	})
	require.NoError(t, err)
	require.NoError(t, tClient.Entity.UpdateOneID(foreign.ID).
		SetParentID(root.ID).
		SetPurchasePrice(999).
		Exec(ctx))

	got, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, root.ID)
	require.NoError(t, err)
	assert.InDelta(t, 33, got.TotalPrice, 0.001)

	// Corrupt cycles terminate at the depth bound and DISTINCT prevents
	// descendants reached more than once from inflating the total.
	require.NoError(t, tClient.Entity.UpdateOneID(root.ID).SetParentID(nested.ID).Exec(ctx))
	got, err = tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, root.ID)
	require.NoError(t, err)
	assert.InDelta(t, 33, got.TotalPrice, 0.001)
	require.NoError(t, tClient.Entity.UpdateOneID(root.ID).ClearParent().Exec(ctx))
}

// TestEntityRepository_CreateFromTemplate_CopiesPhoto verifies that creating
// an entity from a template that has a stored photo copies that photo onto
// the new entity as a primary photo attachment sharing the same blob path.
func TestEntityRepository_CreateFromTemplate_CopiesPhoto(t *testing.T) {
	tote := useToteEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = tote.ID
	parent, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), parent.ID) })

	out, err := tRepos.Entities.CreateFromTemplate(context.Background(), tGroup.ID, EntityCreateFromTemplate{
		Name:          "Tote 01",
		Quantity:      1,
		ParentID:      parent.ID,
		EntityTypeID:  tote.ID,
		PhotoPath:     "grp/documents/deadbeef",
		PhotoMimeType: "image/jpeg",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), out.ID) })

	require.NotEmpty(t, out.Attachments, "created entity must carry the template photo attachment")
	assert.Equal(t, "grp/documents/deadbeef", out.Attachments[0].Path)
	assert.True(t, out.Attachments[0].Primary)
}

func TestEntityRepository_CreateFromTemplate_PreservesTypedFields(t *testing.T) {
	ctx := context.Background()
	containerType := useContainerEntityType(t)
	itemType := useItemEntityType(t)
	container, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "typed-field-parent",
		EntityTypeID: containerType.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(ctx, container.ID) })

	timeValue := time.Date(2026, time.May, 6, 7, 8, 9, 0, time.UTC)
	fields := []EntityFieldData{
		{Type: "text", Name: "text", TextValue: "kept"},
		{Type: testFieldTypeNumber, Name: "number", NumberValue: 73},
		{Type: testFieldTypeBoolean, Name: "boolean", BooleanValue: true},
		{Type: "time", Name: "time", TimeValue: timeValue},
	}
	assertFields := func(out EntityOut) {
		t.Helper()
		require.Len(t, out.Fields, 4)
		byName := lo.SliceToMap(out.Fields, func(field EntityFieldData) (string, EntityFieldData) {
			return field.Name, field
		})
		assert.Equal(t, "kept", byName["text"].TextValue)
		assert.Equal(t, 73, byName["number"].NumberValue)
		assert.True(t, byName["boolean"].BooleanValue)
		assert.WithinDuration(t, timeValue, byName["time"].TimeValue, time.Second)
	}

	single, err := tRepos.Entities.CreateFromTemplate(ctx, tGroup.ID, EntityCreateFromTemplate{
		Name:         "typed-field-single",
		Quantity:     1,
		ParentID:     container.ID,
		EntityTypeID: itemType.ID,
		Fields:       fields,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(ctx, single.ID) })
	assertFields(single)

	batch, err := tRepos.Entities.CreateFromTemplateBatch(ctx, tGroup.ID, EntityBatchCreateFromTemplate{
		Template: EntityCreateFromTemplate{
			Quantity:     1,
			ParentID:     container.ID,
			EntityTypeID: itemType.ID,
			Fields:       fields,
		},
		Count:      2,
		NamePrefix: "typed-field-batch",
	})
	require.NoError(t, err)
	require.Len(t, batch, 2)
	t.Cleanup(func() {
		for _, out := range batch {
			_ = tRepos.Entities.Delete(ctx, out.ID)
		}
	})
	for _, out := range batch {
		assertFields(out)
	}
}

func TestEntityRepository_CreateFromTemplateBatch(t *testing.T) {
	tote := useToteEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = tote.ID
	shelf, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), shelf.ID) })

	outs, err := tRepos.Entities.CreateFromTemplateBatch(context.Background(), tGroup.ID, EntityBatchCreateFromTemplate{
		Template: EntityCreateFromTemplate{
			Quantity:     1,
			ParentID:     shelf.ID,
			EntityTypeID: tote.ID,
		},
		Count:      3,
		NamePrefix: "HDX 27-gal Tote",
	})
	require.NoError(t, err)
	require.Len(t, outs, 3)
	t.Cleanup(func() {
		for _, o := range outs {
			_ = tRepos.Entities.Delete(context.Background(), o.ID)
		}
	})

	assert.Equal(t, "HDX 27-gal Tote 01", outs[0].Name)
	assert.Equal(t, "HDX 27-gal Tote 02", outs[1].Name)
	assert.Equal(t, "HDX 27-gal Tote 03", outs[2].Name)
	// Distinct asset IDs
	assert.NotEqual(t, outs[0].AssetID, outs[1].AssetID)

	// A second batch continues the numbering automatically.
	outs2, err := tRepos.Entities.CreateFromTemplateBatch(context.Background(), tGroup.ID, EntityBatchCreateFromTemplate{
		Template: EntityCreateFromTemplate{
			Quantity:     1,
			ParentID:     shelf.ID,
			EntityTypeID: tote.ID,
		},
		Count:      2,
		NamePrefix: "HDX 27-gal Tote",
	})
	require.NoError(t, err)
	require.Len(t, outs2, 2)
	t.Cleanup(func() {
		for _, o := range outs2 {
			_ = tRepos.Entities.Delete(context.Background(), o.ID)
		}
	})
	assert.Equal(t, "HDX 27-gal Tote 04", outs2[0].Name)
	assert.Equal(t, "HDX 27-gal Tote 05", outs2[1].Name)
}

func TestEntityRepository_CreateFromTemplateBatch_CountBounds(t *testing.T) {
	_, err := tRepos.Entities.CreateFromTemplateBatch(context.Background(), tGroup.ID, EntityBatchCreateFromTemplate{
		Template:   EntityCreateFromTemplate{Quantity: 1},
		Count:      0,
		NamePrefix: "X",
	})
	require.Error(t, err)

	_, err = tRepos.Entities.CreateFromTemplateBatch(context.Background(), tGroup.ID, EntityBatchCreateFromTemplate{
		Template:   EntityCreateFromTemplate{Quantity: 1},
		Count:      101,
		NamePrefix: "X",
	})
	require.Error(t, err)
}

// TestEntityRepository_CreateFromTemplateBatch_RejectsCrossGroupParent verifies
// that CreateFromTemplateBatch performs the same group-ownership check as
// CreateFromTemplate/Create: a ParentID belonging to another tenant group must
// be rejected before any entity is written, not just when the single-entity
// path is used. Regression test for the batch path silently nesting entities
// under another group's tree.
func TestEntityRepository_CreateFromTemplateBatch_RejectsCrossGroupParent(t *testing.T) {
	itemET := useItemEntityType(t)

	foreignGID, _, _ := makeForeignGroup(t)
	foreignContainerType, err := tRepos.EntityTypes.GetDefault(context.Background(), foreignGID, true)
	require.NoError(t, err)
	foreignLoc, err := tRepos.Entities.Create(context.Background(), foreignGID, EntityCreate{
		Name:         "foreign-loc",
		EntityTypeID: foreignContainerType.ID,
	})
	require.NoError(t, err)

	_, err = tRepos.Entities.CreateFromTemplateBatch(context.Background(), tGroup.ID, EntityBatchCreateFromTemplate{
		Template: EntityCreateFromTemplate{
			Quantity:     1,
			ParentID:     foreignLoc.ID,
			EntityTypeID: itemET.ID,
		},
		Count:      2,
		NamePrefix: "Cross Group Batch " + fk.Str(6),
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group ParentID in batch, got %T: %v", err, err)
}

// TestEntityRepository_CreateFromTemplateBatch_DefaultsEntityType verifies that
// batch-creating entities from a template without an explicit entity type
// still succeeds and resolves the group's default item type for every entity
// in the batch, mirroring TestEntityRepository_CreateFromTemplate_DefaultsEntityType
// for the single-entity path (regression test for #1548 parity in the batch path).
func TestEntityRepository_CreateFromTemplateBatch_DefaultsEntityType(t *testing.T) {
	containerET := useContainerEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = containerET.ID
	container, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), container.ID) })

	outs, err := tRepos.Entities.CreateFromTemplateBatch(context.Background(), tGroup.ID, EntityBatchCreateFromTemplate{
		Template: EntityCreateFromTemplate{
			Quantity: 1,
			ParentID: container.ID,
			// EntityTypeID intentionally left zero to exercise the fallback.
		},
		Count:      2,
		NamePrefix: "Batch Default Type " + fk.Str(6),
	})
	require.NoError(t, err)
	require.Len(t, outs, 2)
	t.Cleanup(func() {
		for _, o := range outs {
			_ = tRepos.Entities.Delete(context.Background(), o.ID)
		}
	})

	for _, out := range outs {
		require.NotNil(t, out.EntityType)
		assert.False(t, out.EntityType.IsLocation)
		assert.NotEqual(t, uuid.Nil, out.EntityType.ID)
	}
}
