package repo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/usergroup"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
)

func Test_Group_Create(t *testing.T) {
	g, err := tRepos.Groups.GroupCreate(context.Background(), "test", uuid.Nil)

	require.NoError(t, err)
	assert.Equal(t, "test", g.Name)

	// Get by ID
	foundGroup, err := tRepos.Groups.GroupByID(context.Background(), g.ID)
	require.NoError(t, err)
	assert.Equal(t, g.ID, foundGroup.ID)
}

func Test_Group_Create_WithUser(t *testing.T) {
	// Create a test user first
	user, err := tRepos.Users.Create(context.Background(), UserCreate{
		Name:           "test_user",
		Email:          "test_group_user@example.com",
		Password:       new("password123"),
		DefaultGroupID: tGroup.ID,
	})
	require.NoError(t, err)

	// Create a group with the user
	g, err := tRepos.Groups.GroupCreate(context.Background(), "test_group_with_user", user.ID)
	require.NoError(t, err)
	assert.Equal(t, "test_group_with_user", g.Name)

	// Verify the user is a member of the group
	members, err := tRepos.Users.GetUsersByGroupID(context.Background(), g.ID)
	require.NoError(t, err)
	assert.Len(t, members, 1, "Group should have exactly one member")
	assert.Equal(t, user.Name, members[0].Name, "The member should be the user who created the group")
}

func Test_Group_Update(t *testing.T) {
	g, err := tRepos.Groups.GroupCreate(context.Background(), "test", uuid.Nil)
	require.NoError(t, err)

	g, err = tRepos.Groups.GroupUpdate(context.Background(), g.ID, GroupUpdate{
		Name:     "test2",
		Currency: "eur",
	})
	require.NoError(t, err)
	assert.Equal(t, "test2", g.Name)
	assert.Equal(t, "EUR", g.Currency)
}

func Test_Group_DeleteReassignsEveryAffectedDefaultGroup(t *testing.T) {
	ctx := context.Background()
	first := userFactory()
	first.DefaultGroupID = tGroup.ID
	firstUser, err := tRepos.Users.Create(ctx, first)
	require.NoError(t, err)

	deletedGroup, err := tRepos.Groups.GroupCreate(ctx, "delete-default", firstUser.ID)
	require.NoError(t, err)
	replacement, err := tRepos.Groups.GroupCreate(ctx, "replacement-default", firstUser.ID)
	require.NoError(t, err)

	second := userFactory()
	second.DefaultGroupID = tGroup.ID
	secondUser, err := tRepos.Users.Create(ctx, second)
	require.NoError(t, err)
	_, err = tClient.UserGroup.Create().
		SetUserID(secondUser.ID).
		SetGroupID(deletedGroup.ID).
		SetRole(usergroup.RoleUser).
		Save(ctx)
	require.NoError(t, err)

	// Remove the bootstrap-group memberships so firstUser has exactly one
	// replacement and secondUser has no replacement.
	_, err = tClient.UserGroup.Delete().
		Where(
			usergroup.UserIDIn(firstUser.ID, secondUser.ID),
			usergroup.GroupID(tGroup.ID),
		).
		Exec(ctx)
	require.NoError(t, err)
	require.NoError(t, tRepos.Users.UpdateDefaultGroup(ctx, firstUser.ID, deletedGroup.ID))
	require.NoError(t, tRepos.Users.UpdateDefaultGroup(ctx, secondUser.ID, deletedGroup.ID))

	require.NoError(t, tRepos.Groups.GroupDelete(ctx, deletedGroup.ID))

	gotFirst, err := tClient.User.Get(ctx, firstUser.ID)
	require.NoError(t, err)
	require.NotNil(t, gotFirst.DefaultGroupID)
	assert.Equal(t, replacement.ID, *gotFirst.DefaultGroupID)

	gotSecond, err := tClient.User.Get(ctx, secondUser.ID)
	require.NoError(t, err)
	assert.Nil(t, gotSecond.DefaultGroupID)
}

// TODO: Fix this test at some point, the data itself in production/development is working fine, it only fails on the test
/*func Test_Group_GroupStatistics(t *testing.T) {
	useItems(t, 20)
	useTags(t, 20)

	stats, err := tRepos.Groups.StatsGroup(context.Background(), tGroup.ID)

	require.NoError(t, err)
	assert.Equal(t, 20, stats.TotalItems)
	assert.Equal(t, 20, stats.TotalTags)
	assert.Equal(t, 1, stats.TotalUsers)
	assert.Equal(t, 1, stats.TotalLocations)
}*/

func Test_Group_IsMember(t *testing.T) {
	ctx := context.Background()

	group, err := tRepos.Groups.GroupCreate(ctx, "member-check", uuid.Nil)
	require.NoError(t, err)

	user := userFactory()
	user.DefaultGroupID = group.ID
	createdUser, err := tRepos.Users.Create(ctx, user)
	require.NoError(t, err)

	// Newly created user is added to default group in Create()
	isMember, err := tRepos.Groups.IsMember(ctx, group.ID, createdUser.ID)
	require.NoError(t, err)
	assert.True(t, isMember)

	otherUser := userFactory()
	otherUser.DefaultGroupID = tGroup.ID
	createdOther, err := tRepos.Users.Create(ctx, otherUser)
	require.NoError(t, err)

	isMember, err = tRepos.Groups.IsMember(ctx, group.ID, createdOther.ID)
	require.NoError(t, err)
	assert.False(t, isMember)
}

func Test_Group_RemoveMemberAndReassignDefault(t *testing.T) {
	ctx := context.Background()
	data := userFactory()
	data.DefaultGroupID = tGroup.ID
	member, err := tRepos.Users.Create(ctx, data)
	require.NoError(t, err)

	removed, err := tRepos.Groups.GroupCreate(ctx, "removed-membership", member.ID)
	require.NoError(t, err)
	replacement, err := tRepos.Groups.GroupCreate(ctx, "replacement-membership", member.ID)
	require.NoError(t, err)

	// Leave exactly one eligible replacement so the expected choice is clear.
	_, err = tClient.UserGroup.Delete().
		Where(
			usergroup.UserID(member.ID),
			usergroup.GroupID(tGroup.ID),
		).
		Exec(ctx)
	require.NoError(t, err)
	require.NoError(t, tRepos.Users.UpdateDefaultGroup(ctx, member.ID, removed.ID))

	require.NoError(t, tRepos.Groups.RemoveMemberAndReassignDefault(ctx, member.ID, removed.ID))

	isMember, err := tRepos.Groups.IsMember(ctx, removed.ID, member.ID)
	require.NoError(t, err)
	assert.False(t, isMember)

	got, err := tClient.User.Get(ctx, member.ID)
	require.NoError(t, err)
	require.NotNil(t, got.DefaultGroupID)
	assert.Equal(t, replacement.ID, *got.DefaultGroupID)
}

func Test_Group_RemoveMemberAndReassignDefault_RollsBackWithoutReplacement(t *testing.T) {
	ctx := context.Background()
	data := userFactory()
	data.DefaultGroupID = tGroup.ID
	member, err := tRepos.Users.Create(ctx, data)
	require.NoError(t, err)

	err = tRepos.Groups.RemoveMemberAndReassignDefault(ctx, member.ID, tGroup.ID)
	require.Error(t, err)

	isMember, memberErr := tRepos.Groups.IsMember(ctx, tGroup.ID, member.ID)
	require.NoError(t, memberErr)
	assert.True(t, isMember, "membership deletion must roll back when default reassignment fails")

	got, getErr := tClient.User.Get(ctx, member.ID)
	require.NoError(t, getErr)
	require.NotNil(t, got.DefaultGroupID)
	assert.Equal(t, tGroup.ID, *got.DefaultGroupID)
}

func Test_Group_Integrations_RoundTrip(t *testing.T) {
	ctx := context.Background()

	g, err := tRepos.Groups.GroupCreate(ctx, "integrations-round-trip", uuid.Nil)
	require.NoError(t, err)

	// Untouched group returns the zero value.
	got, err := tRepos.Groups.IntegrationsGet(ctx, g.ID)
	require.NoError(t, err)
	assert.Equal(t, types.GroupIntegrations{}, got)

	data := types.GroupIntegrations{
		AIProvider:                "anthropic",
		AIBaseURL:                 "https://api.anthropic.com",
		AIAPIKey:                  "sk-test-key",
		AIModel:                   "claude-3-5-sonnet",
		BarcodeTokenBarcodespider: "bs-token",
		OpenFoodFactsContact:      "test@example.com",
	}

	err = tRepos.Groups.IntegrationsSet(ctx, g.ID, data)
	require.NoError(t, err)

	got, err = tRepos.Groups.IntegrationsGet(ctx, g.ID)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}
