package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/tag"
)

func tagFactory() TagCreate {
	return TagCreate{
		Name:        fk.Str(10),
		Description: fk.Str(100),
	}
}

func useTags(t *testing.T, len int) []TagOut {
	t.Helper()

	tags := make([]TagOut, len)
	for i := 0; i < len; i++ {
		itm := tagFactory()

		item, err := tRepos.Tags.Create(context.Background(), tGroup.ID, itm)
		require.NoError(t, err)
		tags[i] = item
	}

	t.Cleanup(func() {
		for _, item := range tags {
			_ = tRepos.Tags.delete(context.Background(), item.ID)
		}
	})

	return tags
}

func TestTagRepository_Get(t *testing.T) {
	tags := useTags(t, 1)
	tag := tags[0]

	// Get by ID
	foundLoc, err := tRepos.Tags.GetOne(context.Background(), tGroup.ID, tag.ID)
	require.NoError(t, err)
	assert.Equal(t, tag.ID, foundLoc.ID)
}

func TestTagRepositoryGetAll(t *testing.T) {
	useTags(t, 10)

	all, err := tRepos.Tags.GetAll(context.Background(), tGroup.ID)
	require.NoError(t, err)
	assert.Len(t, all, 10)
}

func TestTagRepository_Create(t *testing.T) {
	loc, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagFactory())
	require.NoError(t, err)

	// Get by ID
	foundLoc, err := tRepos.Tags.GetOne(context.Background(), tGroup.ID, loc.ID)
	require.NoError(t, err)
	assert.Equal(t, loc.ID, foundLoc.ID)

	err = tRepos.Tags.delete(context.Background(), loc.ID)
	require.NoError(t, err)
}

func TestTagsRepository_Update(t *testing.T) {
	loc, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagFactory())
	require.NoError(t, err)

	updateData := TagUpdate{
		ID:          loc.ID,
		Name:        fk.Str(10),
		Description: fk.Str(100),
	}

	update, err := tRepos.Tags.UpdateByGroup(context.Background(), tGroup.ID, updateData)
	require.NoError(t, err)

	foundLoc, err := tRepos.Tags.GetOne(context.Background(), tGroup.ID, loc.ID)
	require.NoError(t, err)

	assert.Equal(t, update.ID, foundLoc.ID)
	assert.Equal(t, update.Name, foundLoc.Name)
	assert.Equal(t, update.Description, foundLoc.Description)

	err = tRepos.Tags.delete(context.Background(), loc.ID)
	require.NoError(t, err)
}

func TestTagRepository_Delete(t *testing.T) {
	loc, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagFactory())
	require.NoError(t, err)

	err = tRepos.Tags.delete(context.Background(), loc.ID)
	require.NoError(t, err)

	_, err = tRepos.Tags.GetOne(context.Background(), tGroup.ID, loc.ID)
	require.Error(t, err)
}

func TestTagRepository_ParentChild(t *testing.T) {
	parent := tagFactory()
	parentTag, err := tRepos.Tags.Create(context.Background(), tGroup.ID, parent)
	require.NoError(t, err)

	child := tagFactory()
	child.ParentID = parentTag.ID
	childTag, err := tRepos.Tags.Create(context.Background(), tGroup.ID, child)
	require.NoError(t, err)

	assert.Equal(t, parentTag.ID, childTag.Parent.ID)

	// Fetch parent and check children
	foundParent, err := tRepos.Tags.GetOne(context.Background(), tGroup.ID, parentTag.ID)
	require.NoError(t, err)
	assert.Len(t, foundParent.Children, 1)
	assert.Equal(t, childTag.ID, foundParent.Children[0].ID)

	// Fetch child and check parent
	foundChild, err := tRepos.Tags.GetOne(context.Background(), tGroup.ID, childTag.ID)
	require.NoError(t, err)
	assert.NotNil(t, foundChild.Parent)
	assert.Equal(t, parentTag.ID, foundChild.Parent.ID)

	// Update child to remove parent
	updateData := TagUpdate{
		ID:          childTag.ID,
		Name:        childTag.Name,
		Description: childTag.Description,
		ParentID:    uuid.Nil,
	}
	updatedChild, err := tRepos.Tags.UpdateByGroup(context.Background(), tGroup.ID, updateData)
	require.NoError(t, err)
	assert.Nil(t, updatedChild.Parent)

	// Fetch parent again, should have no children
	foundParent, err = tRepos.Tags.GetOne(context.Background(), tGroup.ID, parentTag.ID)
	require.NoError(t, err)
	assert.Empty(t, foundParent.Children)
}

func TestTagRepository_MaxDepth(t *testing.T) {
	tags := make([]TagOut, 6)
	var prevID uuid.UUID

	// Create 5 levels
	for i := 0; i < 5; i++ {
		tCreate := tagFactory()
		tCreate.ParentID = prevID
		created, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tCreate)
		require.NoError(t, err)
		tags[i] = created
		prevID = created.ID
	}

	// Try to create 6th level
	tCreate := tagFactory()
	tCreate.ParentID = prevID
	_, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tCreate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max depth of 5 exceeded")

	// Cleanup
	for _, item := range tags {
		if item.ID != uuid.Nil {
			_ = tRepos.Tags.delete(context.Background(), item.ID)
		}
	}
}

func TestTagRepository_MoveSubtreeMaxDepth(t *testing.T) {
	// Create two chains: A->B->C (depth 3) and D->E (depth 2)
	// Try to move D under C -> A->B->C->D->E (depth 5). OK.
	// Try to move D under C, but D has children making it too deep.

	// Chain 1: A -> B -> C
	tagA, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagFactory())
	require.NoError(t, err)

	tagBOrig := tagFactory()
	tagBOrig.ParentID = tagA.ID
	tagB, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagBOrig)
	require.NoError(t, err)

	tagCOrig := tagFactory()
	tagCOrig.ParentID = tagB.ID
	tagC, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagCOrig)
	require.NoError(t, err)

	// Chain 2: D -> E
	tagD, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagFactory())
	require.NoError(t, err)

	tagEOrig := tagFactory()
	tagEOrig.ParentID = tagD.ID
	tagE, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagEOrig)
	require.NoError(t, err)

	// Move D under C. Result depth: 3 (C) + 2 (D->E) = 5. Should succeed.
	updateD := TagUpdate{
		ID:          tagD.ID,
		Name:        tagD.Name,
		Description: tagD.Description,
		ParentID:    tagC.ID,
	}
	_, err = tRepos.Tags.UpdateByGroup(context.Background(), tGroup.ID, updateD)
	require.NoError(t, err)

	// Reset D to root for next test
	updateD.ParentID = uuid.Nil
	_, err = tRepos.Tags.UpdateByGroup(context.Background(), tGroup.ID, updateD)
	require.NoError(t, err)

	// Add F to E -> D -> E -> F (depth 3)
	tagFOrig := tagFactory()
	tagFOrig.ParentID = tagE.ID
	tagF, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagFOrig)
	require.NoError(t, err)

	// Move D under C. Result depth: 3 (C) + 3 (D->E->F) = 6. Should Fail.
	updateD.ParentID = tagC.ID
	_, err = tRepos.Tags.UpdateByGroup(context.Background(), tGroup.ID, updateD)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max depth of 5 exceeded")

	// Cleanup
	_ = tRepos.Tags.delete(context.Background(), tagF.ID)
	_ = tRepos.Tags.delete(context.Background(), tagE.ID)
	_ = tRepos.Tags.delete(context.Background(), tagD.ID)
	_ = tRepos.Tags.delete(context.Background(), tagC.ID)
	_ = tRepos.Tags.delete(context.Background(), tagB.ID)
	_ = tRepos.Tags.delete(context.Background(), tagA.ID)
}

func TestTagRepository_CycleDetection(t *testing.T) {
	// A -> B
	// Try to set A's parent to B. Cycle.

	tagA, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagFactory())
	require.NoError(t, err)

	tagBOrig := tagFactory()
	tagBOrig.ParentID = tagA.ID
	tagB, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagBOrig)
	require.NoError(t, err)

	updateA := TagUpdate{
		ID:          tagA.ID,
		Name:        tagA.Name,
		Description: tagA.Description,
		ParentID:    tagB.ID,
	}
	_, err = tRepos.Tags.UpdateByGroup(context.Background(), tGroup.ID, updateA)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle detected")

	_ = tRepos.Tags.delete(context.Background(), tagB.ID)
	_ = tRepos.Tags.delete(context.Background(), tagA.ID)
}

func TestTagRepository_Create_NoParent(t *testing.T) {
	// Create a tag without a parent
	tagData := tagFactory()
	tagData.ParentID = uuid.Nil // Explicitly set to nil, though factory defaults to it

	createdTag, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagData)
	require.NoError(t, err)

	assert.Equal(t, tagData.Name, createdTag.Name)
	assert.Nil(t, createdTag.Parent)
	assert.Equal(t, uuid.Nil, createdTag.ParentID)

	// Verify persistence
	foundTag, err := tRepos.Tags.GetOne(context.Background(), tGroup.ID, createdTag.ID)
	require.NoError(t, err)
	assert.Nil(t, foundTag.Parent)
	assert.Equal(t, uuid.Nil, foundTag.ParentID)

	_ = tRepos.Tags.delete(context.Background(), createdTag.ID)
}

func TestTagRepository_Update_RemoveParent(t *testing.T) {
	// 1. Create Parent
	parent, err := tRepos.Tags.Create(context.Background(), tGroup.ID, tagFactory())
	require.NoError(t, err)

	// 2. Create Child
	childData := tagFactory()
	childData.ParentID = parent.ID
	child, err := tRepos.Tags.Create(context.Background(), tGroup.ID, childData)
	require.NoError(t, err)

	assert.NotNil(t, child.Parent)
	assert.Equal(t, parent.ID, child.Parent.ID)

	// 3. Update Child to remove parent
	updateData := TagUpdate{
		ID:          child.ID,
		Name:        child.Name,
		Description: child.Description,
		ParentID:    uuid.Nil, // Remove parent
	}
	updatedChild, err := tRepos.Tags.UpdateByGroup(context.Background(), tGroup.ID, updateData)
	require.NoError(t, err)

	assert.Nil(t, updatedChild.Parent)
	assert.Equal(t, uuid.Nil, updatedChild.ParentID)

	// 4. Verify persistence
	foundChild, err := tRepos.Tags.GetOne(context.Background(), tGroup.ID, child.ID)
	require.NoError(t, err)
	assert.Nil(t, foundChild.Parent)
	assert.Equal(t, uuid.Nil, foundChild.ParentID)

	_ = tRepos.Tags.delete(context.Background(), child.ID)
	_ = tRepos.Tags.delete(context.Background(), parent.ID)
}

func TestTagRepository_RelationshipReadsExcludeCrossGroupEdges(t *testing.T) {
	ctx := context.Background()
	parent, err := tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{Name: "scoped-parent"})
	require.NoError(t, err)
	child, err := tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{
		Name:     "scoped-child",
		ParentID: parent.ID,
	})
	require.NoError(t, err)
	_, foreignTagID, _ := makeForeignGroup(t)

	t.Cleanup(func() {
		_ = tClient.Tag.UpdateOneID(child.ID).ClearParent().Exec(context.Background())
		_ = tClient.Tag.UpdateOneID(foreignTagID).ClearParent().Exec(context.Background())
		_ = tRepos.Tags.delete(context.Background(), child.ID)
		_ = tRepos.Tags.delete(context.Background(), parent.ID)
		_ = tRepos.Tags.delete(context.Background(), foreignTagID)
	})

	// Simulate legacy corruption in both directions.
	require.NoError(t, tClient.Tag.UpdateOneID(child.ID).SetParentID(foreignTagID).Exec(ctx))
	require.NoError(t, tClient.Tag.UpdateOneID(foreignTagID).SetParentID(parent.ID).Exec(ctx))

	gotParent, err := tRepos.Tags.GetOne(ctx, tGroup.ID, parent.ID)
	require.NoError(t, err)
	assert.Empty(t, gotParent.Children, "foreign children must not be disclosed")

	gotChild, err := tRepos.Tags.GetOne(ctx, tGroup.ID, child.ID)
	require.NoError(t, err)
	assert.Nil(t, gotChild.Parent, "foreign parents must not be disclosed")
	assert.Equal(t, uuid.Nil, gotChild.ParentID)

	all, err := tRepos.Tags.GetAll(ctx, tGroup.ID)
	require.NoError(t, err)
	matched := false
	for _, item := range all {
		if item.ID == child.ID {
			matched = true
			assert.Equal(t, uuid.Nil, item.ParentID)
		}
	}
	assert.True(t, matched)
}

func TestTagRepository_DescendantsAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	root, err := tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{Name: "descendant-root"})
	require.NoError(t, err)
	child, err := tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{
		Name:     "descendant-child",
		ParentID: root.ID,
	})
	require.NoError(t, err)
	_, foreignTagID, _ := makeForeignGroup(t)

	t.Cleanup(func() {
		_ = tClient.Tag.UpdateOneID(foreignTagID).ClearParent().Exec(context.Background())
		_ = tRepos.Tags.delete(context.Background(), child.ID)
		_ = tRepos.Tags.delete(context.Background(), root.ID)
		_ = tRepos.Tags.delete(context.Background(), foreignTagID)
	})
	require.NoError(t, tClient.Tag.UpdateOneID(foreignTagID).SetParentID(root.ID).Exec(ctx))

	descendants, err := tRepos.Tags.GetDescendantTagIDs(ctx, tGroup.ID, []uuid.UUID{root.ID})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{root.ID, child.ID}, descendants)
	assert.NotContains(t, descendants, foreignTagID)

	_, err = tRepos.Tags.GetDescendantTagIDs(ctx, tGroup.ID, []uuid.UUID{foreignTagID})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err))

	_, err = tRepos.Entities.QueryByGroup(ctx, tGroup.ID, EntityQuery{TagIDs: []uuid.UUID{foreignTagID}})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err))
}

func TestTagRepository_RejectsCrossGroupMutationTargetsAndParents(t *testing.T) {
	ctx := context.Background()
	own, err := tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{Name: "own-tag"})
	require.NoError(t, err)
	foreignGID, foreignTagID, _ := makeForeignGroup(t)
	t.Cleanup(func() {
		_ = tRepos.Tags.delete(context.Background(), own.ID)
		_ = tRepos.Tags.delete(context.Background(), foreignTagID)
	})

	_, err = tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{
		Name:     "foreign-parent-create",
		ParentID: foreignTagID,
	})
	require.Error(t, err)

	_, err = tRepos.Tags.UpdateByGroup(ctx, tGroup.ID, TagUpdate{
		ID:       own.ID,
		Name:     "must-not-change",
		ParentID: foreignTagID,
	})
	require.Error(t, err)

	_, err = tRepos.Tags.UpdateByGroup(ctx, tGroup.ID, TagUpdate{
		ID:   foreignTagID,
		Name: "foreign-must-not-change",
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err))

	gotOwn, err := tRepos.Tags.GetOne(ctx, tGroup.ID, own.ID)
	require.NoError(t, err)
	assert.Equal(t, "own-tag", gotOwn.Name)
	gotForeign, err := tRepos.Tags.GetOne(ctx, foreignGID, foreignTagID)
	require.NoError(t, err)
	assert.NotEqual(t, "foreign-must-not-change", gotForeign.Name)
}

func TestTagRepository_CorruptCycleTraversalIsBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	a, err := tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{Name: "cycle-a"})
	require.NoError(t, err)
	b, err := tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{Name: "cycle-b", ParentID: a.ID})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tClient.Tag.UpdateOneID(a.ID).ClearParent().Exec(context.Background())
		_ = tClient.Tag.UpdateOneID(b.ID).ClearParent().Exec(context.Background())
		_ = tRepos.Tags.delete(context.Background(), b.ID)
		_ = tRepos.Tags.delete(context.Background(), a.ID)
	})
	require.NoError(t, tClient.Tag.UpdateOneID(a.ID).SetParentID(b.ID).Exec(ctx))

	descendants, err := tRepos.Tags.GetDescendantTagIDs(ctx, tGroup.ID, []uuid.UUID{a.ID})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{a.ID, b.ID}, descendants)

	_, err = tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{
		Name:     "cycle-child",
		ParentID: a.ID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max depth")
}

func TestTagRepository_ConcurrentMovesCannotCreateCycle(t *testing.T) {
	ctx := context.Background()
	a, err := tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{Name: "concurrent-a"})
	require.NoError(t, err)
	b, err := tRepos.Tags.Create(ctx, tGroup.ID, TagCreate{Name: "concurrent-b"})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tClient.Tag.UpdateOneID(a.ID).ClearParent().Exec(context.Background())
		_ = tClient.Tag.UpdateOneID(b.ID).ClearParent().Exec(context.Background())
		_ = tRepos.Tags.delete(context.Background(), b.ID)
		_ = tRepos.Tags.delete(context.Background(), a.ID)
	})

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, moveErr := tRepos.Tags.UpdateByGroup(ctx, tGroup.ID, TagUpdate{
			ID:       a.ID,
			Name:     a.Name,
			ParentID: b.ID,
		})
		results <- moveErr
	}()
	go func() {
		<-start
		_, moveErr := tRepos.Tags.UpdateByGroup(ctx, tGroup.ID, TagUpdate{
			ID:       b.ID,
			Name:     b.Name,
			ParentID: a.ID,
		})
		results <- moveErr
	}()
	close(start)

	firstErr := <-results
	secondErr := <-results
	successes := 0
	if firstErr == nil {
		successes++
	}
	if secondErr == nil {
		successes++
	}
	assert.LessOrEqual(t, successes, 1)

	parentID := func(id uuid.UUID) uuid.UUID {
		t.Helper()
		parent, queryErr := tClient.Tag.Query().
			Where(tag.ID(id)).
			QueryParent().
			Only(ctx)
		if ent.IsNotFound(queryErr) {
			return uuid.Nil
		}
		require.NoError(t, queryErr)
		return parent.ID
	}
	assert.False(t, parentID(a.ID) == b.ID && parentID(b.ID) == a.ID)
}
