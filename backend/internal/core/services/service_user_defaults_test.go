package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

func requireDefaultLocationAssetIDs(t *testing.T, gid uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	entities, err := tRepos.Entities.GetAll(ctx, gid)
	require.NoError(t, err)
	require.Len(t, entities, len(defaultLocations()))

	assetIDs := make(map[repo.AssetID]struct{}, len(entities))
	for _, entity := range entities {
		require.Positive(t, entity.AssetID)
		assetIDs[entity.AssetID] = struct{}{}
	}
	require.Len(t, assetIDs, len(entities), "default locations must receive distinct asset IDs")

	updated, err := tSvc.Entities.EnsureAssetID(ctx, gid)
	require.NoError(t, err)
	require.Zero(t, updated, "startup repair must have nothing to backfill after registration")
}

func TestRegisterUserAssignsDefaultLocationAssetIDs(t *testing.T) {
	ctx := context.Background()
	usr, err := tSvc.User.RegisterUser(ctx, UserRegistration{
		Name:     "Asset ID User",
		Email:    fk.Email(),
		Password: "asset-id-password",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.Users.Delete(context.Background(), usr.ID)
		_ = tRepos.Groups.GroupDelete(context.Background(), usr.DefaultGroupID)
	})

	requireDefaultLocationAssetIDs(t, usr.DefaultGroupID)
}

func TestRegisterOIDCUserAssignsDefaultLocationAssetIDs(t *testing.T) {
	ctx := context.Background()
	usr, err := tSvc.User.registerOIDCUser(
		ctx,
		"https://asset-id.test",
		fk.Str(12),
		fk.Email(),
		"OIDC Asset ID User",
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.Users.Delete(context.Background(), usr.ID)
		_ = tRepos.Groups.GroupDelete(context.Background(), usr.DefaultGroupID)
	})

	requireDefaultLocationAssetIDs(t, usr.DefaultGroupID)
}
