package repo

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gocloud.dev/blob"

	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/attachment"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entitytemplate"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/group"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

func uploadLifecycleBlob(t *testing.T, gid uuid.UUID, content []byte) UploadResult {
	t.Helper()

	result, err := tRepos.Attachments.UploadFileByGroupID(context.Background(), gid, ItemCreateAttachment{
		Title:   "lifecycle-" + uuid.NewString(),
		Content: bytes.NewReader(content),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		tRepos.Attachments.cleanupUnreferencedBlobs(context.Background(), []attachmentBlobCandidate{{
			Path:     result.Path,
			MimeType: result.ContentType,
		}})
	})
	return result
}

func lifecycleBlobExists(t *testing.T, repo *AttachmentRepo, path string) bool {
	t.Helper()

	bucket, err := blob.OpenBucket(context.Background(), repo.GetConnString())
	require.NoError(t, err)
	defer func() { require.NoError(t, bucket.Close()) }()
	exists, err := bucket.Exists(context.Background(), repo.GetFullPath(path))
	require.NoError(t, err)
	return exists
}

func installLifecycleTrigger(t *testing.T, name, statement string) {
	t.Helper()

	_, err := tClient.Sql().ExecContext(context.Background(), "DROP TRIGGER IF EXISTS "+name)
	require.NoError(t, err)
	_, err = tClient.Sql().ExecContext(context.Background(), statement)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = tClient.Sql().ExecContext(context.Background(), "DROP TRIGGER IF EXISTS "+name)
	})
}

func TestTemplatePhotoReplacementCleansOldBlob(t *testing.T) {
	ctx := context.Background()
	template, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, templateFactory())
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.EntityTemplates.Delete(ctx, tGroup.ID, template.ID) })

	oldPhoto := uploadLifecycleBlob(t, tGroup.ID, []byte("old-template-photo-"+uuid.NewString()))
	newPhoto := uploadLifecycleBlob(t, tGroup.ID, []byte("new-template-photo-"+uuid.NewString()))
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, tGroup.ID, template.ID, oldPhoto.Path, oldPhoto.ContentType))
	require.True(t, lifecycleBlobExists(t, tRepos.Attachments, oldPhoto.Path))

	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, tGroup.ID, template.ID, newPhoto.Path, newPhoto.ContentType))
	assert.False(t, lifecycleBlobExists(t, tRepos.Attachments, oldPhoto.Path))
	assert.True(t, lifecycleBlobExists(t, tRepos.Attachments, newPhoto.Path))
}

func TestTemplatePhotoCleanupPreservesSharedPathUntilFinalReference(t *testing.T) {
	ctx := context.Background()
	first, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, templateFactory())
	require.NoError(t, err)
	second, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, templateFactory())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.EntityTemplates.Delete(ctx, tGroup.ID, first.ID)
		_ = tRepos.EntityTemplates.Delete(ctx, tGroup.ID, second.ID)
	})

	shared := uploadLifecycleBlob(t, tGroup.ID, []byte("shared-template-photo-"+uuid.NewString()))
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, tGroup.ID, first.ID, shared.Path, shared.ContentType))
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, tGroup.ID, second.ID, shared.Path, shared.ContentType))

	require.NoError(t, tRepos.EntityTemplates.ClearPhoto(ctx, tGroup.ID, first.ID))
	assert.True(t, lifecycleBlobExists(t, tRepos.Attachments, shared.Path), "the second template still references the blob")

	require.NoError(t, tRepos.EntityTemplates.Delete(ctx, tGroup.ID, second.ID))
	assert.False(t, lifecycleBlobExists(t, tRepos.Attachments, shared.Path), "the final reference was deleted")
}

func TestTemplatePhotoClearRollbackPreservesReferenceAndBlob(t *testing.T) {
	ctx := context.Background()
	data := templateFactory()
	data.Name = "clear-photo-rollback-" + uuid.NewString()
	template, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, data)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.EntityTemplates.Delete(ctx, tGroup.ID, template.ID) })
	photo := uploadLifecycleBlob(t, tGroup.ID, []byte("clear-rollback-photo-"+uuid.NewString()))
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, tGroup.ID, template.ID, photo.Path, photo.ContentType))

	triggerName := "fail_template_photo_clear"
	installLifecycleTrigger(t, triggerName, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE UPDATE OF photo_path ON entity_templates
		WHEN OLD.name = '%s' AND NEW.photo_path IS NULL
		BEGIN
			SELECT RAISE(ABORT, 'forced template photo clear failure');
		END`, triggerName, data.Name))

	err = tRepos.EntityTemplates.ClearPhoto(ctx, tGroup.ID, template.ID)
	require.ErrorContains(t, err, "forced template photo clear failure")
	got, err := tRepos.EntityTemplates.GetOne(ctx, tGroup.ID, template.ID)
	require.NoError(t, err)
	assert.Equal(t, photo.Path, got.PhotoPath)
	assert.True(t, lifecycleBlobExists(t, tRepos.Attachments, photo.Path))

	_, err = tClient.Sql().ExecContext(ctx, "DROP TRIGGER IF EXISTS "+triggerName)
	require.NoError(t, err)
	require.NoError(t, tRepos.EntityTemplates.ClearPhoto(ctx, tGroup.ID, template.ID))
	assert.False(t, lifecycleBlobExists(t, tRepos.Attachments, photo.Path))
}

func TestTemplateDeleteRollbackPreservesReferenceAndBlob(t *testing.T) {
	ctx := context.Background()
	data := templateFactory()
	data.Name = "delete-photo-rollback-" + uuid.NewString()
	template, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, data)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.EntityTemplates.Delete(ctx, tGroup.ID, template.ID) })
	photo := uploadLifecycleBlob(t, tGroup.ID, []byte("delete-rollback-photo-"+uuid.NewString()))
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, tGroup.ID, template.ID, photo.Path, photo.ContentType))

	triggerName := "fail_template_photo_delete"
	installLifecycleTrigger(t, triggerName, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE DELETE ON entity_templates
		WHEN OLD.name = '%s'
		BEGIN
			SELECT RAISE(ABORT, 'forced template delete failure');
		END`, triggerName, data.Name))

	err = tRepos.EntityTemplates.Delete(ctx, tGroup.ID, template.ID)
	require.ErrorContains(t, err, "forced template delete failure")
	got, err := tRepos.EntityTemplates.GetOne(ctx, tGroup.ID, template.ID)
	require.NoError(t, err)
	assert.Equal(t, photo.Path, got.PhotoPath)
	assert.True(t, lifecycleBlobExists(t, tRepos.Attachments, photo.Path))

	_, err = tClient.Sql().ExecContext(ctx, "DROP TRIGGER IF EXISTS "+triggerName)
	require.NoError(t, err)
	require.NoError(t, tRepos.EntityTemplates.Delete(ctx, tGroup.ID, template.ID))
	assert.False(t, lifecycleBlobExists(t, tRepos.Attachments, photo.Path))
}

func TestTemplatePhotoSetFailureCleansUnreferencedUpload(t *testing.T) {
	ctx := context.Background()
	photo := uploadLifecycleBlob(t, tGroup.ID, []byte("failed-template-photo-"+uuid.NewString()))
	require.True(t, lifecycleBlobExists(t, tRepos.Attachments, photo.Path))

	err := tRepos.EntityTemplates.SetPhoto(ctx, tGroup.ID, uuid.New(), photo.Path, photo.ContentType)
	require.Error(t, err)
	assert.False(t, lifecycleBlobExists(t, tRepos.Attachments, photo.Path))
}

func TestGroupDeleteCleansTemplatePhotoBlob(t *testing.T) {
	ctx := context.Background()
	g, err := tRepos.Groups.GroupCreate(ctx, "template-photo-delete-"+uuid.NewString(), uuid.Nil)
	require.NoError(t, err)
	template, err := tRepos.EntityTemplates.Create(ctx, g.ID, templateFactory())
	require.NoError(t, err)
	photo := uploadLifecycleBlob(t, g.ID, []byte("group-template-photo-"+uuid.NewString()))
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, g.ID, template.ID, photo.Path, photo.ContentType))

	require.NoError(t, tRepos.Groups.GroupDelete(ctx, g.ID))
	assert.False(t, lifecycleBlobExists(t, tRepos.Attachments, photo.Path))
}

func TestGroupDeleteRollbackPreservesTemplatePhotoBlob(t *testing.T) {
	ctx := context.Background()
	groupName := "template-photo-rollback-" + uuid.NewString()
	g, err := tRepos.Groups.GroupCreate(ctx, groupName, uuid.Nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Groups.GroupDelete(ctx, g.ID) })
	template, err := tRepos.EntityTemplates.Create(ctx, g.ID, templateFactory())
	require.NoError(t, err)
	photo := uploadLifecycleBlob(t, g.ID, []byte("group-rollback-photo-"+uuid.NewString()))
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, g.ID, template.ID, photo.Path, photo.ContentType))

	triggerName := "fail_group_photo_delete"
	installLifecycleTrigger(t, triggerName, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE DELETE ON groups
		WHEN OLD.name = '%s'
		BEGIN
			SELECT RAISE(ABORT, 'forced group delete failure');
		END`, triggerName, groupName))

	err = tRepos.Groups.GroupDelete(ctx, g.ID)
	require.ErrorContains(t, err, "forced group delete failure")
	exists, err := tClient.Group.Query().Where(group.ID(g.ID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, exists)
	templateExists, err := tClient.EntityTemplate.Query().Where(entitytemplate.ID(template.ID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, templateExists)
	assert.True(t, lifecycleBlobExists(t, tRepos.Attachments, photo.Path))

	_, err = tClient.Sql().ExecContext(ctx, "DROP TRIGGER IF EXISTS "+triggerName)
	require.NoError(t, err)
	require.NoError(t, tRepos.Groups.GroupDelete(ctx, g.ID))
	assert.False(t, lifecycleBlobExists(t, tRepos.Attachments, photo.Path))
}

func TestAttachmentCreateSaveFailureCleansUploadedBlob(t *testing.T) {
	ctx := context.Background()
	item := useEntities(t, 1)[0]
	content := []byte("failed-attachment-save-" + uuid.NewString())
	preUpload := uploadLifecycleBlob(t, tGroup.ID, content)
	require.True(t, lifecycleBlobExists(t, tRepos.Attachments, preUpload.Path))

	triggerName := "fail_attachment_blob_insert"
	installLifecycleTrigger(t, triggerName, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE INSERT ON attachments
		WHEN NEW.title = 'forced-save-failure'
		BEGIN
			SELECT RAISE(ABORT, 'forced attachment save failure');
		END`, triggerName))

	_, err := tRepos.Attachments.Create(ctx, item.ID, ItemCreateAttachment{
		Title:   "forced-save-failure",
		Content: bytes.NewReader(content),
	}, attachment.TypeManual, false)
	require.ErrorContains(t, err, "forced attachment save failure")
	assert.False(t, lifecycleBlobExists(t, tRepos.Attachments, preUpload.Path))
}

func tinyLifecyclePNG(t *testing.T) ([]byte, image.Image) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(20 + x), G: uint8(40 + y), B: 100, A: 255})
		}
	}
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, img))
	return out.Bytes(), img
}

func TestThumbnailSaveFailureCleansUploadedBlob(t *testing.T) {
	ctx := context.Background()
	item := useEntities(t, 1)[0]
	pngBytes, img := tinyLifecyclePNG(t)
	source, err := tRepos.Attachments.Create(ctx, item.ID, ItemCreateAttachment{
		Title:   "thumbnail-source.png",
		Content: bytes.NewReader(pngBytes),
	}, attachment.TypePhoto, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Attachments.Delete(ctx, tGroup.ID, source.ID) })

	repos := New(
		tClient,
		tbus,
		tRepos.Attachments.storage,
		"mem://{{ .Topic }}",
		config.Thumbnail{Enabled: true, Width: 128, Height: 128},
	)
	expected, err := repos.Attachments.processThumbnailFromImage(ctx, tGroup.ID, img, source.Title, 1)
	require.NoError(t, err)
	require.True(t, lifecycleBlobExists(t, repos.Attachments, expected.Path))
	t.Cleanup(func() {
		repos.Attachments.cleanupUnreferencedBlobs(ctx, []attachmentBlobCandidate{{
			Path:     expected.Path,
			MimeType: expected.ContentType,
		}})
	})

	triggerName := "fail_thumbnail_blob_insert"
	installLifecycleTrigger(t, triggerName, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE INSERT ON attachments
		WHEN NEW.type = 'thumbnail'
		BEGIN
			SELECT RAISE(ABORT, 'forced thumbnail save failure');
		END`, triggerName))

	err = repos.Attachments.CreateThumbnail(ctx, tGroup.ID, source.ID, source.Title)
	require.ErrorContains(t, err, "forced thumbnail save failure")
	assert.False(t, lifecycleBlobExists(t, repos.Attachments, expected.Path))
	assert.True(t, lifecycleBlobExists(t, repos.Attachments, source.Path), "source blob remains referenced")
}

func TestThumbnailCreationIsIdempotentAfterSuccess(t *testing.T) {
	ctx := context.Background()
	item := useEntities(t, 1)[0]
	pngBytes, _ := tinyLifecyclePNG(t)
	source, err := tRepos.Attachments.Create(ctx, item.ID, ItemCreateAttachment{
		Title:   "idempotent-thumbnail-source.png",
		Content: bytes.NewReader(pngBytes),
	}, attachment.TypePhoto, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.Attachments.Delete(ctx, tGroup.ID, source.ID) })

	repos := New(
		tClient,
		tbus,
		tRepos.Attachments.storage,
		"mem://{{ .Topic }}",
		config.Thumbnail{Enabled: true, Width: 128, Height: 128},
	)
	require.NoError(t, repos.Attachments.CreateThumbnail(ctx, tGroup.ID, source.ID, source.Title))
	withThumbnail, err := tClient.Attachment.Query().
		Where(attachment.ID(source.ID)).
		WithThumbnail().
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, withThumbnail.Edges.Thumbnail)
	firstThumbnailID := withThumbnail.Edges.Thumbnail.ID
	countBefore, err := tClient.Attachment.Query().
		Where(attachment.TypeEQ(attachment.TypeThumbnail)).
		Count(ctx)
	require.NoError(t, err)

	require.NoError(t, repos.Attachments.CreateThumbnail(ctx, tGroup.ID, source.ID, source.Title))
	withThumbnail, err = tClient.Attachment.Query().
		Where(attachment.ID(source.ID)).
		WithThumbnail().
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, withThumbnail.Edges.Thumbnail)
	assert.Equal(t, firstThumbnailID, withThumbnail.Edges.Thumbnail.ID)

	countAfter, err := tClient.Attachment.Query().
		Where(attachment.TypeEQ(attachment.TypeThumbnail)).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, countBefore, countAfter)
}

func TestAttachmentFailureCleanupPreservesSharedTemplateBlob(t *testing.T) {
	ctx := context.Background()
	item := useEntities(t, 1)[0]
	content := []byte("shared-failed-attachment-" + uuid.NewString())
	shared := uploadLifecycleBlob(t, tGroup.ID, content)
	template, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, templateFactory())
	require.NoError(t, err)
	t.Cleanup(func() { _ = tRepos.EntityTemplates.Delete(ctx, tGroup.ID, template.ID) })
	require.NoError(t, tRepos.EntityTemplates.SetPhoto(ctx, tGroup.ID, template.ID, shared.Path, shared.ContentType))

	triggerName := "fail_shared_attachment_insert"
	installLifecycleTrigger(t, triggerName, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE INSERT ON attachments
		WHEN NEW.title = 'forced-shared-save-failure'
		BEGIN
			SELECT RAISE(ABORT, 'forced shared attachment save failure');
		END`, triggerName))

	_, err = tRepos.Attachments.Create(ctx, item.ID, ItemCreateAttachment{
		Title:   "forced-shared-save-failure",
		Content: strings.NewReader(string(content)),
	}, attachment.TypeManual, false)
	require.ErrorContains(t, err, "forced shared attachment save failure")
	assert.True(t, lifecycleBlobExists(t, tRepos.Attachments, shared.Path), "template still references the content-addressed blob")
}
