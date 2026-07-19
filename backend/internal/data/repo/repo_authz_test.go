package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
)

func nowDate() types.Date {
	return types.DateFromTime(time.Now())
}

// makeForeignGroup creates a second tenant group with its own item entity type
// and tag, returning the group ID, a tag ID, and an entity type ID that all
// belong to that foreign group. Callers use these IDs to assert that the
// primary tGroup write paths refuse to attach foreign-tenant references.
func makeForeignGroup(t *testing.T) (gid uuid.UUID, foreignTagID uuid.UUID, foreignTypeID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	other, err := tRepos.Groups.GroupCreate(ctx, "authz-foreign-"+uuid.NewString(), uuid.Nil)
	require.NoError(t, err)

	otherType, err := tRepos.EntityTypes.GetDefault(ctx, other.ID, false)
	require.NoError(t, err)

	otherTag, err := tRepos.Tags.Create(ctx, other.ID, TagCreate{Name: "foreign-tag-" + uuid.NewString()})
	require.NoError(t, err)

	return other.ID, otherTag.ID, otherType.ID
}

func Test_EntityCreate_RejectsForeignParent(t *testing.T) {
	itemET := useItemEntityType(t)

	foreignGID, _, foreignTypeID := makeForeignGroup(t)
	foreignParent, err := tRepos.Entities.Create(context.Background(), foreignGID, EntityCreate{
		Name:         "foreign-parent",
		EntityTypeID: foreignTypeID,
	})
	require.NoError(t, err)

	_, err = tRepos.Entities.Create(context.Background(), tGroup.ID, EntityCreate{
		Name:         "victim",
		EntityTypeID: itemET.ID,
		ParentID:     foreignParent.ID,
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group ParentID, got %T: %v", err, err)
}

func Test_EntityCreate_RejectsForeignEntityType(t *testing.T) {
	_, _, foreignTypeID := makeForeignGroup(t)

	_, err := tRepos.Entities.Create(context.Background(), tGroup.ID, EntityCreate{
		Name:         "victim",
		EntityTypeID: foreignTypeID,
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group EntityTypeID, got %T: %v", err, err)
}

func Test_EntityCreate_RejectsForeignTag(t *testing.T) {
	itemET := useItemEntityType(t)
	_, foreignTagID, _ := makeForeignGroup(t)

	_, err := tRepos.Entities.Create(context.Background(), tGroup.ID, EntityCreate{
		Name:         "victim",
		EntityTypeID: itemET.ID,
		TagIDs:       []uuid.UUID{foreignTagID},
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group TagIDs, got %T: %v", err, err)
}

func Test_EntityPatch_RejectsForeignTag(t *testing.T) {
	itemET := useItemEntityType(t)
	_, foreignTagID, _ := makeForeignGroup(t)

	// Create a victim entity inside tGroup first.
	victim, err := tRepos.Entities.Create(context.Background(), tGroup.ID, EntityCreate{
		Name:         "victim-patch",
		EntityTypeID: itemET.ID,
	})
	require.NoError(t, err)

	err = tRepos.Entities.Patch(context.Background(), tGroup.ID, victim.ID, EntityPatch{
		TagIDs: []uuid.UUID{foreignTagID},
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group tag in Patch, got %T: %v", err, err)
}

func Test_EntityUpdate_RejectsForeignTargetBeforeSideEffects(t *testing.T) {
	ctx := context.Background()
	itemET := useItemEntityType(t)
	foreignGID, _, foreignTypeID := makeForeignGroup(t)
	foreign, err := tRepos.Entities.Create(ctx, foreignGID, EntityCreate{
		Name:         "foreign-update-target",
		EntityTypeID: foreignTypeID,
	})
	require.NoError(t, err)

	out, err := tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, EntityUpdate{
		ID:           foreign.ID,
		Name:         "should-not-be-visible-or-written",
		Quantity:     1,
		EntityTypeID: itemET.ID,
		Fields: []EntityFieldData{{
			Type:      "text",
			Name:      "should-not-exist",
			TextValue: "secret",
		}},
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group update target, got %T: %v", err, err)
	assert.Equal(t, uuid.Nil, out.ID, "foreign entity must not be returned")

	got, err := tRepos.Entities.GetOneByGroup(ctx, foreignGID, foreign.ID)
	require.NoError(t, err)
	assert.Equal(t, "foreign-update-target", got.Name)
	assert.Empty(t, got.Fields, "rejected update must not create fields on the foreign row")
}

func Test_EntityPatch_RejectsForeignTarget(t *testing.T) {
	ctx := context.Background()
	foreignGID, _, foreignTypeID := makeForeignGroup(t)
	foreign, err := tRepos.Entities.Create(ctx, foreignGID, EntityCreate{
		Name:         "foreign-patch-target",
		Quantity:     1,
		EntityTypeID: foreignTypeID,
	})
	require.NoError(t, err)

	quantity := 7.0
	err = tRepos.Entities.Patch(ctx, tGroup.ID, foreign.ID, EntityPatch{Quantity: &quantity})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group patch target, got %T: %v", err, err)

	got, err := tRepos.Entities.GetOneByGroup(ctx, foreignGID, foreign.ID)
	require.NoError(t, err)
	assert.InDelta(t, 1, got.Quantity, 0.000001)
}

func Test_EntityContainerUpdate_RejectsForeignTargetWithoutDisclosure(t *testing.T) {
	ctx := context.Background()
	foreignGID, _, _ := makeForeignGroup(t)
	foreignContainerType, err := tRepos.EntityTypes.GetDefault(ctx, foreignGID, true)
	require.NoError(t, err)
	foreign, err := tRepos.Entities.CreateContainer(ctx, foreignGID, EntityCreate{
		Name:         "foreign-container",
		EntityTypeID: foreignContainerType.ID,
	})
	require.NoError(t, err)

	out, err := tRepos.Entities.UpdateContainer(ctx, tGroup.ID, foreign.ID, EntityUpdate{
		Name: "should-not-be-visible-or-written",
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group container target, got %T: %v", err, err)
	assert.Equal(t, uuid.Nil, out.ID, "foreign container must not be returned")

	got, err := tRepos.Entities.GetContainerByGroup(ctx, foreignGID, foreign.ID)
	require.NoError(t, err)
	assert.Equal(t, "foreign-container", got.Name)
}

func Test_EntityTypeUpdate_RejectsForeignTargetWithoutDisclosure(t *testing.T) {
	ctx := context.Background()
	foreignGID, _, foreignTypeID := makeForeignGroup(t)

	out, err := tRepos.EntityTypes.Update(ctx, tGroup.ID, EntityTypeUpdate{
		ID:   foreignTypeID,
		Name: "should-not-be-visible-or-written",
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group entity type, got %T: %v", err, err)
	assert.Equal(t, uuid.Nil, out.ID, "foreign entity type must not be returned")

	types, err := tRepos.EntityTypes.GetAll(ctx, foreignGID)
	require.NoError(t, err)
	foreignTypeFound := false
	for _, entityType := range types {
		if entityType.ID == foreignTypeID {
			foreignTypeFound = true
			assert.NotEqual(t, "should-not-be-visible-or-written", entityType.Name)
		}
	}
	assert.True(t, foreignTypeFound)
}

func Test_MaintEntryCreate_RejectsForeignItem(t *testing.T) {
	itemET := useItemEntityType(t)
	foreignGID, _, _ := makeForeignGroup(t)
	foreignType, err := tRepos.EntityTypes.GetDefault(context.Background(), foreignGID, false)
	require.NoError(t, err)

	foreignItem, err := tRepos.Entities.Create(context.Background(), foreignGID, EntityCreate{
		Name:         "foreign-item",
		EntityTypeID: foreignType.ID,
	})
	require.NoError(t, err)

	// Caller is in tGroup but supplies a foreign item ID — must be rejected
	// before any maintenance row is written against B's item.
	_, err = tRepos.MaintEntry.Create(context.Background(), tGroup.ID, foreignItem.ID, MaintenanceEntryCreate{
		CompletedDate: nowDate(),
		Name:          "should-be-rejected",
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group MaintEntry.Create, got %T: %v", err, err)

	// Confirm the legitimate tGroup item path still works.
	ownItem, err := tRepos.Entities.Create(context.Background(), tGroup.ID, EntityCreate{
		Name:         "own-item",
		EntityTypeID: itemET.ID,
	})
	require.NoError(t, err)
	_, err = tRepos.MaintEntry.Create(context.Background(), tGroup.ID, ownItem.ID, MaintenanceEntryCreate{
		CompletedDate: nowDate(),
		Name:          "legit",
	})
	require.NoError(t, err)
}

func Test_EntityTemplateCreate_RejectsForeignDefaultLocation(t *testing.T) {
	foreignGID, _, _ := makeForeignGroup(t)
	foreignContainerType, err := tRepos.EntityTypes.GetDefault(context.Background(), foreignGID, true)
	require.NoError(t, err)
	foreignLoc, err := tRepos.Entities.Create(context.Background(), foreignGID, EntityCreate{
		Name:         "foreign-loc",
		EntityTypeID: foreignContainerType.ID,
	})
	require.NoError(t, err)

	_, err = tRepos.EntityTemplates.Create(context.Background(), tGroup.ID, EntityTemplateCreate{
		Name:              "victim-template",
		DefaultLocationID: foreignLoc.ID,
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group DefaultLocationID, got %T: %v", err, err)
}

func Test_EntityTemplateCreateAndUpdate_RejectForeignDefaultTags(t *testing.T) {
	ctx := context.Background()
	_, foreignTagID, _ := makeForeignGroup(t)
	foreignTags := []uuid.UUID{foreignTagID}

	_, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, EntityTemplateCreate{
		Name:          "foreign-tag-template",
		DefaultTagIDs: &foreignTags,
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group DefaultTagIDs, got %T: %v", err, err)

	template, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, EntityTemplateCreate{
		Name: "own-template",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, template.ID)
	})

	_, err = tRepos.EntityTemplates.Update(ctx, tGroup.ID, EntityTemplateUpdate{
		ID:            template.ID,
		Name:          "should-not-be-written",
		DefaultTagIDs: &foreignTags,
	})
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected NotFound for cross-group DefaultTagIDs, got %T: %v", err, err)

	got, err := tRepos.EntityTemplates.GetOne(ctx, tGroup.ID, template.ID)
	require.NoError(t, err)
	assert.Equal(t, "own-template", got.Name)
	assert.Empty(t, got.DefaultTags)
}

func Test_EntityTemplateGetOne_DoesNotExposeLegacyForeignDefaultTags(t *testing.T) {
	ctx := context.Background()
	_, foreignTagID, _ := makeForeignGroup(t)

	template, err := tClient.EntityTemplate.Create().
		SetName("legacy-corrupt-template").
		SetGroupID(tGroup.ID).
		SetDefaultTagIds([]uuid.UUID{foreignTagID}).
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, template.ID)
	})

	got, err := tRepos.EntityTemplates.GetOne(ctx, tGroup.ID, template.ID)
	require.NoError(t, err)
	assert.Empty(t, got.DefaultTags, "legacy foreign tag IDs must never expose foreign tag names")
}

func Test_HierarchyQueries_DoNotTraverseLegacyCrossGroupEdges(t *testing.T) {
	ctx := context.Background()
	ownLocationType := useContainerEntityType(t)
	ownItemType := useItemEntityType(t)
	ownLocation, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "own-location",
		Quantity:     1,
		EntityTypeID: ownLocationType.ID,
	})
	require.NoError(t, err)
	ownItem, err := tRepos.Entities.Create(ctx, tGroup.ID, EntityCreate{
		Name:         "own-item",
		Quantity:     1,
		EntityTypeID: ownItemType.ID,
	})
	require.NoError(t, err)

	foreignGID, _, foreignItemTypeID := makeForeignGroup(t)
	foreignLocationType, err := tRepos.EntityTypes.GetDefault(ctx, foreignGID, true)
	require.NoError(t, err)
	foreignLocation, err := tRepos.Entities.Create(ctx, foreignGID, EntityCreate{
		Name:         "foreign-location-secret",
		Quantity:     1,
		EntityTypeID: foreignLocationType.ID,
	})
	require.NoError(t, err)
	foreignItem, err := tRepos.Entities.Create(ctx, foreignGID, EntityCreate{
		Name:         "foreign-item-secret",
		Quantity:     7,
		EntityTypeID: foreignItemTypeID,
	})
	require.NoError(t, err)

	// Simulate legacy corruption from the historical cross-tenant parent IDOR:
	// an own row points to a foreign parent, and a foreign row points to an own
	// container. Current write APIs reject both shapes.
	_, err = tClient.Entity.UpdateOneID(ownItem.ID).SetParentID(foreignLocation.ID).Save(ctx)
	require.NoError(t, err)
	_, err = tClient.Entity.UpdateOneID(foreignItem.ID).SetParentID(ownLocation.ID).Save(ctx)
	require.NoError(t, err)

	path, err := tRepos.Entities.PathForEntity(ctx, tGroup.ID, ownItem.ID)
	require.NoError(t, err)
	require.Len(t, path, 1)
	assert.Equal(t, ownItem.ID, path[0].ID)

	tree, err := tRepos.Entities.Tree(ctx, tGroup.ID, TreeQuery{WithItems: true})
	require.NoError(t, err)
	var treeIDs []uuid.UUID
	var walk func([]*TreeItem)
	walk = func(items []*TreeItem) {
		for _, item := range items {
			treeIDs = append(treeIDs, item.ID)
			walk(item.Children)
		}
	}
	for i := range tree {
		treeIDs = append(treeIDs, tree[i].ID)
		walk(tree[i].Children)
	}
	assert.NotContains(t, treeIDs, foreignItem.ID)

	containers, err := tRepos.Entities.GetAllContainers(ctx, tGroup.ID, ContainerQuery{})
	require.NoError(t, err)
	for _, container := range containers {
		if container.ID == ownLocation.ID {
			assert.Zero(t, container.ItemCount, "foreign children must not affect own container counts")
		}
	}
}
