package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
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

func Test_Group_InventoryValuationSemantics(t *testing.T) {
	ctx := context.Background()
	inventoryGroup, err := tRepos.Groups.GroupCreate(ctx, "inventory-stats-"+uuid.NewString(), uuid.Nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Groups.GroupDelete(ctx, inventoryGroup.ID) })

	locationType, err := tRepos.EntityTypes.GetDefault(ctx, inventoryGroup.ID, true)
	require.NoError(t, err)
	itemType, err := tRepos.EntityTypes.GetDefault(ctx, inventoryGroup.ID, false)
	require.NoError(t, err)
	inventoryTag, err := tRepos.Tags.Create(ctx, inventoryGroup.ID, TagCreate{Name: "valued"})
	require.NoError(t, err)

	root, err := tRepos.Entities.Create(ctx, inventoryGroup.ID, EntityCreate{
		Name:         "root-location",
		EntityTypeID: locationType.ID,
	})
	require.NoError(t, err)
	nested, err := tRepos.Entities.Create(ctx, inventoryGroup.ID, EntityCreate{
		Name:         "nested-location",
		ParentID:     root.ID,
		EntityTypeID: locationType.ID,
	})
	require.NoError(t, err)

	start := time.Now().Add(-time.Minute)
	createPriced := func(name string, parentID uuid.UUID, quantity, price float64) EntityOut {
		t.Helper()
		out, createErr := tRepos.Entities.Create(ctx, inventoryGroup.ID, EntityCreate{
			Name:         name,
			ParentID:     parentID,
			EntityTypeID: itemType.ID,
			Quantity:     quantity,
			TagIDs:       []uuid.UUID{inventoryTag.ID},
		})
		require.NoError(t, createErr)
		require.NoError(t, tClient.Entity.UpdateOneID(out.ID).SetPurchasePrice(price).Exec(ctx))
		return out
	}

	createPriced("direct-active", root.ID, 2, 10.5)
	createPriced("nested-active", nested.ID, 3, 4)
	sold := createPriced("sold", nested.ID, 5, 100)
	require.NoError(t, tClient.Entity.UpdateOneID(sold.ID).SetSoldDate(time.Now()).Exec(ctx))
	archived := createPriced("archived", root.ID, 7, 200)
	require.NoError(t, tClient.Entity.UpdateOneID(archived.ID).SetArchived(true).Exec(ctx))

	foreignGroupID, _, foreignTypeID := makeForeignGroup(t)
	t.Cleanup(func() { _ = tRepos.Groups.GroupDelete(ctx, foreignGroupID) })
	foreign, err := tRepos.Entities.Create(ctx, foreignGroupID, EntityCreate{
		Name:         "foreign-corrupt-child",
		EntityTypeID: foreignTypeID,
		Quantity:     10,
	})
	require.NoError(t, err)
	require.NoError(t, tClient.Entity.UpdateOneID(foreign.ID).
		SetParentID(root.ID).
		SetPurchasePrice(999).
		AddTagIDs(inventoryTag.ID).
		Exec(ctx))

	// Legacy corruption can also point an entity in this group at a foreign
	// entity type. Type tenant scoping must exclude it everywhere.
	crossType, err := tRepos.Entities.Create(ctx, inventoryGroup.ID, EntityCreate{
		Name:         "foreign-type",
		ParentID:     root.ID,
		EntityTypeID: itemType.ID,
		Quantity:     1,
		TagIDs:       []uuid.UUID{inventoryTag.ID},
	})
	require.NoError(t, err)
	require.NoError(t, tClient.Entity.UpdateOneID(crossType.ID).
		SetEntityTypeID(foreignTypeID).
		SetPurchasePrice(555).
		Exec(ctx))

	stats, err := tRepos.Groups.StatsGroup(ctx, inventoryGroup.ID)
	require.NoError(t, err)
	assert.InDelta(t, 33, stats.TotalItemPrice, 0.001)
	assert.Equal(t, 3, stats.TotalItems, "sold items remain in the item count, while archived and foreign-typed rows do not")
	assert.Equal(t, 2, stats.TotalLocations)

	locations, err := tRepos.Groups.StatsLocationsByPurchasePrice(ctx, inventoryGroup.ID)
	require.NoError(t, err)
	locationTotals := lo.SliceToMap(locations, func(item TotalsByOrganizer) (string, float64) {
		return item.Name, item.Total
	})
	assert.InDelta(t, 33, locationTotals["root-location"], 0.001)
	assert.InDelta(t, 12, locationTotals["nested-location"], 0.001)

	tags, err := tRepos.Groups.StatsTagsByPurchasePrice(ctx, inventoryGroup.ID)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, inventoryTag.ID, tags[0].ID)
	assert.InDelta(t, 33, tags[0].Total, 0.001)

	overTime, err := tRepos.Groups.StatsPurchasePrice(ctx, inventoryGroup.ID, start, time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.InDelta(t, 0, overTime.PriceAtStart, 0.001)
	assert.InDelta(t, 33, overTime.PriceAtEnd, 0.001)
	entryTotal := lo.SumBy(overTime.Entries, func(entry ValueOverTimeEntry) float64 {
		return entry.Value
	})
	assert.InDelta(t, 33, entryTotal, 0.001)
	assert.ElementsMatch(t, []string{"direct-active", "nested-active"}, lo.Map(overTime.Entries, func(entry ValueOverTimeEntry, _ int) string {
		return entry.Name
	}))
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

func Test_Group_RemoveMemberAndReassignDefault_ClearsDefaultWithoutReplacement(t *testing.T) {
	ctx := context.Background()
	data := userFactory()
	data.DefaultGroupID = tGroup.ID
	member, err := tRepos.Users.Create(ctx, data)
	require.NoError(t, err)

	require.NoError(t, tRepos.Groups.RemoveMemberAndReassignDefault(ctx, member.ID, tGroup.ID))

	isMember, memberErr := tRepos.Groups.IsMember(ctx, tGroup.ID, member.ID)
	require.NoError(t, memberErr)
	assert.False(t, isMember)

	got, getErr := tClient.User.Get(ctx, member.ID)
	require.NoError(t, getErr)
	assert.Nil(t, got.DefaultGroupID)
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
