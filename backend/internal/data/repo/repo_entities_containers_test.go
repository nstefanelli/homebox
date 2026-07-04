package repo

import (
	"context"
	"testing"

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

func TestEntityRepository_Create_ContainerDefaultsSync(t *testing.T) {
	tote := useToteEntityType(t)

	cf := containerFactory()
	cf.EntityTypeID = tote.ID
	out, err := tRepos.Entities.Create(context.Background(), tGroup.ID, cf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Entities.Delete(context.Background(), out.ID) })

	assert.True(t, out.SyncChildEntityLocations,
		"entities of a container type must default to syncing child locations")
}

// TestEntityRepository_CreateFromTemplate_ContainerDefaultsSync verifies that
// creating an entity from a template with a container-flagged entity type
// defaults SyncChildEntityLocations to true, mirroring the same behavior
// already covered for Create() in TestEntityRepository_Create_ContainerDefaultsSync.
func TestEntityRepository_CreateFromTemplate_ContainerDefaultsSync(t *testing.T) {
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

	assert.True(t, out.SyncChildEntityLocations,
		"entities created from a template with a container type must default to syncing child locations")
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
