package repo

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/evanoberholster/imagemeta"
	"github.com/gen2brain/avif"
	"github.com/gen2brain/heic"
	"github.com/gen2brain/jpegxl"
	"github.com/gen2brain/webp"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/attachment"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entity"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entitytemplate"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/group"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/pkgs/utils"
	"github.com/zeebo/blake3"
	"go.opentelemetry.io/otel"
	"golang.org/x/image/draw"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/memblob"
	_ "gocloud.dev/blob/s3blob"

	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/awssnssqs"
	_ "gocloud.dev/pubsub/azuresb"
	_ "gocloud.dev/pubsub/gcppubsub"
	_ "gocloud.dev/pubsub/kafkapubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	_ "gocloud.dev/pubsub/natspubsub"
	_ "gocloud.dev/pubsub/rabbitpubsub"
)

// AttachmentRepo is a repository for Attachments table that links Items to their
// associated files while also specifying the type of the attachment.
type AttachmentRepo struct {
	db         *ent.Client
	storage    config.Storage
	pubSubConn string
	thumbnail  config.Thumbnail
}

type (
	ItemAttachment struct {
		ID        uuid.UUID       `json:"id"`
		CreatedAt time.Time       `json:"createdAt"`
		UpdatedAt time.Time       `json:"updatedAt"`
		Type      string          `json:"type"`
		Primary   bool            `json:"primary"`
		Path      string          `json:"path"`
		Title     string          `json:"title"`
		MimeType  string          `json:"mimeType,omitempty"`
		Thumbnail *ent.Attachment `json:"thumbnail,omitempty"`
	}

	ItemAttachmentUpdate struct {
		ID      uuid.UUID `json:"-"`
		Type    string    `json:"type"`
		Title   string    `json:"title"`
		Primary bool      `json:"primary"`
	}

	ItemCreateAttachment struct {
		Title   string    `json:"title"`
		Content io.Reader `json:"content"`
	}

	attachmentBlobCandidate struct {
		Path     string
		MimeType string
	}
)

const MimeTypeLinkURL = "link/url"

// Thumbnail source limits are enforced before full decode for every supported
// raster format. This bounds worst-case decoder allocations from tiny,
// maliciously crafted files with enormous dimensions.
const (
	maxThumbnailSourceDimension = 32_768
	maxThumbnailSourcePixels    = 50_000_000
)

var externalLinkMimeTypes = []string{
	MimeTypeLinkURL,
}

func MimeTypeForSourceType(sourceType string) (string, bool) {
	switch sourceType {
	case "link":
		return MimeTypeLinkURL, true
	default:
		return "", false
	}
}

func isExternalLink(mimeType string) bool {
	for _, m := range externalLinkMimeTypes {
		if m == mimeType {
			return true
		}
	}
	return false
}

func ToItemAttachment(attachment *ent.Attachment) ItemAttachment {
	return ItemAttachment{
		ID:        attachment.ID,
		CreatedAt: attachment.CreatedAt,
		UpdatedAt: attachment.UpdatedAt,
		Type:      attachment.Type.String(),
		Primary:   attachment.Primary,
		Path:      attachment.Path,
		Title:     attachment.Title,
		MimeType:  attachment.MimeType,
		Thumbnail: attachment.Edges.Thumbnail,
	}
}

// normalizePath converts backslashes to forward slashes and trims slashes from both ends
// This ensures consistent path separators for blob storage which expects forward slashes
func normalizePath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	return strings.Trim(path, "/")
}

// legacyFlatPathToken is the gocloud fileblob escape sequence for a backslash
// (0x5c). On Windows, pre-v0.22.1 attachment keys built with filepath.Join used
// "\" as the path separator; fileblob escaped that to this token, so files were
// stored as flat names at the bucket root. v0.22.1+ uses "/" so files now live
// in real subdirectories.
const legacyFlatPathToken = "__0x5c__"

// bucketLocalDir returns the absolute on-disk directory backing the fileblob
// bucket, or "" if the storage backend is not a file:// URL.
func (r *AttachmentRepo) bucketLocalDir() (string, error) {
	cs := r.storage.ConnString
	if !strings.HasPrefix(cs, "file://") {
		return "", nil
	}
	// Homebox-specific shortcut for "relative to cwd".
	if strings.HasPrefix(cs, "file:///./") {
		return filepath.Abs(strings.TrimPrefix(cs, "file:///./"))
	}
	raw := strings.TrimPrefix(cs, "file://")
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	// On Windows a file URL looks like file:///C:/foo, so strip the empty-host
	// leading slash before treating the rest as a native path.
	if runtime.GOOS == "windows" && len(raw) >= 3 && raw[0] == '/' && raw[2] == ':' {
		raw = raw[1:]
	}
	return filepath.Abs(raw)
}

// MigrateLegacyFlatPaths renames attachment files written by older homebox
// versions on Windows from a flat-escaped layout into a real subdirectory
// layout. It is a no-op when the storage backend is not file:// or when no
// matching files are found, so it is safe to run on every startup. Callers
// should gate invocation on runtime.GOOS == "windows" since no other platform
// can produce these files.
func (r *AttachmentRepo) MigrateLegacyFlatPaths() error {
	root, err := r.bucketLocalDir()
	if err != nil {
		return err
	}
	if root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read bucket root %s: %w", root, err)
	}

	moved, skipped := 0, 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.Contains(name, legacyFlatPathToken) {
			continue
		}

		decoded := strings.ReplaceAll(name, legacyFlatPathToken, string(filepath.Separator))
		oldPath := filepath.Join(root, name)
		newPath := filepath.Join(root, decoded)

		if _, statErr := os.Stat(newPath); statErr == nil {
			log.Warn().Str("source", oldPath).Str("target", newPath).
				Msg("legacy attachment migration: target already exists, leaving source in place")
			skipped++
			continue
		} else if !os.IsNotExist(statErr) {
			log.Err(statErr).Str("target", newPath).Msg("legacy attachment migration: failed to stat target")
			skipped++
			continue
		}

		if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
			log.Err(err).Str("dir", filepath.Dir(newPath)).Msg("legacy attachment migration: failed to create parent directory")
			skipped++
			continue
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			log.Err(err).Str("source", oldPath).Str("target", newPath).Msg("legacy attachment migration: failed to rename file")
			skipped++
			continue
		}
		moved++
	}

	if moved > 0 || skipped > 0 {
		log.Info().Int("moved", moved).Int("skipped", skipped).
			Msg("migrated legacy flat-encoded attachment files")
	}
	return nil
}

func (r *AttachmentRepo) path(gid uuid.UUID, hash string) string {
	// Always use forward slashes for consistency across platforms
	// This ensures paths are stored in the database with forward slashes
	return fmt.Sprintf("%s/documents/%s", gid.String(), hash)
}

func (r *AttachmentRepo) fullPath(relativePath string) string {
	// Normalize path separators to forward slashes for blob storage
	// The blob library expects forward slashes in keys regardless of OS
	normalizedRelativePath := normalizePath(relativePath)

	// Always use forward slashes when joining paths for blob storage
	if r.storage.PrefixPath == "" {
		return normalizedRelativePath
	}
	normalizedPrefix := normalizePath(r.storage.PrefixPath)

	if normalizedPrefix == "" {
		return normalizedRelativePath
	}

	return fmt.Sprintf("%s/%s", normalizedPrefix, normalizedRelativePath)
}

func (r *AttachmentRepo) GetFullPath(relativePath string) string {
	return r.fullPath(relativePath)
}

func (r *AttachmentRepo) GetConnString() string {
	// Handle the default case for file storage
	// which is file:///./ meaning relative to the current working directory
	if strings.HasPrefix(r.storage.ConnString, "file:///./") {
		dir, err := filepath.Abs(strings.TrimPrefix(r.storage.ConnString, "file:///./"))
		if runtime.GOOS == "windows" {
			dir = fmt.Sprintf("/%s", dir)
		}
		if err != nil {
			log.Err(err).Msg("failed to get absolute path for attachment directory")
			return r.storage.ConnString
		}
		return strings.ReplaceAll(fmt.Sprintf("file://%s?no_tmp_dir=true", dir), "\\", "/")
	} else if strings.HasPrefix(r.storage.ConnString, "file://") {
		// Handle the case for file storage with an absolute path
		// Convert Windows paths to a format compatible with fileblob
		// e.g. file:///C:/path/to/file becomes file:///C/path
		dir := strings.TrimPrefix(strings.ReplaceAll(r.storage.ConnString, "\\", "/"), "file://")
		if runtime.GOOS == "windows" {
			// Remove the colon from the drive letter (in case the user adds it)
			dir = strings.ReplaceAll(dir, ":", "")
			// Ensure the path starts with a slash for Windows compatibility
			dir = fmt.Sprintf("/%s", dir)
		}
		return fmt.Sprintf("file://%s", dir)
	}
	return r.storage.ConnString
}

func attachmentBlobCandidates(attachments []*ent.Attachment) []attachmentBlobCandidate {
	candidates := make([]attachmentBlobCandidate, 0, len(attachments)*2)
	for _, att := range attachments {
		if att.Path != "" && !isExternalLink(att.MimeType) {
			candidates = append(candidates, attachmentBlobCandidate{
				Path:     att.Path,
				MimeType: att.MimeType,
			})
		}
		if thumbnail := att.Edges.Thumbnail; thumbnail != nil && thumbnail.Path != "" {
			candidates = append(candidates, attachmentBlobCandidate{
				Path:     thumbnail.Path,
				MimeType: thumbnail.MimeType,
			})
		}
	}
	return candidates
}

// deleteAttachmentThumbnailsTx removes thumbnail rows inside the caller's
// transaction. The source attachment FK uses ON DELETE SET NULL, so deleting
// the thumbnail safely clears the edge before the source/entity is removed.
func deleteAttachmentThumbnailsTx(
	ctx context.Context,
	tx *ent.Tx,
	attachments []*ent.Attachment,
) ([]attachmentBlobCandidate, error) {
	candidates := attachmentBlobCandidates(attachments)
	for _, att := range attachments {
		if att.Edges.Thumbnail == nil {
			continue
		}
		if err := tx.Attachment.DeleteOneID(att.Edges.Thumbnail.ID).Exec(ctx); err != nil {
			return nil, err
		}
	}
	return candidates, nil
}

// cleanupUnreferencedBlobs is intentionally post-commit: database state is the
// source of truth, and a storage failure must never roll back references after
// their files have already been removed. Cleanup is best-effort; unremoved
// blobs are harmless orphans and can be retried later.
func (r *AttachmentRepo) cleanupUnreferencedBlobs(
	ctx context.Context,
	candidates []attachmentBlobCandidate,
) {
	paths := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.Path == "" || isExternalLink(candidate.MimeType) {
			continue
		}
		paths[candidate.Path] = struct{}{}
	}
	if len(paths) == 0 {
		return
	}

	var bucket *blob.Bucket
	for path := range paths {
		attachmentRef, err := r.db.Attachment.Query().
			Where(attachment.Path(path)).
			Exist(ctx)
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("failed to check attachment blob references")
			continue
		}
		templateRef, err := r.db.EntityTemplate.Query().
			Where(entitytemplate.PhotoPath(path)).
			Exist(ctx)
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("failed to check template blob references")
			continue
		}
		if attachmentRef || templateRef {
			continue
		}

		if bucket == nil {
			bucket, err = blob.OpenBucket(ctx, r.GetConnString())
			if err != nil {
				log.Warn().Err(err).Msg("database deletion committed but blob cleanup bucket could not be opened")
				return
			}
		}
		if err := bucket.Delete(ctx, r.fullPath(path)); err != nil {
			log.Warn().Err(err).Str("path", path).Msg("database deletion committed but blob cleanup failed")
		}
	}
	if bucket != nil {
		if err := bucket.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close blob cleanup bucket")
		}
	}
}

func (r *AttachmentRepo) Create(ctx context.Context, itemID uuid.UUID, doc ItemCreateAttachment, typ attachment.Type, primary bool) (*ent.Attachment, error) {
	ctx, span := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.Create")
	defer span.End()

	tx, err := r.db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	bldrId := uuid.New()

	bldr := tx.Attachment.Create().
		SetID(bldrId).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SetType(typ).
		SetEntityID(itemID).
		SetTitle(doc.Title)

	if typ == attachment.TypePhoto && primary {
		bldr = bldr.SetPrimary(true)
		err := tx.Attachment.Update().
			Where(
				attachment.HasEntityWith(entity.ID(itemID)),
				attachment.IDNEQ(bldrId),
			).
			SetPrimary(false).
			Exec(ctx)
		if err != nil {
			log.Err(err).Msg("failed to remove primary from other attachments")
			return nil, err
		}
	} else if typ == attachment.TypePhoto {
		// Autoset primary to true if this is the first attachment
		// that is of type photo
		cnt, err := tx.Attachment.Query().
			Where(
				attachment.HasEntityWith(entity.ID(itemID)),
				attachment.TypeEQ(typ),
			).
			Count(ctx)
		if err != nil {
			log.Err(err).Msg("failed to count attachments")
			return nil, err
		}

		if cnt == 0 {
			bldr = bldr.SetPrimary(true)
		}
	}

	// Get the group ID for the item the attachment is being created for
	itemGroup, err := tx.Entity.Query().QueryGroup().Where(group.HasEntitiesWith(entity.ID(itemID))).First(ctx)
	if err != nil {
		log.Err(err).Msg("failed to get item group")
		return nil, err
	}

	// Upload the file to the storage bucket
	uploadResult, err := r.UploadFile(ctx, itemGroup, doc)
	if err != nil {
		return nil, err
	}

	bldr = bldr.SetMimeType(uploadResult.ContentType)
	bldr = bldr.SetPath(uploadResult.Path)

	attachmentDb, err := bldr.Save(ctx)
	if err != nil {
		log.Err(err).Msg("failed to save attachment to database")
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		log.Err(err).Msg("failed to commit transaction")
		return nil, err
	}
	committed = true

	if r.thumbnail.Enabled {
		pubsubString, err := utils.GenerateSubPubConn(r.pubSubConn, "thumbnails")
		if err != nil {
			log.Warn().Err(err).Msg("attachment saved but thumbnail notification configuration is invalid")
		} else {
			topic, openErr := pubsub.OpenTopic(ctx, pubsubString)
			if openErr != nil {
				log.Warn().Err(openErr).Msg("attachment saved but thumbnail topic could not be opened")
			} else {
				defer func() {
					if shutdownErr := topic.Shutdown(ctx); shutdownErr != nil {
						log.Warn().Err(shutdownErr).Msg("failed to shut down thumbnail topic")
					}
				}()
				if sendErr := topic.Send(ctx, &pubsub.Message{
					Body: []byte(fmt.Sprintf("attachment_created:%s", attachmentDb.ID.String())),
					Metadata: map[string]string{
						"group_id":      itemGroup.ID.String(),
						"attachment_id": attachmentDb.ID.String(),
						"title":         doc.Title,
						"path":          attachmentDb.Path,
					},
				}); sendErr != nil {
					log.Warn().Err(sendErr).Msg("attachment saved but thumbnail notification failed")
				}
			}
		}
	}

	return r.Get(ctx, itemGroup.ID, attachmentDb.ID)
}

func (r *AttachmentRepo) CreateExternalLink(ctx context.Context, entityID uuid.UUID, externalID string, title string, mimeType string, attType attachment.Type) (*ent.Attachment, error) {
	ctx, span := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.CreateExternalLink")
	defer span.End()

	if attType == "" {
		attType = attachment.TypeAttachment
	}

	att, err := r.db.Attachment.Create().
		SetID(uuid.New()).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SetType(attType).
		SetEntityID(entityID).
		SetTitle(title).
		SetPath(externalID).
		SetMimeType(mimeType).
		SetPrimary(false).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	return att, nil
}

func (r *AttachmentRepo) Get(ctx context.Context, gid uuid.UUID, id uuid.UUID) (*ent.Attachment, error) {
	first, err := r.db.Attachment.Query().Where(attachment.ID(id)).Only(ctx)
	if err != nil {
		return nil, err
	}
	if first.Type == attachment.TypeThumbnail {
		// If the attachment is a thumbnail, get the parent attachment and check if it belongs to the specified group
		return r.db.Attachment.
			Query().
			Where(attachment.ID(id),
				attachment.HasThumbnailWith(attachment.HasEntityWith(entity.HasGroupWith(group.ID(gid)))),
			).
			WithEntity().
			WithThumbnail().
			Only(ctx)
	} else {
		// For regular attachments, check if the attachment's item belongs to the specified group
		return r.db.Attachment.
			Query().
			Where(attachment.ID(id),
				attachment.HasEntityWith(entity.HasGroupWith(group.ID(gid))),
			).
			WithEntity().
			WithThumbnail().
			Only(ctx)
	}
}

// GetForEntity additionally verifies that id belongs to the entity identified
// by the route. Thumbnail IDs are accepted when their source attachment
// belongs to entityID.
func (r *AttachmentRepo) GetForEntity(
	ctx context.Context,
	gid, entityID, id uuid.UUID,
) (*ent.Attachment, error) {
	return r.db.Attachment.Query().
		Where(
			attachment.ID(id),
			attachment.Or(
				attachment.HasEntityWith(
					entity.ID(entityID),
					entity.HasGroupWith(group.ID(gid)),
				),
				attachment.HasThumbnailWith(attachment.HasEntityWith(
					entity.ID(entityID),
					entity.HasGroupWith(group.ID(gid)),
				)),
			),
		).
		WithEntity().
		WithThumbnail().
		Only(ctx)
}

func (r *AttachmentRepo) Update(
	ctx context.Context,
	gid, entityID, id uuid.UUID,
	data *ItemAttachmentUpdate,
) (*ent.Attachment, error) {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	current, err := tx.Attachment.Query().
		Where(
			attachment.ID(id),
			attachment.HasEntityWith(
				entity.ID(entityID),
				entity.HasGroupWith(group.ID(gid)),
			),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}

	typ := attachment.Type(data.Type)
	bldr := tx.Attachment.UpdateOne(current).
		SetType(typ).
		SetTitle(data.Title)

	// Primary only applies to photos
	if typ == attachment.TypePhoto {
		bldr = bldr.SetPrimary(data.Primary)
	} else {
		bldr = bldr.SetPrimary(false)
	}

	// Only remove primary status from other photo attachments when setting a new photo as primary
	if typ == attachment.TypePhoto && data.Primary {
		err = tx.Attachment.Update().
			Where(
				attachment.HasEntityWith(
					entity.ID(entityID),
					entity.HasGroupWith(group.ID(gid)),
				),
				attachment.IDNEQ(id),
				attachment.TypeEQ(attachment.TypePhoto),
			).
			SetPrimary(false).
			Exec(ctx)
		if err != nil {
			return nil, err
		}
	}

	if _, err := bldr.Save(ctx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	return r.GetForEntity(ctx, gid, entityID, id)
}

func (r *AttachmentRepo) Delete(ctx context.Context, gid uuid.UUID, id uuid.UUID) error {
	ctx, span := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.Delete")
	defer span.End()

	tx, err := r.db.Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	doc, err := tx.Attachment.Query().
		Where(
			attachment.ID(id),
			attachment.HasEntityWith(entity.HasGroupWith(group.ID(gid))),
		).
		WithThumbnail().
		Only(ctx)
	if err != nil {
		return err
	}

	candidates, err := deleteAttachmentThumbnailsTx(ctx, tx, []*ent.Attachment{doc})
	if err != nil {
		return err
	}
	if err := tx.Attachment.DeleteOneID(id).Exec(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	r.cleanupUnreferencedBlobs(ctx, candidates)
	return nil
}

// DeleteForEntity rejects a route attachment ID that belongs to another
// entity, even when both entities are in the same group.
func (r *AttachmentRepo) DeleteForEntity(ctx context.Context, gid, entityID, id uuid.UUID) error {
	exists, err := r.db.Attachment.Query().
		Where(
			attachment.ID(id),
			attachment.HasEntityWith(
				entity.ID(entityID),
				entity.HasGroupWith(group.ID(gid)),
			),
		).
		Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return &ent.NotFoundError{}
	}
	return r.Delete(ctx, gid, id)
}

func (r *AttachmentRepo) Rename(ctx context.Context, gid uuid.UUID, id uuid.UUID, title string) (*ent.Attachment, error) {
	// Validate that the attachment belongs to the specified group
	_, err := r.db.Attachment.Query().
		Where(
			attachment.ID(id),
			attachment.HasEntityWith(entity.HasGroupWith(group.ID(gid))),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}

	return r.db.Attachment.UpdateOneID(id).SetTitle(title).Save(ctx)
}

//nolint:gocyclo
func (r *AttachmentRepo) CreateThumbnail(ctx context.Context, groupId, attachmentId uuid.UUID, title string, path string) error {
	ctx, span := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.CreateThumbnail")
	defer span.End()

	log.Debug().Msg("starting thumbnail creation")
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	log.Debug().Msg("set initial database transaction")
	att := tx.Attachment.Create().
		SetID(uuid.New()).
		SetTitle(fmt.Sprintf("%s-thumb", title)).
		SetType("thumbnail")

	log.Debug().Msg("opening original file")
	bucket, err := blob.OpenBucket(ctx, r.GetConnString())
	if err != nil {
		log.Err(err).Msg("failed to open bucket")
		err := tx.Rollback()
		if err != nil {
			return err
		}
		return err
	}
	defer func(bucket *blob.Bucket) {
		err := bucket.Close()
		if err != nil {
			err := tx.Rollback()
			if err != nil {
				return
			}
			log.Err(err).Msg("failed to close bucket")
		}
	}(bucket)

	origFile, err := bucket.Open(r.fullPath(path))
	if err != nil {
		err := tx.Rollback()
		if err != nil {
			return err
		}
		return err
	}
	defer func(file fs.File) {
		err := file.Close()
		if err != nil {
			err := tx.Rollback()
			if err != nil {
				return
			}
			log.Err(err).Msg("failed to close file")
		}
	}(origFile)

	log.Debug().Msg("stat original file for file size")
	stats, err := origFile.Stat()
	if err != nil {
		err := tx.Rollback()
		if err != nil {
			return err
		}
		log.Err(err).Msg("failed to stat original file")
		return err
	}

	if stats.Size() > 100*1024*1024 {
		return fmt.Errorf("original file %s is too large to create a thumbnail", title)
	}

	log.Debug().Msg("reading original file content")
	// Use LimitReader as a safety measure to prevent reading more than 100MB
	// even if the stat size is incorrect or tampered
	limitedReader := io.LimitReader(origFile, 100*1024*1024)
	contentBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		err := tx.Rollback()
		if err != nil {
			return err
		}
		log.Err(err).Msg("failed to read original file content")
		return err
	}

	log.Debug().Msg("detecting content type of original file")
	contentType := http.DetectContentType(contentBytes[:min(512, len(contentBytes))])

	if contentType == "application/octet-stream" {
		switch {
		case strings.HasSuffix(title, ".heic") || strings.HasSuffix(title, ".heif"):
			contentType = "image/heic"
		case strings.HasSuffix(title, ".avif"):
			contentType = "image/avif"
		case strings.HasSuffix(title, ".jxl"):
			contentType = "image/jxl"
		}
	}

	if err := validateThumbnailSourceDimensions(contentType, contentBytes); err != nil {
		return err
	}

	// Pre-read orientation once for all image types that support it
	// This avoids re-decoding the metadata for each image type
	var orientation uint16 = 1 // Default orientation
	if contentType != "image/avif" && contentType != "image/jxl" {
		imageMeta, err := imagemeta.Decode(bytes.NewReader(contentBytes))
		if err == nil {
			orientation = uint16(imageMeta.Orientation)
		}
		// If error, just use default orientation (1)
	}

	switch {
	case isImageFile(contentType):
		log.Debug().Msg("creating thumbnail for image file")
		img, _, err := image.Decode(bytes.NewReader(contentBytes))
		if err != nil {
			log.Err(err).Msg("failed to decode image file")
			err := tx.Rollback()
			if err != nil {
				log.Err(err).Msg("failed to rollback transaction")
				return err
			}
			return err
		}
		uploadResult, err := r.processThumbnailFromImage(ctx, groupId, img, title, orientation)
		if err != nil {
			err := tx.Rollback()
			if err != nil {
				return err
			}
			return err
		}
		att.SetPath(uploadResult.Path)
	case contentType == "image/webp":
		log.Debug().Msg("creating thumbnail for webp file")
		img, err := webp.Decode(bytes.NewReader(contentBytes))
		if err != nil {
			log.Err(err).Msg("failed to decode webp image")
			err := tx.Rollback()
			if err != nil {
				return err
			}
			return err
		}
		uploadResult, err := r.processThumbnailFromImage(ctx, groupId, img, title, orientation)
		if err != nil {
			err := tx.Rollback()
			if err != nil {
				return err
			}
			return err
		}
		att.SetPath(uploadResult.Path)
	case contentType == "image/avif":
		log.Debug().Msg("creating thumbnail for avif file")
		img, err := avif.Decode(bytes.NewReader(contentBytes))
		if err != nil {
			log.Err(err).Msg("failed to decode avif image")
			err := tx.Rollback()
			if err != nil {
				return err
			}
			return err
		}
		uploadResult, err := r.processThumbnailFromImage(ctx, groupId, img, title, uint16(1))
		if err != nil {
			err := tx.Rollback()
			if err != nil {
				return err
			}
			return err
		}
		att.SetPath(uploadResult.Path)
	case contentType == "image/heic" || contentType == "image/heif":
		log.Debug().Msg("creating thumbnail for heic file")
		img, err := heic.Decode(bytes.NewReader(contentBytes))
		if err != nil {
			log.Err(err).Msg("failed to decode avif image")
			err := tx.Rollback()
			if err != nil {
				return err
			}
			return err
		}
		uploadResult, err := r.processThumbnailFromImage(ctx, groupId, img, title, orientation)
		if err != nil {
			err := tx.Rollback()
			if err != nil {
				return err
			}
			return err
		}
		att.SetPath(uploadResult.Path)
	case contentType == "image/jxl":
		log.Debug().Msg("creating thumbnail for jpegxl file")
		img, err := jpegxl.Decode(bytes.NewReader(contentBytes))
		if err != nil {
			log.Err(err).Msg("failed to decode avif image")
			err := tx.Rollback()
			if err != nil {
				return err
			}
			return err
		}
		uploadResult, err := r.processThumbnailFromImage(ctx, groupId, img, title, uint16(1))
		if err != nil {
			err := tx.Rollback()
			if err != nil {
				return err
			}
			return err
		}
		att.SetPath(uploadResult.Path)
	default:
		return fmt.Errorf("file type %s is not supported for thumbnail creation or document thumnails disabled", title)
	}

	att.SetMimeType("image/webp")

	log.Debug().Msg("saving thumbnail attachment to database")
	thumbnail, err := att.Save(ctx)
	if err != nil {
		return err
	}

	_, err = tx.Attachment.UpdateOneID(attachmentId).SetThumbnail(thumbnail).Save(ctx)
	if err != nil {
		return err
	}

	log.Debug().Msg("finishing thumbnail creation transaction")
	if err := tx.Commit(); err != nil {
		log.Err(err).Msg("failed to commit transaction")
		return err
	}
	committed = true
	return nil
}

func (r *AttachmentRepo) CreateMissingThumbnails(ctx context.Context, groupId uuid.UUID) (int, error) {
	ctx, span := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.CreateMissingThumbnails")
	defer span.End()

	if !r.thumbnail.Enabled {
		return 0, nil
	}

	attachments, err := r.db.Attachment.Query().
		Where(
			attachment.HasEntityWith(entity.HasGroupWith(group.ID(groupId))),
			attachment.TypeNEQ("thumbnail"),
		).
		All(ctx)
	if err != nil {
		return 0, err
	}

	pubsubString, err := utils.GenerateSubPubConn(r.pubSubConn, "thumbnails")
	if err != nil {
		return 0, fmt.Errorf("generate thumbnail pubsub connection: %w", err)
	}
	topic, err := pubsub.OpenTopic(ctx, pubsubString)
	if err != nil {
		return 0, fmt.Errorf("open thumbnail pubsub topic: %w", err)
	}
	defer func() {
		if shutdownErr := topic.Shutdown(ctx); shutdownErr != nil {
			log.Warn().Err(shutdownErr).Msg("failed to shut down thumbnail topic")
		}
	}()

	count := 0
	for _, attachment := range attachments {
		exists, err := attachment.QueryThumbnail().Exist(ctx)
		if err != nil {
			return count, err
		}
		if !exists {
			if count > 0 && count%100 == 0 {
				time.Sleep(2 * time.Second)
			}
			err = topic.Send(ctx, &pubsub.Message{
				Body: []byte(fmt.Sprintf("attachment_created:%s", attachment.ID.String())),
				Metadata: map[string]string{
					"group_id":      groupId.String(),
					"attachment_id": attachment.ID.String(),
					"title":         attachment.Title,
					"path":          attachment.Path,
				},
			})
			if err != nil {
				return count, err
			}
			count++
		}
	}

	return count, nil
}

// UploadResult contains the results of uploading a file
type UploadResult struct {
	Path        string
	ContentType string
}

func (r *AttachmentRepo) UploadFile(ctx context.Context, itemGroup *ent.Group, doc ItemCreateAttachment) (UploadResult, error) {
	ctx, span := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.UploadFile")
	defer span.End()

	// Prepare for the hashing of the file contents
	hashOut := make([]byte, 32)

	// Use a buffer to store content for blake3 key derivation and storage
	// While buffering, we compute MD5 and blake3 hashes in parallel for efficiency
	buf := new(bytes.Buffer)

	// Create hash writers
	blake3Hasher := blake3.New()
	md5Hasher := md5.New()

	// Use MultiWriter to write to buffer and both hashers simultaneously
	multiWriter := io.MultiWriter(buf, blake3Hasher, md5Hasher)

	_, err := io.Copy(multiWriter, doc.Content)
	if err != nil {
		log.Err(err).Msg("failed to read file content")
		return UploadResult{}, err
	}

	// Now the buffer contains all the data, and streaming hashes are computed
	contentBytes := buf.Bytes()

	// Derive the blake3 key using the group ID as context
	// Note: DeriveKey requires the full content buffer (not streaming)
	blake3.DeriveKey(itemGroup.ID.String(), contentBytes, hashOut)

	// Write the file to the blob storage bucket which might be a local file system or cloud storage
	bucket, err := blob.OpenBucket(ctx, r.GetConnString())
	if err != nil {
		log.Err(err).Msg("failed to open bucket")
		return UploadResult{}, err
	}
	defer func(bucket *blob.Bucket) {
		err := bucket.Close()
		if err != nil {
			log.Err(err).Msg("failed to close bucket")
		}
	}(bucket)

	contentType := http.DetectContentType(contentBytes[:min(512, len(contentBytes))])
	options := &blob.WriterOptions{
		ContentType: contentType,
		ContentMD5:  md5Hasher.Sum(nil),
	}
	relativePath := r.path(itemGroup.ID, fmt.Sprintf("%x", hashOut))
	fullPath := r.fullPath(relativePath)
	err = bucket.WriteAll(ctx, fullPath, contentBytes, options)
	if err != nil {
		log.Err(err).Msg("failed to write file to bucket")
		return UploadResult{}, err
	}

	return UploadResult{
		Path:        relativePath,
		ContentType: contentType,
	}, nil
}

// UploadFileByGroupID uploads a blob for a group looked up by ID. Used by callers
// (e.g. template photos) that don't already hold the *ent.Group.
func (r *AttachmentRepo) UploadFileByGroupID(ctx context.Context, gid uuid.UUID, doc ItemCreateAttachment) (UploadResult, error) {
	grp, err := r.db.Group.Get(ctx, gid)
	if err != nil {
		return UploadResult{}, err
	}
	return r.UploadFile(ctx, grp, doc)
}

func isImageFile(mimetype string) bool {
	// Check file extension for image types
	return strings.Contains(mimetype, "image/jpeg") || strings.Contains(mimetype, "image/png") || strings.Contains(mimetype, "image/gif")
}

func validateImageConfig(config image.Config) error {
	if config.Width <= 0 || config.Height <= 0 {
		return fmt.Errorf("invalid image dimensions %dx%d", config.Width, config.Height)
	}
	if config.Width > maxThumbnailSourceDimension || config.Height > maxThumbnailSourceDimension {
		return fmt.Errorf(
			"image dimensions %dx%d exceed maximum dimension %d",
			config.Width,
			config.Height,
			maxThumbnailSourceDimension,
		)
	}
	if int64(config.Width)*int64(config.Height) > maxThumbnailSourcePixels {
		return fmt.Errorf(
			"image dimensions %dx%d exceed maximum pixel count %d",
			config.Width,
			config.Height,
			maxThumbnailSourcePixels,
		)
	}
	return nil
}

func validateThumbnailSourceDimensions(contentType string, content []byte) error {
	reader := bytes.NewReader(content)
	var (
		config image.Config
		err    error
	)

	switch {
	case isImageFile(contentType):
		config, _, err = image.DecodeConfig(reader)
	case contentType == "image/webp":
		config, err = webp.DecodeConfig(reader)
	case contentType == "image/avif":
		config, err = avif.DecodeConfig(reader)
	case contentType == "image/heic" || contentType == "image/heif":
		config, err = heic.DecodeConfig(reader)
	case contentType == "image/jxl":
		config, err = jpegxl.DecodeConfig(reader)
	default:
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode image dimensions: %w", err)
	}
	return validateImageConfig(config)
}

// calculateThumbnailDimensions calculates new dimensions that preserve aspect ratio
// while fitting within the configured maximum width and height
func calculateThumbnailDimensions(origWidth, origHeight, maxWidth, maxHeight int) (int, int) {
	if origWidth <= maxWidth && origHeight <= maxHeight {
		return origWidth, origHeight
	}

	// Calculate scaling factors for both dimensions
	scaleX := float64(maxWidth) / float64(origWidth)
	scaleY := float64(maxHeight) / float64(origHeight)

	// Use the smaller scaling factor to ensure both dimensions fit
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	newWidth := int(float64(origWidth) * scale)
	newHeight := int(float64(origHeight) * scale)

	// Ensure we don't get zero dimensions
	if newWidth < 1 {
		newWidth = 1
	}
	if newHeight < 1 {
		newHeight = 1
	}

	return newWidth, newHeight
}

// processThumbnailFromImage handles the common thumbnail processing logic after image decoding
// Returns the thumbnail file path or an error
func (r *AttachmentRepo) processThumbnailFromImage(ctx context.Context, groupId uuid.UUID, img image.Image, title string, orientation uint16) (UploadResult, error) {
	ctx, span := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.processThumbnailFromImage")
	defer span.End()

	bounds := img.Bounds()

	_, exifSpan := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.processThumbnailFromImage.exif")
	// Apply EXIF orientation if needed
	if orientation > 1 {
		img = utils.ApplyOrientation(img, orientation)
		bounds = img.Bounds()
	}
	exifSpan.End()

	_, resizeSpan := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.processThumbnailFromImage.resize")
	newWidth, newHeight := calculateThumbnailDimensions(bounds.Dx(), bounds.Dy(), r.thumbnail.Width, r.thumbnail.Height)
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(dst, dst.Rect, img, img.Bounds(), draw.Over, nil)
	resizeSpan.End()

	_, encodeSpan := otel.Tracer("data").Start(ctx, "repo.AttachmentRepo.processThumbnailFromImage.encode")
	buf := new(bytes.Buffer)
	err := webp.Encode(buf, dst, webp.Options{Quality: 80, Lossless: false})
	if err != nil {
		return UploadResult{}, err
	}
	contentBytes := buf.Bytes()
	encodeSpan.End()

	log.Debug().Msg("uploading thumbnail file")
	// Get the group for uploading the thumbnail
	group, err := r.db.Group.Get(ctx, groupId)
	if err != nil {
		return UploadResult{}, err
	}

	uploadResult, err := r.UploadFile(ctx, group, ItemCreateAttachment{
		Title:   fmt.Sprintf("%s-thumb", title),
		Content: bytes.NewReader(contentBytes),
	})
	if err != nil {
		log.Err(err).Msg("failed to upload thumbnail file")
		return UploadResult{}, err
	}

	return uploadResult, nil
}
