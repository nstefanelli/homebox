package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

func createOIDCTestUser(t *testing.T, password *string, issuer, subject string) repo.UserOut {
	t.Helper()
	ctx := context.Background()
	group, err := tRepos.Groups.GroupCreate(ctx, "oidc-test-"+fk.Str(6), uuid.Nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Groups.GroupDelete(context.Background(), group.ID) })

	create := repo.UserCreate{
		Name:           "OIDC Test",
		Email:          fk.Email(),
		Password:       password,
		DefaultGroupID: group.ID,
		IsOwner:        true,
	}
	var usr repo.UserOut
	if issuer == "" && subject == "" {
		usr, err = tRepos.Users.Create(ctx, create)
	} else {
		usr, err = tRepos.Users.CreateWithOIDC(ctx, create, issuer, subject)
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Users.Delete(context.Background(), usr.ID) })
	return usr
}

func TestLoginOIDCRefusesToLinkLocalPasswordAccountByEmail(t *testing.T) {
	ctx := context.Background()
	password := "local-password"
	usr := createOIDCTestUser(t, &password, "", "")

	_, err := tSvc.User.LoginOIDC(ctx, "https://untrusted-idp.example", "attacker-subject", usr.Email, usr.Name)
	require.ErrorIs(t, err, ErrorOIDCAccountConflict)

	got, err := tRepos.Users.GetOneID(ctx, usr.ID)
	require.NoError(t, err)
	assert.Nil(t, got.OidcIssuer)
	assert.Nil(t, got.OidcSubject)
}

func TestLoginOIDCRefusesToReplaceExistingOIDCIdentityByEmail(t *testing.T) {
	ctx := context.Background()
	usr := createOIDCTestUser(t, nil, "https://trusted-idp.example", "trusted-subject")

	_, err := tSvc.User.LoginOIDC(ctx, "https://other-idp.example", "other-subject", usr.Email, usr.Name)
	require.ErrorIs(t, err, ErrorOIDCAccountConflict)

	got, err := tRepos.Users.GetOneID(ctx, usr.ID)
	require.NoError(t, err)
	require.NotNil(t, got.OidcIssuer)
	require.NotNil(t, got.OidcSubject)
	assert.Equal(t, "https://trusted-idp.example", *got.OidcIssuer)
	assert.Equal(t, "trusted-subject", *got.OidcSubject)
}

func TestLoginOIDCMigratesOnlyUnclaimedPasswordlessLegacyAccount(t *testing.T) {
	ctx := context.Background()
	usr := createOIDCTestUser(t, nil, "", "")

	token, err := tSvc.User.LoginOIDC(ctx, "https://trusted-idp.example", "legacy-subject", usr.Email, usr.Name)
	require.NoError(t, err)
	assert.NotEmpty(t, token.Raw)

	got, err := tRepos.Users.GetOneOIDC(ctx, "https://trusted-idp.example", "legacy-subject")
	require.NoError(t, err)
	assert.Equal(t, usr.ID, got.ID)
}

func TestRegisterOIDCUserCleansGroupWhenUserInsertFails(t *testing.T) {
	ctx := context.Background()
	password := "existing-password"
	existing := createOIDCTestUser(t, &password, "", "")

	before, err := tClient.Group.Query().Count(ctx)
	require.NoError(t, err)

	_, err = tSvc.User.registerOIDCUser(
		ctx,
		"https://trusted-idp.example",
		"duplicate-email-subject",
		existing.Email,
		"Duplicate",
	)
	require.Error(t, err)

	after, err := tClient.Group.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after, "failed OIDC registration must not leave an orphan group")
}
