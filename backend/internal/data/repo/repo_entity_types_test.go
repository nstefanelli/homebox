package repo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func entityTypeFactory() EntityTypeCreate {
	return EntityTypeCreate{
		Name:       fk.Str(10),
		IsLocation: true,
		Icon:       "mdi-cube",
	}
}

func TestEntityTypesRepository_Create_IsContainer(t *testing.T) {
	data := entityTypeFactory()
	data.IsContainer = true

	result, err := tRepos.EntityTypes.Create(context.Background(), tGroup.ID, data)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.ID)
	assert.True(t, result.IsContainer)
	assert.True(t, result.IsLocation)

	// Cleanup
	err = tRepos.EntityTypes.Delete(context.Background(), tGroup.ID, result.ID)
	require.NoError(t, err)
}

func TestEntityTypesRepository_Update_IsContainer(t *testing.T) {
	created, err := tRepos.EntityTypes.Create(context.Background(), tGroup.ID, entityTypeFactory())
	require.NoError(t, err)
	assert.False(t, created.IsContainer)

	updated, err := tRepos.EntityTypes.Update(context.Background(), tGroup.ID, EntityTypeUpdate{
		ID:          created.ID,
		Name:        created.Name,
		IsLocation:  true,
		IsContainer: true,
		Icon:        created.Icon,
	})
	require.NoError(t, err)
	assert.True(t, updated.IsContainer)

	// Cleanup
	err = tRepos.EntityTypes.Delete(context.Background(), tGroup.ID, created.ID)
	require.NoError(t, err)
}

func TestEntityTypesRepository_RejectsContainerWithoutLocation(t *testing.T) {
	ctx := context.Background()

	_, err := tRepos.EntityTypes.Create(ctx, tGroup.ID, EntityTypeCreate{
		Name:        "invalid-container-" + uuid.NewString(),
		IsContainer: true,
		IsLocation:  false,
	})
	require.ErrorIs(t, err, ErrContainerRequiresLocation)

	created, err := tRepos.EntityTypes.Create(ctx, tGroup.ID, entityTypeFactory())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.EntityTypes.Delete(context.Background(), tGroup.ID, created.ID)
	})

	_, err = tRepos.EntityTypes.Update(ctx, tGroup.ID, EntityTypeUpdate{
		ID:          created.ID,
		Name:        created.Name,
		IsContainer: true,
		IsLocation:  false,
		Icon:        created.Icon,
	})
	require.ErrorIs(t, err, ErrContainerRequiresLocation)

	persisted, err := tClient.EntityType.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, persisted.IsLocation)
	assert.False(t, persisted.IsContainer)
}

func TestEntityTypeSchemaRejectsContainerWithoutLocation(t *testing.T) {
	_, err := tClient.EntityType.Create().
		SetName("invalid-schema-container-" + uuid.NewString()).
		SetIsContainer(true).
		SetIsLocation(false).
		SetGroupID(tGroup.ID).
		Save(context.Background())
	require.Error(t, err)
}
