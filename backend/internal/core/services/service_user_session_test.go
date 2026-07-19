package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/authroles"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/authtokens"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/user"
	"github.com/sysadminsmedia/homebox/backend/pkgs/hasher"
)

// countUserSessions counts user-role auth tokens for uid. createSessionToken
// issues two rows per session (user + attachment); for session-revocation
// assertions we only care about the user-role rows since they're the bearer
// principal everywhere except the attachment download URL.
func countUserSessions(t *testing.T, ctx context.Context, uid uuid.UUID) int {
	t.Helper()
	c, err := tClient.AuthTokens.Query().
		Where(
			authtokens.HasUserWith(user.ID(uid)),
			authtokens.HasRolesWith(authroles.RoleEQ(authroles.RoleUser)),
		).
		Count(ctx)
	require.NoError(t, err)
	return c
}

func countAllSessionTokens(t *testing.T, ctx context.Context, uid uuid.UUID) int {
	t.Helper()
	c, err := tClient.AuthTokens.Query().
		Where(authtokens.HasUserWith(user.ID(uid))).
		Count(ctx)
	require.NoError(t, err)
	return c
}

// M1: RenewToken must invalidate the token it rotated, leaving only the new
// one valid afterward. Without this guarantee a leaked refresh request keeps
// the prior bearer alive in parallel with its replacement.
func TestRenewToken_InvalidatesPriorToken(t *testing.T) {
	ctx := context.Background()
	usr := newTestUserWithPassword(t, "renew-pw-1")

	old, err := tSvc.User.createSessionToken(ctx, usr.ID, false)
	require.NoError(t, err)
	require.Equal(t, 1, countUserSessions(t, ctx, usr.ID))
	require.Equal(t, 2, countAllSessionTokens(t, ctx, usr.ID))

	renewed, err := tSvc.User.RenewToken(ctx, old.Raw)
	require.NoError(t, err)
	assert.NotEqual(t, old.Raw, renewed.Raw, "RenewToken should mint a fresh raw token")

	// Exactly one session token remains, and it's the new one.
	assert.Equal(t, 1, countUserSessions(t, ctx, usr.ID), "old token should be revoked")
	assert.Equal(t, 2, countAllSessionTokens(t, ctx, usr.ID), "only the new user/attachment pair should remain")

	// The new token authenticates.
	_, err = tRepos.AuthTokens.GetUserFromToken(ctx, hasher.HashToken(renewed.Raw))
	require.NoError(t, err)

	// The old token does NOT authenticate.
	_, err = tRepos.AuthTokens.GetUserFromToken(ctx, hasher.HashToken(old.Raw))
	require.Error(t, err)
	_, err = tRepos.AuthTokens.GetUserFromToken(ctx, hasher.HashToken(old.AttachmentToken))
	require.Error(t, err, "refresh must revoke the old restricted attachment token too")

	_, err = tRepos.AuthTokens.GetUserFromToken(ctx, hasher.HashToken(renewed.AttachmentToken))
	require.NoError(t, err)
}

// M2: ChangePassword must revoke every session for the user except the one
// making the request. Other devices / stolen cookies are killed atomically
// with the password change; the caller stays logged in.
func TestChangePassword_RevokesOtherSessions_KeepsCurrent(t *testing.T) {
	ctx := context.Background()
	usr := newTestUserWithPassword(t, "old-cp-pw")

	current, err := tSvc.User.createSessionToken(ctx, usr.ID, false)
	require.NoError(t, err)
	other1, err := tSvc.User.createSessionToken(ctx, usr.ID, false)
	require.NoError(t, err)
	other2, err := tSvc.User.createSessionToken(ctx, usr.ID, false)
	require.NoError(t, err)
	require.Equal(t, 3, countUserSessions(t, ctx, usr.ID))
	require.Equal(t, 6, countAllSessionTokens(t, ctx, usr.ID))

	// Mark the request context as authed via the "current" session token so
	// ChangePassword preserves it.
	reqCtx := SetUserCtx(ctx, &usr, current.Raw)
	svcCtx := Context{Context: reqCtx, UID: usr.ID, GID: usr.DefaultGroupID, User: &usr}

	err = tSvc.User.ChangePassword(svcCtx, "old-cp-pw", "new-cp-pw")
	require.NoError(t, err, "ChangePassword should succeed with correct current password")

	// Only the current session survives.
	assert.Equal(t, 1, countUserSessions(t, ctx, usr.ID), "exactly one session (current) should remain")
	assert.Equal(t, 2, countAllSessionTokens(t, ctx, usr.ID), "both tokens in the current session pair should remain")

	// Current token still authenticates.
	_, err = tRepos.AuthTokens.GetUserFromToken(ctx, hasher.HashToken(current.Raw))
	require.NoError(t, err, "current session must remain valid")
	_, err = tRepos.AuthTokens.GetUserFromToken(ctx, hasher.HashToken(current.AttachmentToken))
	require.NoError(t, err, "current attachment token must remain valid")

	for _, raw := range []string{other1.Raw, other1.AttachmentToken, other2.Raw, other2.AttachmentToken} {
		_, err = tRepos.AuthTokens.GetUserFromToken(ctx, hasher.HashToken(raw))
		require.Error(t, err, "every token from other sessions must be revoked")
	}

	// Login works with the new password and not the old.
	_, err = tSvc.User.Login(ctx, usr.Email, "new-cp-pw", false)
	require.NoError(t, err)
	_, err = tSvc.User.Login(ctx, usr.Email, "old-cp-pw", false)
	require.ErrorIs(t, err, ErrorInvalidLogin)
}

// M2 fallback: when there's no session token in context (e.g. API-key auth),
// ChangePassword must still revoke every session token — nothing to preserve.
func TestChangePassword_NoSessionToken_RevokesAll(t *testing.T) {
	ctx := context.Background()
	usr := newTestUserWithPassword(t, "no-sess-pw")

	_, err := tSvc.User.createSessionToken(ctx, usr.ID, false)
	require.NoError(t, err)
	_, err = tSvc.User.createSessionToken(ctx, usr.ID, false)
	require.NoError(t, err)
	require.Equal(t, 2, countUserSessions(t, ctx, usr.ID))
	require.Equal(t, 4, countAllSessionTokens(t, ctx, usr.ID))

	svcCtx := Context{Context: ctx, UID: usr.ID, GID: usr.DefaultGroupID, User: &usr}

	err = tSvc.User.ChangePassword(svcCtx, "no-sess-pw", "no-sess-pw-new")
	require.NoError(t, err)

	assert.Equal(t, 0, countUserSessions(t, ctx, usr.ID), "all sessions should be revoked when no current session is in context")
	assert.Equal(t, 0, countAllSessionTokens(t, ctx, usr.ID))
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	ctx := context.Background()
	usr := newTestUserWithPassword(t, "correct-current")
	svcCtx := Context{Context: ctx, UID: usr.ID, GID: usr.DefaultGroupID, User: &usr}

	err := tSvc.User.ChangePassword(svcCtx, "wrong-current", "valid-new-password")
	require.ErrorIs(t, err, ErrorCurrentPasswordWrong)

	_, err = tSvc.User.Login(ctx, usr.Email, "correct-current", false)
	require.NoError(t, err, "rejected change must preserve the old password")
}

func TestChangePasswordRejectsShortNewPassword(t *testing.T) {
	ctx := context.Background()
	usr := newTestUserWithPassword(t, "correct-current-short")
	svcCtx := Context{Context: ctx, UID: usr.ID, GID: usr.DefaultGroupID, User: &usr}

	err := tSvc.User.ChangePassword(svcCtx, "correct-current-short", strings.Repeat("x", PasswordMinLength-1))
	require.ErrorIs(t, err, ErrorPasswordTooShort)

	_, err = tSvc.User.Login(ctx, usr.Email, "correct-current-short", false)
	require.NoError(t, err, "too-short change must preserve the old password")
}

func TestLogoutRevokesUserAndAttachmentTokenPair(t *testing.T) {
	ctx := context.Background()
	usr := newTestUserWithPassword(t, "logout-pair-pw")

	session, err := tSvc.User.createSessionToken(ctx, usr.ID, false)
	require.NoError(t, err)
	require.Equal(t, 2, countAllSessionTokens(t, ctx, usr.ID))

	require.NoError(t, tSvc.User.Logout(ctx, session.Raw))
	assert.Equal(t, 0, countAllSessionTokens(t, ctx, usr.ID))

	_, err = tRepos.AuthTokens.GetUserFromToken(ctx, hasher.HashToken(session.Raw))
	require.Error(t, err)
	_, err = tRepos.AuthTokens.GetUserFromToken(ctx, hasher.HashToken(session.AttachmentToken))
	require.Error(t, err)
}

// M4: RegisterUser must reject passwords shorter than PasswordMinLength.
func TestRegisterUser_RejectsShortPassword(t *testing.T) {
	ctx := context.Background()
	short := strings.Repeat("a", PasswordMinLength-1)

	_, err := tSvc.User.RegisterUser(ctx, UserRegistration{
		Name:     "Short Pwd User",
		Email:    fk.Email(),
		Password: short,
	})
	require.ErrorIs(t, err, ErrorPasswordTooShort)
}

func TestRegisterUser_RejectsEmptyPassword(t *testing.T) {
	_, err := tSvc.User.RegisterUser(context.Background(), UserRegistration{
		Name:     "Empty Pwd User",
		Email:    fk.Email(),
		Password: "",
	})
	require.ErrorIs(t, err, ErrorPasswordTooShort)
}

func TestRegisterUser_AcceptsMinLengthPassword(t *testing.T) {
	usr, err := tSvc.User.RegisterUser(context.Background(), UserRegistration{
		Name:     "Min Pwd User",
		Email:    fk.Email(),
		Password: strings.Repeat("a", PasswordMinLength),
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, usr.ID)
}
