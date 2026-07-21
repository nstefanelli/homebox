package services

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gocloud.dev/blob"

	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/attachment"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entity"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entityfield"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entitytype"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/group"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/predicate"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/tag"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

func tagInGroup(gid uuid.UUID) predicate.Tag {
	return tag.HasGroupWith(group.ID(gid))
}

// TestExportRoundTrip writes some entities into a fresh source group, runs
// the export to produce a zip artifact, and then replays that artifact into
// a separate empty destination group. Counts and selected fields are
// asserted on the destination side.
//
// This is the load-bearing integration test for the raw-SQL dump/restore
// path: anything that doesn't round-trip cleanly (timestamps, UUIDs, JSON
// columns, self-referential FKs) shows up here.
func TestExportRoundTrip(t *testing.T) {
	ctx := context.Background()

	// --- Source group with data ----------------------------------------
	src, err := tRepos.Groups.GroupCreate(ctx, "export-src-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)

	containerET, err := tRepos.EntityTypes.GetDefault(ctx, src.ID, true)
	require.NoError(t, err)
	itemET, err := tRepos.EntityTypes.GetDefault(ctx, src.ID, false)
	require.NoError(t, err)

	// One location, one item nested in it.
	loc, err := tRepos.Entities.Create(ctx, src.ID, repo.EntityCreate{
		Name:         defaultLocationGarage,
		Description:  "primary",
		EntityTypeID: containerET.ID,
	})
	require.NoError(t, err)

	item, err := tRepos.Entities.Create(ctx, src.ID, repo.EntityCreate{
		Name:         "Drill",
		Description:  "cordless",
		ParentID:     loc.ID,
		EntityTypeID: itemET.ID,
	})
	require.NoError(t, err)

	// Tag and link to the item (exercises the tag_entities junction).
	tg, err := tRepos.Tags.Create(ctx, src.ID, repo.TagCreate{
		Name:        "tools",
		Description: "stuff that hits other stuff",
	})
	require.NoError(t, err)
	_, err = tClient.Entity.UpdateOneID(item.ID).AddTagIDs(tg.ID).Save(ctx)
	require.NoError(t, err)

	defaultTagIDs := []uuid.UUID{tg.ID}
	template, err := tRepos.EntityTemplates.Create(ctx, src.ID, repo.EntityTemplateCreate{
		Name:          "Tool template",
		DefaultTagIDs: &defaultTagIDs,
	})
	require.NoError(t, err)
	templatePhotoBytes := []byte("template photo blob")
	templatePhotoUpload, err := tRepos.Attachments.UploadFileByGroupID(ctx, src.ID, repo.ItemCreateAttachment{
		Title:   "template-photo.jpg",
		Content: bytes.NewReader(templatePhotoBytes),
	})
	require.NoError(t, err)
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(
		ctx,
		src.ID,
		template.ID,
		templatePhotoUpload.Path,
		templatePhotoUpload.ContentType,
	))

	// Real attachment + a fabricated thumbnail row pointing at it.
	// This is the scenario that broke before: the thumbnail row has
	// entity_attachments=NULL and is reachable only via the parent's
	// attachment_thumbnail FK, so the original entity-only scope missed it.
	parentAtt, err := tRepos.Attachments.Create(ctx, item.ID,
		repo.ItemCreateAttachment{
			Title:   "manual.pdf",
			Content: bytes.NewReader([]byte("dummy pdf body")),
		},
		attachment.TypeManual, false)
	require.NoError(t, err)

	srcGroup, err := tClient.Group.Get(ctx, src.ID)
	require.NoError(t, err)
	thumbUpload, err := tRepos.Attachments.UploadFile(ctx, srcGroup,
		repo.ItemCreateAttachment{
			Title:   "manual-thumb",
			Content: bytes.NewReader([]byte("dummy thumbnail body")),
		})
	require.NoError(t, err)
	thumbAtt, err := tClient.Attachment.Create().
		SetType(attachment.TypeThumbnail).
		SetTitle("manual-thumb").
		SetPath(thumbUpload.Path).
		SetMimeType("image/webp").
		Save(ctx)
	require.NoError(t, err)
	_, err = tClient.Attachment.UpdateOneID(parentAtt.ID).SetThumbnailID(thumbAtt.ID).Save(ctx)
	require.NoError(t, err)

	// --- Export --------------------------------------------------------
	expRow, err := tRepos.Exports.Create(ctx, src.ID)
	require.NoError(t, err)

	artifactPath, sizeBytes, err := tSvc.Exports.buildArtifact(ctx, expRow.ID, src.ID)
	require.NoError(t, err)
	require.NotEmpty(t, artifactPath)
	require.Positive(t, sizeBytes)

	// Artifact must live under the source group's prefix.
	assert.True(t, strings.HasPrefix(artifactPath, src.ID.String()+"/exports/"),
		"artifact path %q must be scoped to source group", artifactPath)

	// --- Destination: fresh group with seeded defaults -----------------
	// Run the real registration flow so the destination carries the actual
	// seeder output (locations created via CreateContainer with quantity at
	// the schema default of 1 and sequential asset IDs, plus default tags).
	// Hand-rolled lookalike rows previously masked a readiness-check bug
	// that rejected every genuinely registered collection.
	dstUser, err := tSvc.User.RegisterUser(ctx, UserRegistration{
		Name:     "Export Dst User",
		Email:    fk.Email(),
		Password: "export-dst-password",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.Users.Delete(context.Background(), dstUser.ID)
		_ = tRepos.Groups.GroupDelete(context.Background(), dstUser.DefaultGroupID)
	})
	dst, err := tRepos.Groups.GroupByID(ctx, dstUser.DefaultGroupID)
	require.NoError(t, err)

	ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, dst.ID)
	require.NoError(t, err)
	require.True(t, ready, "dst group with only seeded defaults must be importable")

	// Stage the just-built artifact as if it had been uploaded for import.
	// We re-publish it under the destination's import prefix to satisfy the
	// worker's scope check.
	importKey := dst.ID.String() + "/imports/" + uuid.New().String() + ".zip"
	require.NoError(t, copyBlobUnderTest(ctx, tSvc.Exports, artifactPath, importKey))

	// Create the tracked import row the worker reads to find the upload key
	// and to report status/progress against.
	impRow, err := tRepos.Exports.CreateImport(ctx, dst.ID, importKey, sizeBytes)
	require.NoError(t, err)
	tSvc.Exports.RunImport(ctx, dst.ID, dstUser.ID, impRow.ID)

	// --- Assertions ----------------------------------------------------
	dstEntities, err := tClient.Entity.Query().Where(entity.HasGroupWith(group.ID(dst.ID))).All(ctx)
	require.NoError(t, err)
	require.Len(t, dstEntities, 2, "exactly the location and the item should remain — seeded defaults wiped, source data restored")

	gotItem, err := tClient.Entity.Query().
		Where(entity.HasGroupWith(group.ID(dst.ID)), entity.Name("Drill")).
		Only(ctx)
	require.NoError(t, err)

	parent, err := gotItem.QueryParent().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, defaultLocationGarage, parent.Name, "parent FK must be restored on second pass")

	tags, err := gotItem.QueryTag().All(ctx)
	require.NoError(t, err)
	require.Len(t, tags, 1, "tag_entities junction must round-trip")
	assert.Equal(t, "tools", tags[0].Name)

	// Seeded tags must be gone — only the imported "tools" tag should remain.
	allTags, err := tClient.Tag.Query().Where(tagInGroup(dst.ID)).All(ctx)
	require.NoError(t, err)
	require.Len(t, allTags, 1, "seeded tags should have been wiped")
	assert.Equal(t, "tools", allTags[0].Name)

	importedTemplates, err := tRepos.EntityTemplates.GetAll(ctx, dst.ID)
	require.NoError(t, err)
	require.Len(t, importedTemplates, 1)
	importedTemplate, err := tRepos.EntityTemplates.GetOne(ctx, dst.ID, importedTemplates[0].ID)
	require.NoError(t, err)
	require.Len(t, importedTemplate.DefaultTags, 1)
	assert.Equal(t, "tools", importedTemplate.DefaultTags[0].Name)
	assert.True(t, strings.HasPrefix(importedTemplate.PhotoPath, dst.ID.String()+"/documents/"))
	assert.NotEqual(t, templatePhotoUpload.Path, importedTemplate.PhotoPath)

	// IDs are intentionally regenerated on import (so re-importing the same
	// archive into a server that already has the data doesn't conflict on
	// PK). Names + relationship structure are what matters.
	assert.NotEqual(t, item.ID, gotItem.ID, "import should remap PKs")
	assert.NotEqual(t, tg.ID, tags[0].ID, "import should remap PKs")

	// Attachment + thumbnail must both round-trip with the parent→thumbnail
	// link intact and both blobs present at their new on-disk paths.
	gotAtts, err := gotItem.QueryAttachments().All(ctx)
	require.NoError(t, err)
	require.Len(t, gotAtts, 1, "parent attachment row must round-trip")

	gotThumb, err := gotAtts[0].QueryThumbnail().Only(ctx)
	require.NoError(t, err, "parent attachment must have its thumbnail edge restored")
	assert.Equal(t, "image/webp", gotThumb.MimeType)

	// Imported paths must be rewritten to the destination group's prefix —
	// otherwise the DB would point at the source group and on-delete cascade
	// would leak blobs.
	dstPrefix := dst.ID.String() + "/"
	assert.True(t, strings.HasPrefix(gotAtts[0].Path, dstPrefix),
		"parent attachment path must point at dst group (got %q)", gotAtts[0].Path)
	assert.True(t, strings.HasPrefix(gotThumb.Path, dstPrefix),
		"thumbnail path must point at dst group (got %q)", gotThumb.Path)
	assert.NotContains(t, gotAtts[0].Path, src.ID.String(),
		"source gid must not appear anywhere in the imported path")

	bk, err := blob.OpenBucket(ctx, tRepos.Attachments.GetConnString())
	require.NoError(t, err)
	defer func() { _ = bk.Close() }()

	parentBlob, err := bk.ReadAll(ctx, tRepos.Attachments.GetFullPath(gotAtts[0].Path))
	require.NoError(t, err, "parent attachment blob must be present at the rewritten path")
	assert.Equal(t, "dummy pdf body", string(parentBlob))

	thumbBlob, err := bk.ReadAll(ctx, tRepos.Attachments.GetFullPath(gotThumb.Path))
	require.NoError(t, err, "thumbnail blob must be present at the rewritten path")
	assert.Equal(t, "dummy thumbnail body", string(thumbBlob))

	templateBlob, err := bk.ReadAll(ctx, tRepos.Attachments.GetFullPath(importedTemplate.PhotoPath))
	require.NoError(t, err, "template photo blob must be restored at the rewritten path")
	assert.Equal(t, templatePhotoBytes, templateBlob)
}

func TestCSVExportImportPreservesItemParent(t *testing.T) {
	ctx := context.Background()

	src, err := tRepos.Groups.GroupCreate(ctx, "csv-parent-src-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)

	locationType, err := tRepos.EntityTypes.GetDefault(ctx, src.ID, true)
	require.NoError(t, err)
	itemType, err := tRepos.EntityTypes.GetDefault(ctx, src.ID, false)
	require.NoError(t, err)

	location, err := tRepos.Entities.Create(ctx, src.ID, repo.EntityCreate{
		ImportRef:    "loc-ref",
		Name:         defaultLocationGarage,
		EntityTypeID: locationType.ID,
	})
	require.NoError(t, err)

	parent, err := tRepos.Entities.Create(ctx, src.ID, repo.EntityCreate{
		ImportRef:    "parent-ref",
		Name:         "Toolbox",
		ParentID:     location.ID,
		EntityTypeID: itemType.ID,
	})
	require.NoError(t, err)

	_, err = tRepos.Entities.Create(ctx, src.ID, repo.EntityCreate{
		ImportRef:    "child-ref",
		Name:         "Screwdriver",
		ParentID:     parent.ID,
		EntityTypeID: itemType.ID,
	})
	require.NoError(t, err)

	rows, err := tSvc.Entities.ExportCSV(ctx, src.ID, "https://homebox.example")
	require.NoError(t, err)

	header := rows[0]
	col := func(name string) int {
		t.Helper()
		for i, h := range header {
			if h == name {
				return i
			}
		}
		require.FailNowf(t, "missing CSV column", "column %q not found in %v", name, header)
		return -1
	}

	nameCol := col("HB.name")
	parentRefCol := col("HB.parent_import_ref")
	locationCol := col("HB.location")

	var childRow []string
	for _, row := range rows[1:] {
		if row[nameCol] == "Screwdriver" {
			childRow = row
			break
		}
	}
	require.NotNil(t, childRow)
	assert.Equal(t, "parent-ref", childRow[parentRefCol])
	assert.Equal(t, defaultLocationGarage, childRow[locationCol])

	importRows := [][]string{header}
	for _, row := range rows[1:] {
		if row[nameCol] != defaultLocationGarage {
			importRows = append(importRows, row)
		}
	}

	var csvBuf bytes.Buffer
	writer := csv.NewWriter(&csvBuf)
	require.NoError(t, writer.WriteAll(importRows))
	require.NoError(t, writer.Error())

	dst, err := tRepos.Groups.GroupCreate(ctx, "csv-parent-dst-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)

	imported, err := tSvc.Entities.CsvImport(ctx, dst.ID, bytes.NewReader(csvBuf.Bytes()))
	require.NoError(t, err)
	require.Equal(t, len(importRows)-1, imported)

	importedChild, err := tRepos.Entities.GetByRef(ctx, dst.ID, "child-ref")
	require.NoError(t, err)
	require.NotNil(t, importedChild.Parent)
	assert.Equal(t, "Toolbox", importedChild.Parent.Name)
}

func TestCSVImportRejectsSelfParentRef(t *testing.T) {
	ctx := context.Background()

	dst, err := tRepos.Groups.GroupCreate(ctx, "csv-self-parent-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)

	csvData := strings.Join([]string{
		"HB.import_ref,HB.parent_import_ref,HB.location,HB.name",
		"self-ref,self-ref," + defaultLocationGarage + ",Loop",
		"",
	}, "\n")

	_, err = tSvc.Entities.CsvImport(ctx, dst.ID, strings.NewReader(csvData))
	require.Error(t, err)
	assert.ErrorContains(t, err, `entity "self-ref" cannot be its own parent`)
}

type csvImportGroupCounts struct {
	Entities  int
	Locations int
	Tags      int
	Types     int
	Fields    int
}

func getCSVImportGroupCounts(t *testing.T, gid uuid.UUID) csvImportGroupCounts {
	t.Helper()
	ctx := context.Background()

	entities, err := tClient.Entity.Query().
		Where(entity.HasGroupWith(group.ID(gid))).
		Count(ctx)
	require.NoError(t, err)
	locations, err := tClient.Entity.Query().
		Where(
			entity.HasGroupWith(group.ID(gid)),
			entity.HasEntityTypeWith(
				entitytype.IsLocation(true),
				entitytype.HasGroupWith(group.ID(gid)),
			),
		).
		Count(ctx)
	require.NoError(t, err)
	tags, err := tClient.Tag.Query().
		Where(tag.HasGroupWith(group.ID(gid))).
		Count(ctx)
	require.NoError(t, err)
	types, err := tClient.EntityType.Query().
		Where(entitytype.HasGroupWith(group.ID(gid))).
		Count(ctx)
	require.NoError(t, err)
	fields, err := tClient.EntityField.Query().
		Where(entityfield.HasEntityWith(entity.HasGroupWith(group.ID(gid)))).
		Count(ctx)
	require.NoError(t, err)

	return csvImportGroupCounts{
		Entities:  entities,
		Locations: locations,
		Tags:      tags,
		Types:     types,
		Fields:    fields,
	}
}

func TestCSVImportRollsBackLateParentReferenceFailure(t *testing.T) {
	ctx := context.Background()
	dst, err := tRepos.Groups.GroupCreate(ctx, "csv-parent-rollback-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)
	before := getCSVImportGroupCounts(t, dst.ID)

	csvData := strings.Join([]string{
		"HB.import_ref,HB.parent_import_ref,HB.location,HB.tags,HB.name",
		"first-ref,,Rollback Room/Box,rollback-tag-a,First",
		"child-ref,missing-parent,Rollback Room/Box,rollback-tag-b,Child",
		"",
	}, "\n")

	imported, err := tSvc.Entities.CsvImport(ctx, dst.ID, strings.NewReader(csvData))
	require.Error(t, err)
	assert.Zero(t, imported)
	require.ErrorContains(t, err, `error resolving parent entity with ref "missing-parent"`)
	assert.Equal(t, before, getCSVImportGroupCounts(t, dst.ID),
		"late parent resolution must roll back row entities, locations, tags, types, and fields")
}

func TestCSVImportRollsBackFieldFailureAfterCreatingRelatedRows(t *testing.T) {
	ctx := context.Background()
	dst, err := tRepos.Groups.GroupCreate(ctx, "csv-field-rollback-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)
	before := getCSVImportGroupCounts(t, dst.ID)

	_, err = tClient.Sql().ExecContext(ctx, `
		CREATE TRIGGER csv_import_field_failure
		BEFORE INSERT ON entity_fields
		WHEN NEW.name = 'force-rollback'
		BEGIN
			SELECT RAISE(ABORT, 'forced CSV field failure');
		END;
	`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = tClient.Sql().ExecContext(
			context.Background(),
			`DROP TRIGGER IF EXISTS csv_import_field_failure`,
		)
	})

	csvData := strings.Join([]string{
		"HB.import_ref,HB.location,HB.tags,HB.name,HB.field.force-rollback",
		"field-ref,Rollback House/Shelf,rollback-field-tag,Field failure,boom",
		"",
	}, "\n")

	imported, err := tSvc.Entities.CsvImport(ctx, dst.ID, strings.NewReader(csvData))
	require.Error(t, err)
	assert.Zero(t, imported)
	require.ErrorContains(t, err, "forced CSV field failure")
	assert.Equal(t, before, getCSVImportGroupCounts(t, dst.ID),
		"a field write failure must roll back row entities, locations, tags, types, and fields")
}

// TestIsGroupReadyForImport_BlocksUserCreatedRows asserts that the import
// gate blocks not just on items but on user-created rows in any table the
// import would wipe (tags, entity_templates, notifiers, custom entity_types,
// and custom locations beyond the seeded baseline). The pure-seed and
// pure-empty cases must still pass.
func TestIsGroupReadyForImport_BlocksUserCreatedRows(t *testing.T) {
	ctx := context.Background()

	t.Run("empty group passes", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-empty-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.True(t, ready, "empty group must be importable")
	})

	t.Run("only seeded defaults passes", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-seed-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		locET, err := tRepos.EntityTypes.GetDefault(ctx, g.ID, true)
		require.NoError(t, err)
		for _, name := range []string{"Living Room", defaultLocationGarage, "Kitchen", "Bedroom", "Bathroom", "Office", "Attic", "Basement"} {
			_, err := tRepos.Entities.Create(ctx, g.ID, repo.EntityCreate{Name: name, EntityTypeID: locET.ID})
			require.NoError(t, err)
		}
		for _, name := range []string{"Appliances", "IOT", "Electronics", "Servers", "General", "Important"} {
			_, err := tRepos.Tags.Create(ctx, g.ID, repo.TagCreate{Name: name})
			require.NoError(t, err)
		}
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.True(t, ready, "full seed baseline must be importable")
	})

	t.Run("extra tag blocks", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-tag-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		for i := 0; i <= len(defaultTags()); i++ {
			_, err := tRepos.Tags.Create(ctx, g.ID, repo.TagCreate{Name: fk.Str(8)})
			require.NoError(t, err)
		}
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.False(t, ready, "tag count beyond seed baseline must block")
	})

	t.Run("single replacement tag blocks despite being below seed count", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-tag-shape-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		_, err = tRepos.Tags.Create(ctx, g.ID, repo.TagCreate{Name: "My Custom Tag"})
		require.NoError(t, err)
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.False(t, ready, "custom tag must not be mistaken for a deleted seed row")
	})

	t.Run("extra location blocks", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-loc-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		locET, err := tRepos.EntityTypes.GetDefault(ctx, g.ID, true)
		require.NoError(t, err)
		for i := 0; i <= len(defaultLocations()); i++ {
			_, err := tRepos.Entities.Create(ctx, g.ID, repo.EntityCreate{Name: fk.Str(8), EntityTypeID: locET.ID})
			require.NoError(t, err)
		}
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.False(t, ready, "location count beyond seed baseline must block")
	})

	t.Run("single replacement location blocks despite being below seed count", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-loc-shape-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		locET, err := tRepos.EntityTypes.GetDefault(ctx, g.ID, true)
		require.NoError(t, err)
		_, err = tRepos.Entities.Create(ctx, g.ID, repo.EntityCreate{Name: "My Shed", EntityTypeID: locET.ID})
		require.NoError(t, err)
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.False(t, ready, "custom location must not be mistaken for a deleted seed row")
	})

	t.Run("notifier blocks", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-not-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		_, err = tRepos.Notifiers.Create(ctx, g.ID, tUser.ID, repo.NotifierCreate{
			Name:     "n",
			URL:      "ntfy://x/topic",
			IsActive: true,
		})
		require.NoError(t, err)
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.False(t, ready, "any notifier must block")
	})

	t.Run("template blocks", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-tpl-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		_, err = tRepos.EntityTemplates.Create(ctx, g.ID, repo.EntityTemplateCreate{Name: "t"})
		require.NoError(t, err)
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.False(t, ready, "any entity template must block")
	})

	t.Run("custom entity_type blocks", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-et-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		// Trigger lazy creation of both defaults, then add a third custom type.
		_, err = tRepos.EntityTypes.GetDefault(ctx, g.ID, true)
		require.NoError(t, err)
		_, err = tRepos.EntityTypes.GetDefault(ctx, g.ID, false)
		require.NoError(t, err)
		_, err = tRepos.EntityTypes.Create(ctx, g.ID, repo.EntityTypeCreate{Name: "Custom", IsLocation: false})
		require.NoError(t, err)
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.False(t, ready, "entity_type beyond Item/Location defaults must block")
	})

	t.Run("single custom entity_type blocks despite being below seed count", func(t *testing.T) {
		g, err := tRepos.Groups.GroupCreate(ctx, "ready-et-shape-"+fk.Str(4), uuid.Nil)
		require.NoError(t, err)
		_, err = tRepos.EntityTypes.Create(ctx, g.ID, repo.EntityTypeCreate{Name: "Custom", IsLocation: false})
		require.NoError(t, err)
		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, g.ID)
		require.NoError(t, err)
		assert.False(t, ready, "custom type must not be mistaken for a missing default type")
	})
}

// TestIsGroupReadyForImport_FreshRegistration exercises the real registration
// seeder instead of hand-built lookalike rows. The seeder creates default
// locations via CreateContainer, which leaves quantity at the schema default
// of 1 and assigns each row a sequential asset ID — a shape the readiness
// check must accept. Regression: the check previously required
// Quantity == 0 && AssetID == 0, so every freshly registered collection got
// a 409 on import.
func TestIsGroupReadyForImport_FreshRegistration(t *testing.T) {
	ctx := context.Background()

	usr, err := tSvc.User.RegisterUser(ctx, UserRegistration{
		Name:     "Import Ready User",
		Email:    fk.Email(),
		Password: "import-ready-password",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.Users.Delete(context.Background(), usr.ID)
		_ = tRepos.Groups.GroupDelete(context.Background(), usr.DefaultGroupID)
	})
	gid := usr.DefaultGroupID

	ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, gid)
	require.NoError(t, err)
	assert.True(t, ready, "a freshly registered collection containing only seeder output must be importable")

	// Negative case: one real user-created item flips readiness off.
	itemET, err := tRepos.EntityTypes.GetDefault(ctx, gid, false)
	require.NoError(t, err)
	userItem, err := tRepos.Entities.Create(ctx, gid, repo.EntityCreate{
		Name:         "User Drill",
		EntityTypeID: itemET.ID,
	})
	require.NoError(t, err)

	ready, err = tSvc.Exports.IsGroupReadyForImport(ctx, gid)
	require.NoError(t, err)
	assert.False(t, ready, "a user-created item must block import")

	// A user-created location that shadows a seed name but carries real data
	// (quantity beyond the pristine defaults) must also block, even when it
	// reuses the freed in-range asset ID.
	require.NoError(t, tRepos.Entities.DeleteByGroup(ctx, gid, userItem.ID))
	locET, err := tRepos.EntityTypes.GetDefault(ctx, gid, true)
	require.NoError(t, err)
	seeded, err := tClient.Entity.Query().
		Where(entity.HasGroupWith(group.ID(gid)), entity.Name(defaultLocationGarage)).
		Only(ctx)
	require.NoError(t, err)
	require.NoError(t, tRepos.Entities.DeleteByGroup(ctx, gid, seeded.ID))
	_, err = tRepos.Entities.Create(ctx, gid, repo.EntityCreate{
		Name:         defaultLocationGarage,
		EntityTypeID: locET.ID,
		Quantity:     5,
		AssetID:      repo.AssetID(seeded.AssetID),
	})
	require.NoError(t, err)

	ready, err = tSvc.Exports.IsGroupReadyForImport(ctx, gid)
	require.NoError(t, err)
	assert.False(t, ready, "a seed-named location with a non-default quantity must block")
}

// TestIsGroupReadyForImport_BlocksRecreatedSeedNamedLocation covers the
// delete-then-recreate hole: a user deletes a seeded location and later
// creates a new, still-empty location with the same name. Per-row emptiness
// cannot tell that row from a pristine seed, but its asset_id can — the
// seeder assigns the contiguous range 1..len(defaultLocations()) on a fresh
// group, while a recreated location gets either the next counter value
// (past the seed range once the group has any real usage) or 0 (with
// auto-increment disabled), a mixed shape alongside the surviving 1..N seed
// rows. Both must block import, or the wipe would silently delete a
// user-created location.
func TestIsGroupReadyForImport_BlocksRecreatedSeedNamedLocation(t *testing.T) {
	ctx := context.Background()

	// registerGroup registers a real user (running the actual seeder),
	// deletes the seeded "Attic", and returns the group ID plus the default
	// Location entity-type ID for recreating it.
	registerGroup := func(t *testing.T) (uuid.UUID, uuid.UUID) {
		t.Helper()
		usr, err := tSvc.User.RegisterUser(ctx, UserRegistration{
			Name:     "Recreate User",
			Email:    fk.Email(),
			Password: "recreate-password",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = tRepos.Users.Delete(context.Background(), usr.ID)
			_ = tRepos.Groups.GroupDelete(context.Background(), usr.DefaultGroupID)
		})
		gid := usr.DefaultGroupID
		seeded, err := tClient.Entity.Query().
			Where(entity.HasGroupWith(group.ID(gid)), entity.Name("Attic")).
			Only(ctx)
		require.NoError(t, err)
		require.NoError(t, tRepos.Entities.DeleteByGroup(ctx, gid, seeded.ID))
		locET, err := tRepos.EntityTypes.GetDefault(ctx, gid, true)
		require.NoError(t, err)
		return gid, locET.ID
	}

	t.Run("recreated location with out-of-seed-range asset id blocks", func(t *testing.T) {
		gid, locETID := registerGroup(t)
		// Any real usage pushes the asset counter past the seed range; the
		// recreated location then gets an ID the seeder could never assign.
		_, err := tRepos.Entities.Create(ctx, gid, repo.EntityCreate{
			Name:         "Attic",
			EntityTypeID: locETID,
			Quantity:     1,
			AssetID:      42,
		})
		require.NoError(t, err)

		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, gid)
		require.NoError(t, err)
		assert.False(t, ready, "an empty seed-named location with an out-of-range asset id is user-created and must block")
	})

	t.Run("recreated location with asset id 0 alongside seeded rows blocks", func(t *testing.T) {
		gid, locETID := registerGroup(t)
		// auto_increment_asset_id=false shape: the new location gets asset 0
		// while the surviving seeds keep 1..N — a mixed shape.
		_, err := tRepos.Entities.Create(ctx, gid, repo.EntityCreate{
			Name:         "Attic",
			EntityTypeID: locETID,
			Quantity:     1,
		})
		require.NoError(t, err)

		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, gid)
		require.NoError(t, err)
		assert.False(t, ready, "asset id 0 mixed with seeded 1..N rows must block")
	})

	t.Run("recreated location duplicating a seeded asset id blocks", func(t *testing.T) {
		gid, locETID := registerGroup(t)
		other, err := tClient.Entity.Query().
			Where(entity.HasGroupWith(group.ID(gid)), entity.Name("Kitchen")).
			Only(ctx)
		require.NoError(t, err)
		_, err = tRepos.Entities.Create(ctx, gid, repo.EntityCreate{
			Name:         "Attic",
			EntityTypeID: locETID,
			Quantity:     1,
			AssetID:      repo.AssetID(other.AssetID),
		})
		require.NoError(t, err)

		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, gid)
		require.NoError(t, err)
		assert.False(t, ready, "duplicate asset ids among seed candidates must block")
	})

	t.Run("recreated location just past the seed range blocks", func(t *testing.T) {
		gid, locETID := registerGroup(t)
		_, err := tRepos.Entities.Create(ctx, gid, repo.EntityCreate{
			Name:         "Attic",
			EntityTypeID: locETID,
			Quantity:     1,
			AssetID:      repo.AssetID(len(defaultLocations()) + 1),
		})
		require.NoError(t, err)

		ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, gid)
		require.NoError(t, err)
		assert.False(t, ready, "the first asset id past the seed range must block")
	})
}

// A legacy group (seeded at asset 0 / quantity 0) whose locations were later
// backfilled by EnsureAssetID carries asset IDs 1..N with quantity still 0.
// seedLocationNumericShapeOK's doc comment claims that shape stays
// importable; pin it so a refactor can't silently regress it.
func TestIsGroupReadyForImport_AcceptsBackfilledLegacySeeds(t *testing.T) {
	ctx := context.Background()
	usr, err := tSvc.User.RegisterUser(ctx, UserRegistration{
		Name:     "Backfill User",
		Email:    fk.Email(),
		Password: "backfill-password",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.Users.Delete(context.Background(), usr.ID)
		_ = tRepos.Groups.GroupDelete(context.Background(), usr.DefaultGroupID)
	})
	gid := usr.DefaultGroupID

	// Shape the seeds into backfilled-legacy form: EnsureAssetID assigns
	// 1..N (already true of the current seeder) but never touches quantity,
	// which legacy seeders left at 0.
	_, err = tClient.Entity.Update().
		Where(entity.HasGroupWith(group.ID(gid))).
		SetQuantity(0).
		Save(ctx)
	require.NoError(t, err)

	ready, err := tSvc.Exports.IsGroupReadyForImport(ctx, gid)
	require.NoError(t, err)
	assert.True(t, ready, "backfilled legacy seeds (asset 1..N, quantity 0) must stay importable")
}

func testZipReader(t *testing.T, entries map[string]any) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, value := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(w).Encode(value))
	}
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	return zr
}

func TestReplayImportRowsRejectsUnmappedForeignTenantFK(t *testing.T) {
	ctx := context.Background()
	foreign, err := tRepos.Groups.GroupCreate(ctx, "foreign-fk-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)
	foreignType, err := tRepos.EntityTypes.GetDefault(ctx, foreign.ID, false)
	require.NoError(t, err)

	dst, err := tRepos.Groups.GroupCreate(ctx, "foreign-fk-dst-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)
	srcGID := uuid.New()
	zr := testZipReader(t, map[string]any{
		"entities.json": []map[string]any{{
			"id":                   uuid.NewString(),
			"group_entities":       srcGID.String(),
			"entity_type_entities": foreignType.ID.String(),
		}},
	})

	tx, err := tClient.Sql().BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tSvc.Exports.replayImportRows(ctx, tx, zr, dst.ID, tUser.ID, srcGID)
	require.Error(t, err)
	assert.ErrorContains(t, err, "unmapped foreign key")
}

func TestEnforceZipUncompressedLimitUsesAbsoluteAndJSONCaps(t *testing.T) {
	t.Run("absolute total cap", func(t *testing.T) {
		zr := &zip.Reader{File: []*zip.File{{
			FileHeader: zip.FileHeader{
				Name:               attachmentsDir + "blob",
				UncompressedSize64: maxImportUncompressedBytes + 1,
			},
		}}}
		err := enforceZipUncompressedLimit(zr, 1<<30)
		require.Error(t, err)
		assert.ErrorContains(t, err, "exceeds limit")
	})

	t.Run("json entry cap", func(t *testing.T) {
		zr := &zip.Reader{File: []*zip.File{{
			FileHeader: zip.FileHeader{
				Name:               "entities.json",
				UncompressedSize64: maxImportJSONEntryBytes + 1,
			},
		}}}
		err := enforceZipUncompressedLimit(zr, 1<<30)
		require.Error(t, err)
		assert.ErrorContains(t, err, "JSON entry")
	})

	t.Run("large upload arithmetic cannot overflow ratio", func(t *testing.T) {
		zr := &zip.Reader{File: []*zip.File{{
			FileHeader: zip.FileHeader{
				Name:               attachmentsDir + "blob",
				UncompressedSize64: maxImportUncompressedBytes + 1,
			},
		}}}
		err := enforceZipUncompressedLimit(zr, int64(^uint64(0)>>1))
		require.Error(t, err)
		assert.ErrorContains(t, err, "exceeds limit")
	})
}

func TestValidateTemplatePhotoArchiveRejectsMissingBlob(t *testing.T) {
	templateID := uuid.New()
	zr := testZipReader(t, map[string]any{
		"entity_templates.json": []map[string]any{{
			"id":         templateID.String(),
			"photo_path": uuid.NewString() + "/documents/blob",
		}},
	})
	err := validateTemplatePhotoArchive(zr)
	require.Error(t, err)
	assert.ErrorContains(t, err, "blob is missing")
}

func TestValidateAttachmentBlobArchiveRejectsMissingBlob(t *testing.T) {
	attachmentID := uuid.New()
	zr := testZipReader(t, map[string]any{
		"attachments.json": []map[string]any{{
			"id":        attachmentID.String(),
			"path":      uuid.NewString() + "/documents/blob",
			"mime_type": "application/pdf",
		}},
	})

	err := validateAttachmentBlobArchive(zr)
	require.Error(t, err)
	assert.ErrorContains(t, err, "blob is missing")
}

func TestRewriteTemplatePhotoPathRejectsTraversal(t *testing.T) {
	src := uuid.New()
	dst := uuid.New()
	row := map[string]any{
		"photo_path": src.String() + "/documents/../secrets/blob",
	}
	err := rewriteTemplatePhotoPath(row, src, dst)
	require.Error(t, err)
	assert.ErrorContains(t, err, "canonical")
}

func TestCopyTemplatePhotoBlobsFailsWhenReferencedBlobIsMissing(t *testing.T) {
	ctx := context.Background()
	g, err := tRepos.Groups.GroupCreate(ctx, "missing-template-photo-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)
	tmpl, err := tRepos.EntityTemplates.Create(ctx, g.ID, repo.EntityTemplateCreate{Name: "missing photo"})
	require.NoError(t, err)
	missingPath := g.ID.String() + "/documents/" + uuid.NewString()
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, g.ID, tmpl.ID, missingPath, "image/png"))

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err = tSvc.Exports.copyTemplatePhotoBlobs(ctx, zw, g.ID)
	require.Error(t, err)
	require.ErrorContains(t, err, "template")
	_ = zw.Close()
}

func TestCopyAttachmentBlobsFailsWhenReferencedBlobIsMissing(t *testing.T) {
	ctx := context.Background()
	g, err := tRepos.Groups.GroupCreate(ctx, "missing-attachment-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)
	itemType, err := tRepos.EntityTypes.GetDefault(ctx, g.ID, false)
	require.NoError(t, err)
	item, err := tRepos.Entities.Create(ctx, g.ID, repo.EntityCreate{
		Name:         "missing attachment item",
		EntityTypeID: itemType.ID,
	})
	require.NoError(t, err)

	missingPath := g.ID.String() + "/documents/" + uuid.NewString()
	_, err = tClient.Attachment.Create().
		SetType(attachment.TypeManual).
		SetTitle("missing.pdf").
		SetPath(missingPath).
		SetMimeType("application/pdf").
		SetEntityID(item.ID).
		Save(ctx)
	require.NoError(t, err)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err = tSvc.Exports.copyAttachmentBlobs(ctx, zw, g.ID)
	require.Error(t, err)
	require.ErrorContains(t, err, "blob")
	_ = zw.Close()
}

func TestCopyExactSize(t *testing.T) {
	var dst bytes.Buffer
	n, err := copyExactSize(&dst, strings.NewReader("abc"), 3)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)

	dst.Reset()
	_, err = copyExactSize(&dst, strings.NewReader("ab"), 3)
	require.Error(t, err)

	dst.Reset()
	_, err = copyExactSize(&dst, strings.NewReader("abcd"), 3)
	require.Error(t, err)
}

func TestCopyDeclaredZipEntryRejectsSizeMismatch(t *testing.T) {
	var dst bytes.Buffer
	require.NoError(t, copyDeclaredZipEntry(&dst, strings.NewReader("abc"), 3))
	assert.Equal(t, "abc", dst.String())

	dst.Reset()
	err := copyDeclaredZipEntry(&dst, strings.NewReader("abcd"), 3)
	require.ErrorContains(t, err, "exceeds declared size")

	dst.Reset()
	err = copyDeclaredZipEntry(&dst, strings.NewReader("ab"), 3)
	require.ErrorContains(t, err, "size mismatch")

	err = copyDeclaredZipEntry(&dst, strings.NewReader(""), math.MaxUint64)
	require.ErrorContains(t, err, "unsupported")
}

func TestRunImportMarksRedeliveredRunningJobFailedAndRetainsUpload(t *testing.T) {
	ctx := context.Background()
	g, err := tRepos.Groups.GroupCreate(ctx, "running-import-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)

	uploadKey := g.ID.String() + "/imports/" + uuid.NewString() + ".zip"
	bucket, err := blob.OpenBucket(ctx, tRepos.Attachments.GetConnString())
	require.NoError(t, err)
	defer func() { _ = bucket.Close() }()
	fullPath := tRepos.Attachments.GetFullPath(uploadKey)
	require.NoError(t, bucket.WriteAll(ctx, fullPath, []byte("staged"), nil))
	t.Cleanup(func() { _ = bucket.Delete(context.Background(), fullPath) })

	row, err := tRepos.Exports.CreateImport(ctx, g.ID, uploadKey, int64(len("staged")))
	require.NoError(t, err)
	require.NoError(t, tRepos.Exports.SetRunning(ctx, g.ID, row.ID))

	tSvc.Exports.RunImport(ctx, g.ID, tUser.ID, row.ID)

	got, err := tRepos.Exports.Get(ctx, g.ID, row.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Contains(t, got.Error, "interrupted")
	exists, err := bucket.Exists(ctx, fullPath)
	require.NoError(t, err)
	assert.True(t, exists, "staged archive must be retained for recovery")
}

// copyBlobUnderTest reuses the export service's bucket plumbing to copy a
// blob from one key to another in the same backing store. Used to "stage"
// the just-produced export under the destination group's import prefix.
func copyBlobUnderTest(ctx context.Context, svc *ExportService, srcKey, dstKey string) error {
	att := svc.repos.Attachments
	bk, err := blob.OpenBucket(ctx, att.GetConnString())
	if err != nil {
		return err
	}
	defer func() { _ = bk.Close() }()

	r, err := bk.NewReader(ctx, att.GetFullPath(srcKey), nil)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	w, err := bk.NewWriter(ctx, att.GetFullPath(dstKey), nil)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}
