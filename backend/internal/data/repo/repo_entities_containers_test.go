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
