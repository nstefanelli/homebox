package repo

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
