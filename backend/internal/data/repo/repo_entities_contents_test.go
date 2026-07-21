package repo

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEntityRepository_ContentsRoundTrip verifies that the free-text contents
// manifest persists through Create and UpdateByGroup and is returned verbatim
// (line breaks included) on EntityOut.
func TestEntityRepository_ContentsRoundTrip(t *testing.T) {
	ctx := context.Background()
	locationType := usePlainLocationEntityType(t)
	prefix := "contents-rt-" + uuid.NewString()
	manifest := "Baby Hats\n3x AA batteries\nWinter Gloves"

	created, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         prefix + "-tote",
		EntityTypeID: locationType.ID,
		Contents:     manifest,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.Entities.Delete(context.Background(), created.ID)
	})
	assert.Equal(t, manifest, created.Contents, "Create must return contents verbatim")

	fetched, err := tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, manifest, fetched.Contents, "GetOneByGroup must return contents verbatim")

	// Update replaces the whole manifest (last-write-wins by design).
	appended := manifest + "\nWool Socks"
	updated, err := tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, EntityUpdate{
		ID:           created.ID,
		Name:         created.Name,
		EntityTypeID: locationType.ID,
		Contents:     appended,
	})
	require.NoError(t, err)
	assert.Equal(t, appended, updated.Contents, "UpdateByGroup must persist and return the new contents")

	fetched, err = tRepos.Entities.GetOneByGroup(ctx, tGroup.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, appended, fetched.Contents)

	// Clearing works too: an update without contents empties the manifest.
	cleared, err := tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, EntityUpdate{
		ID:           created.ID,
		Name:         created.Name,
		EntityTypeID: locationType.ID,
	})
	require.NoError(t, err)
	assert.Empty(t, cleared.Contents)
}

// TestEntityRepository_QueryByGroup_SearchMatchesContents verifies the q search
// predicate matches contents lines with the same semantics as name/description
// (substring, case-insensitive), through the same QueryByGroup path
// HandleEntitiesGetAll uses.
func TestEntityRepository_QueryByGroup_SearchMatchesContents(t *testing.T) {
	ctx := context.Background()
	locationType := usePlainLocationEntityType(t)

	token := "manifestline" + strings.ReplaceAll(uuid.NewString(), "-", "")
	tote, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "contents-search-tote-" + uuid.NewString(),
		EntityTypeID: locationType.ID,
		Contents:     "Baby Hats\n" + token + "\nWinter Gloves",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.Entities.Delete(context.Background(), tote.ID)
	})

	find := func(search string) PaginationResult[EntitySummary] {
		t.Helper()
		result, err := tRepos.Entities.QueryByGroup(ctx, tGroup.ID, EntityQuery{
			Page:            -1,
			PageSize:        -1,
			Search:          search,
			IncludeAllKinds: true,
		})
		require.NoError(t, err)
		return result
	}

	// Exact contents line finds the container itself.
	result := find(token)
	require.Len(t, result.Items, 1, "a contents line must find the entity that carries it")
	assert.Equal(t, tote.ID, result.Items[0].ID)

	// Same case handling as the other q fields (ContainsFold): different case
	// and substring both match.
	result = find(strings.ToUpper(token))
	require.Len(t, result.Items, 1, "contents match must be case-insensitive like name/description")
	assert.Equal(t, tote.ID, result.Items[0].ID)

	result = find(token[3 : len(token)-3])
	require.Len(t, result.Items, 1, "contents match must be substring-based like name/description")
	assert.Equal(t, tote.ID, result.Items[0].ID)

	// The default items-only kind filter still applies: without the explicit
	// all-kinds opt-in, the location-typed tote stays hidden even though its
	// contents match.
	itemsOnly, err := tRepos.Entities.QueryByGroup(ctx, tGroup.ID, EntityQuery{
		Page:     -1,
		PageSize: -1,
		Search:   token,
	})
	require.NoError(t, err)
	assert.Empty(t, itemsOnly.Items, "kind filters must keep AND-composing with the widened q predicate")

	// Negative: a q that matches nothing still returns nothing.
	miss := find("no-such-manifest-line-" + uuid.NewString())
	assert.Empty(t, miss.Items)
	assert.Equal(t, 0, miss.Total)
}
