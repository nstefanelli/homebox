package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

func createOwnedGroupAndUserForDeleteTest(t *testing.T) (repo.Group, repo.UserOut) {
	t.Helper()
	ctx := context.Background()
	group, err := tRepos.Groups.GroupCreate(ctx, "account-delete-group", uuid.Nil)
	require.NoError(t, err)
	password := "test-password"
	owner, err := tRepos.Users.Create(ctx, repo.UserCreate{
		Name:           "account-delete-owner",
		Email:          fk.Email(),
		Password:       &password,
		DefaultGroupID: group.ID,
		IsOwner:        true,
	})
	require.NoError(t, err)
	return group, owner
}

func TestDeleteSelfRejectsOwnerOfSharedGroup(t *testing.T) {
	ctx := context.Background()
	group, owner := createOwnedGroupAndUserForDeleteTest(t)
	_ = createGroupMemberForTest(t, group, "shared-group-member")

	err := tSvc.User.DeleteSelf(ctx, owner.ID)
	require.ErrorIs(t, err, ErrorOwnedGroupHasMembers)
	_, err = tRepos.Users.GetOneID(ctx, owner.ID)
	require.NoError(t, err)
	_, err = tRepos.Groups.GroupByID(ctx, group.ID)
	require.NoError(t, err)
}

func TestDeleteSelfRemovesSolelyOwnedGroups(t *testing.T) {
	ctx := context.Background()
	group, owner := createOwnedGroupAndUserForDeleteTest(t)

	require.NoError(t, tSvc.User.DeleteSelf(ctx, owner.ID))
	_, err := tRepos.Users.GetOneID(ctx, owner.ID)
	require.True(t, ent.IsNotFound(err))
	_, err = tRepos.Groups.GroupByID(ctx, group.ID)
	require.True(t, ent.IsNotFound(err))
}
