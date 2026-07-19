package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister_EmailCaseInsensitiveUniqueness(t *testing.T) {
	ctx := context.Background()
	base := strings.ToLower(fk.Str(10))
	lower := base + "@example.com"
	upper := strings.ToUpper(base) + "@EXAMPLE.COM"
	const password = "correct-horse-battery-staple"

	_, err := tSvc.User.RegisterUser(ctx, UserRegistration{
		Name:     fk.Str(8),
		Email:    lower,
		Password: password,
	})
	require.NoError(t, err)

	groupCountBeforeDuplicate, err := tClient.Group.Query().Count(ctx)
	require.NoError(t, err)
	_, err = tSvc.User.RegisterUser(ctx, UserRegistration{
		Name:     fk.Str(8),
		Email:    upper,
		Password: password,
	})
	require.ErrorIs(t, err, ErrorEmailAlreadyExists)
	groupCountAfterDuplicate, err := tClient.Group.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, groupCountBeforeDuplicate, groupCountAfterDuplicate, "duplicate registration must not leave an orphan group")

	tok, err := tSvc.User.Login(ctx, lower, password, false)
	require.NoError(t, err)
	assert.NotEmpty(t, tok.Raw)

	tok, err = tSvc.User.Login(ctx, upper, password, false)
	require.NoError(t, err)
	assert.NotEmpty(t, tok.Raw)
}

func TestRegister_NewGroupUserInsertFailureCleansGroup(t *testing.T) {
	ctx := context.Background()
	email := fk.Email()

	groupsBefore, err := tClient.Group.Query().Count(ctx)
	require.NoError(t, err)
	usersBefore, err := tClient.User.Query().Count(ctx)
	require.NoError(t, err)

	// An empty name passes the early request-independent checks and creates the
	// group, but ent rejects the user row. This deterministically exercises
	// the same compensating branch as a uniqueness race after the precheck.
	_, err = tSvc.User.RegisterUser(ctx, UserRegistration{
		Name:     "",
		Email:    email,
		Password: "valid-password",
	})
	require.Error(t, err)

	groupsAfter, err := tClient.Group.Query().Count(ctx)
	require.NoError(t, err)
	usersAfter, err := tClient.User.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, groupsBefore, groupsAfter, "failed registration must remove its newly-created group")
	assert.Equal(t, usersBefore, usersAfter, "failed registration must not leave a partial user")
}
