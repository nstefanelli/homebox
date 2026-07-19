package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

func createGroupMemberForTest(t *testing.T, group repo.Group, name string) repo.UserOut {
	t.Helper()
	password := "test-password"
	member, err := tRepos.Users.Create(context.Background(), repo.UserCreate{
		Name:           name,
		Email:          fk.Email(),
		Password:       &password,
		DefaultGroupID: group.ID,
		IsOwner:        false,
	})
	require.NoError(t, err)
	return member
}

func TestGroupServiceRejectsAdministrativeMutationsFromNonOwner(t *testing.T) {
	ctx := context.Background()
	ownerCtx := Context{Context: ctx, UID: tUser.ID, GID: tGroup.ID}
	ownedGroup, err := tSvc.Group.CreateGroup(ownerCtx, "owner-gated-group")
	require.NoError(t, err)

	member := createGroupMemberForTest(t, ownedGroup, "ordinary-member")
	memberCtx := Context{Context: ctx, UID: member.ID, GID: ownedGroup.ID}

	_, err = tSvc.Group.UpdateGroup(memberCtx, repo.GroupUpdate{Name: "hijacked", Currency: "USD"})
	require.ErrorIs(t, err, ErrGroupOwnerRequired)
	require.ErrorIs(t, tSvc.Group.DeleteGroup(memberCtx), ErrGroupOwnerRequired)
	_, _, err = tSvc.Group.NewInvitation(memberCtx, 1, time.Now().Add(time.Hour))
	require.ErrorIs(t, err, ErrGroupOwnerRequired)
	_, err = tSvc.Group.GetInvitations(memberCtx)
	require.ErrorIs(t, err, ErrGroupOwnerRequired)
	require.ErrorIs(t, tSvc.Group.DeleteInvitation(memberCtx, ownedGroup.ID), ErrGroupOwnerRequired)
	require.ErrorIs(t, tSvc.Group.RemoveMember(memberCtx, tUser.ID), ErrGroupOwnerRequired)

	ownerCtx.GID = ownedGroup.ID
	require.ErrorIs(t, tSvc.Group.RemoveMember(ownerCtx, uuid.New()), ErrGroupMemberNotFound)

	unchanged, err := tRepos.Groups.GroupByID(ctx, ownedGroup.ID)
	require.NoError(t, err)
	require.Equal(t, "owner-gated-group", unchanged.Name)
}

func TestGroupServiceValidatesGroupAndInvitationInputs(t *testing.T) {
	ctx := Context{Context: context.Background(), UID: tUser.ID, GID: tGroup.ID}
	_, err := tSvc.Group.CreateGroup(ctx, "   ")
	require.ErrorIs(t, err, ErrInvalidGroupName)
	owned, err := tSvc.Group.CreateGroup(ctx, "validation-owner-group")
	require.NoError(t, err)
	ctx.GID = owned.ID

	_, err = tSvc.Group.UpdateGroup(ctx, repo.GroupUpdate{Name: "   ", Currency: "USD"})
	require.ErrorIs(t, err, ErrInvalidGroupName)

	_, _, err = tSvc.Group.NewInvitation(ctx, 0, time.Now().Add(time.Hour))
	require.ErrorIs(t, err, ErrInvalidInvitation)
	_, _, err = tSvc.Group.NewInvitation(ctx, 1, time.Now().Add(-time.Hour))
	require.ErrorIs(t, err, ErrInvalidInvitation)
}

func TestGroupServiceAllowsSafeSelfLeaveAndOwnerRemoval(t *testing.T) {
	ctx := context.Background()
	ownerCtx := Context{Context: ctx, UID: tUser.ID, GID: tGroup.ID}
	ownedGroup, err := tSvc.Group.CreateGroup(ownerCtx, "self-leave-group")
	require.NoError(t, err)
	ownerCtx.GID = ownedGroup.ID

	require.ErrorIs(t, tSvc.Group.RemoveMember(ownerCtx, tUser.ID), ErrGroupOwnerCannotLeave)

	lastGroupMember := createGroupMemberForTest(t, ownedGroup, "last-group-member")
	lastGroupMemberCtx := Context{Context: ctx, UID: lastGroupMember.ID, GID: ownedGroup.ID}
	require.NoError(t, tSvc.Group.RemoveMember(lastGroupMemberCtx, lastGroupMember.ID))
	afterLastLeave, err := tRepos.Users.GetOneID(ctx, lastGroupMember.ID)
	require.NoError(t, err)
	require.Empty(t, afterLastLeave.GroupIDs)
	require.Equal(t, uuid.Nil, afterLastLeave.DefaultGroupID)

	removedMember := createGroupMemberForTest(t, ownedGroup, "owner-removed-member")
	require.NoError(t, tSvc.Group.RemoveMember(ownerCtx, removedMember.ID))
	afterRemoval, err := tRepos.Users.GetOneID(ctx, removedMember.ID)
	require.NoError(t, err)
	require.Empty(t, afterRemoval.GroupIDs)
	require.Equal(t, uuid.Nil, afterRemoval.DefaultGroupID)

	leavingMember := createGroupMemberForTest(t, ownedGroup, "member-with-fallback")
	leavingCtx := Context{Context: ctx, UID: leavingMember.ID, GID: ownedGroup.ID}
	fallback, err := tSvc.Group.CreateGroup(leavingCtx, "member-fallback-group")
	require.NoError(t, err)

	require.NoError(t, tSvc.Group.RemoveMember(leavingCtx, leavingMember.ID))
	after, err := tRepos.Users.GetOneID(ctx, leavingMember.ID)
	require.NoError(t, err)
	require.NotContains(t, after.GroupIDs, ownedGroup.ID)
	require.Equal(t, fallback.ID, after.DefaultGroupID)
}
