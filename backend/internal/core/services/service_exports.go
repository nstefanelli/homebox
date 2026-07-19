package services

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"gocloud.dev/blob"
	"gocloud.dev/pubsub"

	"github.com/sysadminsmedia/homebox/backend/internal/core/services/reporting/eventbus"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entity"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entitytemplate"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entitytype"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/group"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/notifier"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/tag"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/pkgs/utils"
)

// ExportSchemaVersion is the on-disk version of the export zip layout.
// Bump this when manifest/file shapes change in incompatible ways and import
// can no longer round-trip an older export.
const ExportSchemaVersion = 1

// Frequently referenced on-disk table names.
const (
	entitiesTable        = "entities"
	entityTemplatesTable = "entity_templates"
)

// Pubsub topic names used by the export and import workers.
const (
	TopicCollectionExport = "collection_export"
	TopicCollectionImport = "collection_import"
)

// ManifestFile is the name of the manifest entry inside the zip artifact.
const manifestFile = "manifest.json"

// attachmentsDir is the prefix inside the zip for attachment blobs.
const attachmentsDir = "attachments/"

// templatePhotosDir stores template photo blobs by source template UUID.
const templatePhotosDir = "template-photos/"

const (
	maxZipExpansionRatio        = uint64(100)
	maxImportUncompressedBytes  = uint64(4 << 30)
	maxImportJSONEntryBytes     = uint64(128 << 20)
	maxImportManifestEntryBytes = uint64(1 << 20)
	maxImportZipEntries         = 100_000
)

// tableSpec describes how to extract one table's rows scoped to a group, and
// how to handle foreign keys on import.
//
// New fields/columns flow through automatically: export uses SELECT * and
// import builds INSERT from the JSON keys. Adding a new TABLE still requires
// editing this list and (probably) the dependency graph; same for adding a
// new FK column to an existing table that points at another exported table.
type tableSpec struct {
	// name is the SQL table name.
	name string
	// scope is a SQL WHERE fragment with one ? placeholder for the group ID.
	// Use "" to fetch every row in the table.
	scope string
	// pkCol is the primary-key column name. "" for junction tables that have
	// no single-column PK (e.g. tag_entities).
	pkCol string
	// groupCols are columns whose values are remapped to the destination
	// group_id on import (the various "group_xxx" FK columns).
	groupCols []string
	// userCols are columns whose values are remapped to the importing user
	// (notifiers being the only example).
	userCols []string
	// fkCols are immediate foreign keys: { column → target table }. The
	// import looks each value up in the id map populated by earlier table
	// inserts and substitutes the new id.
	fkCols map[string]string
	// deferCols are foreign keys whose target row may not exist yet at the
	// time this row is inserted (self-references and forward-circular refs).
	// They are nulled on insert and patched in a second pass.
	deferCols map[string]string
}

// exportTables defines the export/import schema. Order matters: imports run
// in this order, so each table's non-deferred FK targets must already be
// present.
//
// Why every PK is remapped on import: a real "fresh server" import would
// keep original IDs, but if the user re-imports a backup into the same
// server (or a server that already received this export once), reusing PKs
// causes UNIQUE-constraint violations. Remapping always = simpler invariant.
//
// Self-referential FKs (entities.entity_children, tags.tag_children,
// attachments.attachment_thumbnail) and forward-circular FKs
// (entity_types.entity_type_default_template,
// entity_templates.entity_template_location) live in deferCols so the first
// INSERT pass can succeed; the second pass patches them with remapped IDs.
var exportTables = []tableSpec{
	{
		name:      "entity_types",
		scope:     "group_entity_types = ?",
		pkCol:     "id",
		groupCols: []string{"group_entity_types"},
		deferCols: map[string]string{"entity_type_default_template": entityTemplatesTable},
	},
	{
		name:      entityTemplatesTable,
		scope:     "group_entity_templates = ?",
		pkCol:     "id",
		groupCols: []string{"group_entity_templates"},
		deferCols: map[string]string{"entity_template_location": entitiesTable},
	},
	{
		name:   "template_fields",
		scope:  "entity_template_fields IN (SELECT id FROM entity_templates WHERE group_entity_templates = ?)",
		pkCol:  "id",
		fkCols: map[string]string{"entity_template_fields": entityTemplatesTable},
	},
	{
		name:      "tags",
		scope:     "group_tags = ?",
		pkCol:     "id",
		groupCols: []string{"group_tags"},
		deferCols: map[string]string{"tag_children": "tags"},
	},
	{
		name:      entitiesTable,
		scope:     "group_entities = ?",
		pkCol:     "id",
		groupCols: []string{"group_entities"},
		fkCols:    map[string]string{"entity_type_entities": "entity_types"},
		deferCols: map[string]string{"entity_children": entitiesTable},
	},
	{
		name:   "entity_fields",
		scope:  "entity_fields IN (SELECT id FROM entities WHERE group_entities = ?)",
		pkCol:  "id",
		fkCols: map[string]string{"entity_fields": entitiesTable},
	},
	{
		name:   "maintenance_entries",
		scope:  "entity_id IN (SELECT id FROM entities WHERE group_entities = ?)",
		pkCol:  "id",
		fkCols: map[string]string{"entity_id": entitiesTable},
	},
	{
		// Two-part scope: the regular attachments owned by an entity in this
		// group, PLUS the thumbnail rows those attachments point at (which
		// have entity_attachments=NULL and are linked only via
		// attachment_thumbnail on the parent). Each ? is the same gid;
		// dumpTable/wipeGroup expand based on placeholder count.
		name: "attachments",
		scope: "entity_attachments IN (SELECT id FROM entities WHERE group_entities = ?)" +
			" OR id IN (SELECT attachment_thumbnail FROM attachments" +
			" WHERE attachment_thumbnail IS NOT NULL" +
			" AND entity_attachments IN (SELECT id FROM entities WHERE group_entities = ?))",
		pkCol:     "id",
		fkCols:    map[string]string{"entity_attachments": entitiesTable},
		deferCols: map[string]string{"attachment_thumbnail": "attachments"},
	},
	{
		name:   "tag_entities",
		scope:  "tag_id IN (SELECT id FROM tags WHERE group_tags = ?)",
		fkCols: map[string]string{"tag_id": "tags", "entity_id": entitiesTable},
	},
	{
		name:      "notifiers",
		scope:     "group_id = ?",
		pkCol:     "id",
		groupCols: []string{"group_id"},
		userCols:  []string{"user_id"},
	},
}

// Manifest is the contents of manifest.json inside the export zip.
type Manifest struct {
	SchemaVersion  int            `json:"schemaVersion"`
	ExportedAt     time.Time      `json:"exportedAt"`
	GroupID        uuid.UUID      `json:"groupId"`
	HomeboxVersion string         `json:"homeboxVersion,omitempty"`
	Counts         map[string]int `json:"counts"`
}

// ExportService orchestrates the export and import jobs. It is wired into
// AllServices and invoked by the pubsub workers in app/api/recurring.go.
//
// Every public method takes the requesting tenant's group id and refuses to
// operate on data that does not belong to that group.
type ExportService struct {
	db         *ent.Client
	repos      *repo.AllRepos
	bus        *eventbus.EventBus
	storage    config.Storage
	pubSubConn string
	dialect    string // "sqlite3" or "postgres"
}

// Enqueue creates a pending Export row for gid and publishes a job to the
// export topic. The actual zip-building happens in the worker.
func (s *ExportService) Enqueue(ctx context.Context, gid uuid.UUID) (repo.ExportOut, error) {
	ctx, span := otel.Tracer("services").Start(ctx, "ExportService.Enqueue")
	defer span.End()

	out, err := s.repos.Exports.Create(ctx, gid)
	if err != nil {
		return out, err
	}

	if err := s.publishExportJob(ctx, gid, out.ID); err != nil {
		_ = s.repos.Exports.SetFailed(ctx, gid, out.ID, "failed to enqueue: "+err.Error())
		return out, err
	}

	s.publishMutation(gid)
	return out, nil
}

// EnqueueImport creates a tracked import row pointing at the zip already
// staged at uploadKey and publishes a job for the worker to pick up. The
// returned row carries the ID the frontend can poll for progress.
// uploadKey must live under "{gid}/imports/" — the worker re-validates
// this before reading.
func (s *ExportService) EnqueueImport(ctx context.Context, gid uuid.UUID, userID uuid.UUID, uploadKey string, sizeBytes int64) (repo.ExportOut, error) {
	ctx, span := otel.Tracer("services").Start(ctx, "ExportService.EnqueueImport")
	defer span.End()

	row, err := s.repos.Exports.CreateImport(ctx, gid, uploadKey, sizeBytes)
	if err != nil {
		return row, err
	}

	if err := s.publishImportJob(ctx, gid, userID, row.ID); err != nil {
		// Mark the row failed so the user sees what happened instead of a
		// permanently-pending entry. Best-effort: if the SetFailed also
		// fails we still return the publish error to the caller.
		_ = s.repos.Exports.SetFailed(ctx, gid, row.ID, "failed to enqueue: "+err.Error())
		return row, err
	}
	return row, nil
}

// IsGroupReadyForImport returns true only when a group is empty or contains
// an exact subset of the starter rows created during registration. Import
// wipes those rows before replaying the archive, so a count-only check is not
// safe: deleting one default and replacing it with one custom row preserves
// the count while turning the wipe into user-data loss.
func (s *ExportService) IsGroupReadyForImport(ctx context.Context, gid uuid.UUID) (bool, error) {
	templates, err := s.db.EntityTemplate.Query().Where(entitytemplate.HasGroupWith(group.ID(gid))).Count(ctx)
	if err != nil {
		return false, err
	}
	if templates != 0 {
		return false, nil
	}

	notifiers, err := s.db.Notifier.Query().Where(notifier.HasGroupWith(group.ID(gid))).Count(ctx)
	if err != nil {
		return false, err
	}
	if notifiers != 0 {
		return false, nil
	}

	types, err := s.db.EntityType.Query().
		Where(entitytype.HasGroupWith(group.ID(gid))).
		WithDefaultTemplate().
		All(ctx)
	if err != nil {
		return false, err
	}
	locationTypeIDs := make(map[uuid.UUID]struct{})
	seenType := map[bool]bool{}
	for _, typ := range types {
		expectedName := "Item"
		if typ.IsLocation {
			expectedName = "Location"
		}
		if seenType[typ.IsLocation] ||
			typ.Name != expectedName ||
			typ.Description != "" ||
			typ.Icon != "" ||
			typ.IsContainer ||
			typ.Edges.DefaultTemplate != nil {
			return false, nil
		}
		seenType[typ.IsLocation] = true
		if typ.IsLocation {
			locationTypeIDs[typ.ID] = struct{}{}
		}
	}

	allowedLocationNames := make(map[string]struct{}, len(defaultLocations()))
	for _, loc := range defaultLocations() {
		allowedLocationNames[loc.Name] = struct{}{}
	}
	seenLocations := make(map[string]struct{}, len(allowedLocationNames))
	entities, err := s.db.Entity.Query().
		Where(entity.HasGroupWith(group.ID(gid))).
		WithEntityType().
		WithParent().
		WithTag().
		WithFields().
		WithMaintenanceEntries().
		WithAttachments().
		All(ctx)
	if err != nil {
		return false, err
	}
	for _, entRow := range entities {
		if !isSeedLocation(entRow, locationTypeIDs, allowedLocationNames) {
			return false, nil
		}
		if _, duplicate := seenLocations[entRow.Name]; duplicate {
			return false, nil
		}
		seenLocations[entRow.Name] = struct{}{}
	}

	allowedTagNames := make(map[string]struct{}, len(defaultTags()))
	for _, seedTag := range defaultTags() {
		allowedTagNames[seedTag.Name] = struct{}{}
	}
	seenTags := make(map[string]struct{}, len(allowedTagNames))
	tags, err := s.db.Tag.Query().
		Where(tag.HasGroupWith(group.ID(gid))).
		WithParent().
		WithEntities().
		All(ctx)
	if err != nil {
		return false, err
	}
	for _, tagRow := range tags {
		if _, allowed := allowedTagNames[tagRow.Name]; !allowed ||
			tagRow.Description != "" ||
			tagRow.Color != "" ||
			tagRow.Icon != "" ||
			tagRow.Edges.Parent != nil ||
			len(tagRow.Edges.Entities) != 0 {
			return false, nil
		}
		if _, duplicate := seenTags[tagRow.Name]; duplicate {
			return false, nil
		}
		seenTags[tagRow.Name] = struct{}{}
	}

	return true, nil
}

func isSeedLocation(
	row *ent.Entity,
	locationTypeIDs map[uuid.UUID]struct{},
	allowedNames map[string]struct{},
) bool {
	if row.Edges.EntityType == nil {
		return false
	}
	if _, ok := locationTypeIDs[row.Edges.EntityType.ID]; !ok {
		return false
	}
	if _, ok := allowedNames[row.Name]; !ok {
		return false
	}

	return row.Description == "" &&
		row.ImportRef == "" &&
		row.Notes == "" &&
		row.Quantity == 0 &&
		!row.Insured &&
		!row.Archived &&
		row.AssetID == 0 &&
		!row.SyncChildEntityLocations &&
		row.SerialNumber == "" &&
		row.ModelNumber == "" &&
		row.Manufacturer == "" &&
		row.Icon == "" &&
		!row.LifetimeWarranty &&
		row.WarrantyExpires.IsZero() &&
		row.WarrantyDetails == "" &&
		row.PurchaseDate.IsZero() &&
		row.PurchaseFrom == "" &&
		row.PurchasePrice == 0 &&
		row.SoldDate.IsZero() &&
		row.SoldTo == "" &&
		row.SoldPrice == 0 &&
		row.SoldNotes == "" &&
		row.Edges.Parent == nil &&
		len(row.Edges.Tag) == 0 &&
		len(row.Edges.Fields) == 0 &&
		len(row.Edges.MaintenanceEntries) == 0 &&
		len(row.Edges.Attachments) == 0
}

// RunExport is invoked by the pubsub subscriber when an export job message is
// received. It transitions the row through running → completed/failed and
// uploads the artifact to blob storage.
func (s *ExportService) RunExport(ctx context.Context, exportID, gid uuid.UUID) {
	ctx, span := otel.Tracer("services").Start(ctx, "ExportService.RunExport")
	defer span.End()

	exp, err := s.repos.Exports.Get(ctx, gid, exportID)
	if err != nil {
		log.Err(err).Stringer("export_id", exportID).Stringer("gid", gid).Msg("export job: row not found or wrong group")
		return
	}
	if exp.Status != "pending" {
		if exp.Status == "running" {
			err := errors.New("export was interrupted while running; start a new export")
			if setErr := s.repos.Exports.SetFailed(ctx, gid, exportID, err.Error()); setErr != nil {
				log.Error().Err(setErr).Stringer("export_id", exportID).Msg("export job: could not persist interrupted state")
			}
			s.publishMutation(gid)
		}
		log.Warn().Stringer("export_id", exportID).Str("status", exp.Status).Msg("export job: not pending, skipping")
		return
	}

	if err := s.repos.Exports.SetRunning(ctx, gid, exportID); err != nil {
		log.Err(err).Msg("export job: failed to mark running")
		return
	}
	s.publishMutation(gid)

	artifactPath, sizeBytes, err := s.buildArtifact(ctx, exportID, gid)
	if err != nil {
		log.Err(err).Stringer("export_id", exportID).Msg("export job: failed")
		_ = s.repos.Exports.SetFailed(ctx, gid, exportID, err.Error())
		s.publishMutation(gid)
		return
	}

	if err := s.repos.Exports.SetCompleted(ctx, gid, exportID, artifactPath, sizeBytes); err != nil {
		log.Err(err).Msg("export job: failed to mark completed")
	}
	s.publishMutation(gid)
}

// buildArtifact does the actual zip generation: dump every group-scoped
// table to JSON, copy attachment blobs, write manifest, upload to blob
// storage. Returns the blob key and total size.
func (s *ExportService) buildArtifact(ctx context.Context, exportID, gid uuid.UUID) (string, int64, error) {
	tmp, err := os.CreateTemp("", fmt.Sprintf("homebox-export-%s-*.zip", exportID))
	if err != nil {
		return "", 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	zw := zip.NewWriter(tmp)

	counts := make(map[string]int)
	dbSql := s.db.Sql()
	for i, spec := range exportTables {
		rows, err := dumpTable(ctx, dbSql, s.dialect, spec, gid)
		if err != nil {
			_ = zw.Close()
			return "", 0, fmt.Errorf("dump %s: %w", spec.name, err)
		}
		counts[spec.name] = len(rows)

		w, err := zw.Create(spec.name + ".json")
		if err != nil {
			_ = zw.Close()
			return "", 0, fmt.Errorf("zip create %s.json: %w", spec.name, err)
		}
		enc := json.NewEncoder(w)
		if err := enc.Encode(rows); err != nil {
			_ = zw.Close()
			return "", 0, fmt.Errorf("zip encode %s.json: %w", spec.name, err)
		}

		// Coarse-grained progress: 0..80% spans the table dumps, 80..95% the
		// attachment copies, 95..100% the upload.
		pct := int(float64(i+1) / float64(len(exportTables)) * 80)
		_ = s.repos.Exports.SetProgress(ctx, gid, exportID, pct)
	}

	// Copy attachment blobs into the zip.
	if err := s.copyAttachmentBlobs(ctx, zw, gid); err != nil {
		_ = zw.Close()
		return "", 0, fmt.Errorf("copy attachments: %w", err)
	}
	if err := s.copyTemplatePhotoBlobs(ctx, zw, gid); err != nil {
		_ = zw.Close()
		return "", 0, fmt.Errorf("copy template photos: %w", err)
	}
	_ = s.repos.Exports.SetProgress(ctx, gid, exportID, 95)

	// Manifest last so we know the counts.
	mf := Manifest{
		SchemaVersion: ExportSchemaVersion,
		ExportedAt:    time.Now().UTC(),
		GroupID:       gid,
		Counts:        counts,
	}
	mw, err := zw.Create(manifestFile)
	if err != nil {
		_ = zw.Close()
		return "", 0, fmt.Errorf("zip create manifest: %w", err)
	}
	if err := json.NewEncoder(mw).Encode(mf); err != nil {
		_ = zw.Close()
		return "", 0, fmt.Errorf("zip encode manifest: %w", err)
	}

	if err := zw.Close(); err != nil {
		return "", 0, fmt.Errorf("zip close: %w", err)
	}

	// Upload to blob storage.
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", 0, fmt.Errorf("seek temp: %w", err)
	}
	stat, err := tmp.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("stat temp: %w", err)
	}
	size := stat.Size()

	artifactPath := fmt.Sprintf("%s/exports/%s.zip", gid.String(), exportID.String())
	bucket, err := blob.OpenBucket(ctx, s.repos.Attachments.GetConnString())
	if err != nil {
		return "", 0, fmt.Errorf("open bucket: %w", err)
	}
	defer func() { _ = bucket.Close() }()

	bw, err := bucket.NewWriter(ctx, s.repos.Attachments.GetFullPath(artifactPath), &blob.WriterOptions{
		ContentType: "application/zip",
	})
	if err != nil {
		return "", 0, fmt.Errorf("blob writer: %w", err)
	}
	if _, err := io.Copy(bw, tmp); err != nil {
		_ = bw.Close()
		return "", 0, fmt.Errorf("blob copy: %w", err)
	}
	if err := bw.Close(); err != nil {
		return "", 0, fmt.Errorf("blob close: %w", err)
	}

	return artifactPath, size, nil
}

// copyAttachmentBlobs streams every attachment blob in the group — including
// thumbnail rows — into the zip under attachments/{attachment_id}. Lookup on
// the import side uses the file's stem (the attachment UUID) via the id map.
//
// Reuses the attachments tableSpec scope so the row dump and the blob copy
// can never disagree about which attachments belong to the group.
func (s *ExportService) copyAttachmentBlobs(ctx context.Context, zw *zip.Writer, gid uuid.UUID) error {
	var spec tableSpec
	for _, t := range exportTables {
		if t.name == "attachments" {
			spec = t
			break
		}
	}

	q := "SELECT id, path, mime_type FROM attachments WHERE " + rebindPlaceholders(spec.scope, s.dialect)
	args := make([]any, 0, strings.Count(spec.scope, "?"))
	for i := 0; i < cap(args); i++ {
		args = append(args, gid.String())
	}
	rows, err := s.db.Sql().QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	type attRef struct{ id, path, mimeType string }
	var refs []attRef
	for rows.Next() {
		var id, path, mimeType string
		if err := rows.Scan(&id, &path, &mimeType); err != nil {
			return err
		}
		if path == "" || mimeType == repo.MimeTypeLinkURL {
			continue
		}
		refs = append(refs, attRef{id: id, path: path, mimeType: mimeType})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	bucket, err := blob.OpenBucket(ctx, s.repos.Attachments.GetConnString())
	if err != nil {
		return err
	}
	defer func() { _ = bucket.Close() }()

	for _, ref := range refs {
		if err := validateDocumentBlobPath(ref.path, gid); err != nil {
			return fmt.Errorf("attachment %s: %w", ref.id, err)
		}
		r, err := bucket.NewReader(ctx, s.repos.Attachments.GetFullPath(ref.path), nil)
		if err != nil {
			return fmt.Errorf("attachment %s blob %q: %w", ref.id, ref.path, err)
		}
		w, err := zw.Create(attachmentsDir + ref.id)
		if err != nil {
			_ = r.Close()
			return err
		}
		if _, err := io.Copy(w, r); err != nil {
			_ = r.Close()
			return err
		}
		if err := r.Close(); err != nil {
			return err
		}
	}
	return nil
}

// copyTemplatePhotoBlobs writes every template photo under a stable archive
// name keyed by the source template ID. Unlike attachment thumbnails, a
// missing template photo is a hard export error: silently omitting it would
// produce a backup whose template row points at a blob that cannot be restored.
func (s *ExportService) copyTemplatePhotoBlobs(ctx context.Context, zw *zip.Writer, gid uuid.UUID) error {
	rows, err := s.db.EntityTemplate.Query().
		Where(entitytemplate.HasGroupWith(group.ID(gid)), entitytemplate.PhotoPathNEQ("")).
		All(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	bucket, err := blob.OpenBucket(ctx, s.repos.Attachments.GetConnString())
	if err != nil {
		return err
	}
	defer func() { _ = bucket.Close() }()

	for _, tmpl := range rows {
		if err := validateDocumentBlobPath(tmpl.PhotoPath, gid); err != nil {
			return fmt.Errorf("template %s: %w", tmpl.ID, err)
		}
		r, err := bucket.NewReader(ctx, s.repos.Attachments.GetFullPath(tmpl.PhotoPath), nil)
		if err != nil {
			return fmt.Errorf("template %s photo %q: %w", tmpl.ID, tmpl.PhotoPath, err)
		}
		w, err := zw.Create(templatePhotosDir + tmpl.ID.String())
		if err != nil {
			_ = r.Close()
			return err
		}
		if _, err := io.Copy(w, r); err != nil {
			_ = r.Close()
			return err
		}
		if err := r.Close(); err != nil {
			return err
		}
	}
	return nil
}

// publishExportJob sends a message on the export topic.
func (s *ExportService) publishExportJob(ctx context.Context, gid, exportID uuid.UUID) error {
	conn, err := utils.GenerateSubPubConn(s.pubSubConn, TopicCollectionExport)
	if err != nil {
		return err
	}
	topic, err := pubsub.OpenTopic(ctx, conn)
	if err != nil {
		return err
	}
	defer func() { _ = topic.Shutdown(ctx) }()
	return topic.Send(ctx, &pubsub.Message{
		Body: []byte("collection_export:" + exportID.String()),
		Metadata: map[string]string{
			"group_id":  gid.String(),
			"export_id": exportID.String(),
		},
	})
}

// publishImportJob sends a message on the import topic. The worker loads
// the tracked import row by importID, reads the staged upload from blob
// storage at the row's artifact_path, unzips, restores into the group
// identified by gid, then deletes the staged upload.
func (s *ExportService) publishImportJob(ctx context.Context, gid, userID, importID uuid.UUID) error {
	conn, err := utils.GenerateSubPubConn(s.pubSubConn, TopicCollectionImport)
	if err != nil {
		return err
	}
	topic, err := pubsub.OpenTopic(ctx, conn)
	if err != nil {
		return err
	}
	defer func() { _ = topic.Shutdown(ctx) }()
	return topic.Send(ctx, &pubsub.Message{
		Body: []byte("collection_import:" + gid.String()),
		Metadata: map[string]string{
			"group_id":  gid.String(),
			"user_id":   userID.String(),
			"import_id": importID.String(),
		},
	})
}

func (s *ExportService) publishMutation(gid uuid.UUID) {
	if s.bus != nil {
		s.bus.Publish(eventbus.EventExportMutation, eventbus.GroupMutationEvent{GID: gid})
	}
}

// dumpTable runs SELECT * for spec.scope and returns each row as a JSON-
// friendly map. UUIDs and JSON-blob columns come back from sqlite as []byte;
// we coerce to string here so json.Marshal does the right thing.
//
// Scope clauses may contain multiple ? placeholders (e.g. for an OR-of-
// subqueries). Each placeholder is filled with the same gid — none of the
// existing scopes need to vary by placeholder.
func dumpTable(ctx context.Context, db *sql.DB, dialect string, spec tableSpec, gid uuid.UUID) ([]map[string]any, error) {
	// #nosec G202 -- spec comes exclusively from the compile-time exportTables
	// allowlist; values in the scope are still passed as query parameters.
	q := "SELECT * FROM " + spec.name
	var args []any
	if spec.scope != "" {
		q += " WHERE " + rebindPlaceholders(spec.scope, dialect)
		for i := 0; i < strings.Count(spec.scope, "?"); i++ {
			args = append(args, gid.String())
		}
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = normalizeScan(vals[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// normalizeScan converts driver-returned values into JSON-marshallable
// shapes. The two big ones: []byte (UUIDs and JSON blobs in sqlite) becomes
// string, and time.Time stays as time.Time so json.Marshal renders RFC3339.
func normalizeScan(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	default:
		return v
	}
}

// rebindPlaceholders rewrites "?" to "$1", "$2", … for postgres. SQLite uses
// "?" natively. Assumes scope clauses use a single placeholder per occurrence.
func rebindPlaceholders(s, dialect string) string {
	if dialect != "postgres" {
		return s
	}
	var b strings.Builder
	n := 0
	for _, ch := range s {
		if ch == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// =============================================================================
// Import path
// =============================================================================

// RunImport is invoked by the pubsub subscriber when an import job message
// is received. It loads the tracked import row, validates the staged
// upload, asserts the destination group is empty, and replays every row.
// Status/progress on the row drives the polling UI on the frontend.
func (s *ExportService) RunImport(ctx context.Context, gid, userID, importID uuid.UUID) {
	ctx, span := otel.Tracer("services").Start(ctx, "ExportService.RunImport")
	defer span.End()

	row, err := s.repos.Exports.Get(ctx, gid, importID)
	if err != nil {
		log.Err(err).Stringer("import_id", importID).Stringer("gid", gid).Msg("import job: row not found or wrong group")
		return
	}
	if row.Kind != "import" {
		log.Error().Stringer("import_id", importID).Str("kind", row.Kind).Msg("import job: row is not an import, refusing")
		return
	}
	if row.Status != "pending" {
		if row.Status == "running" {
			errMsg := "import was interrupted while running; the staged archive was retained so it can be imported again"
			if setErr := s.repos.Exports.SetFailed(ctx, gid, importID, errMsg); setErr != nil {
				log.Error().Err(setErr).Stringer("import_id", importID).Msg("import job: could not persist interrupted state")
			}
			s.publishImportFinished(gid)
		}
		log.Warn().Stringer("import_id", importID).Str("status", row.Status).Msg("import job: not pending, skipping")
		return
	}
	uploadKey := row.ArtifactPath

	// Hard scope check: refuse anything that doesn't live under the caller's
	// group prefix. Defence in depth — the handler already enforced this.
	prefix := gid.String() + "/imports/"
	if !strings.HasPrefix(uploadKey, prefix) {
		log.Error().Str("upload_key", uploadKey).Stringer("gid", gid).Msg("import job: upload key outside group prefix, refusing")
		_ = s.repos.Exports.SetFailed(ctx, gid, importID, "upload outside group prefix")
		s.publishImportFinished(gid)
		return
	}

	if err := s.repos.Exports.SetRunning(ctx, gid, importID); err != nil {
		log.Err(err).Stringer("import_id", importID).Msg("import job: failed to mark running")
		return
	}
	s.publishImportFinished(gid)

	cleanupUpload := false
	if err := s.runImport(ctx, gid, userID, importID, uploadKey, row.SizeBytes); err != nil {
		log.Err(err).Stringer("gid", gid).Msg("import job: failed")
		if setErr := s.repos.Exports.SetFailed(ctx, gid, importID, err.Error()); setErr != nil {
			log.Error().Err(setErr).Stringer("import_id", importID).Msg("import job: failed to persist failure; retaining staged archive")
		} else {
			cleanupUpload = true
		}
	} else {
		// On success the upload zip has been fully restored; keep the row
		// size_bytes (set when the upload was staged) and just flip status.
		if err := s.repos.Exports.SetCompleted(ctx, gid, importID, uploadKey, row.SizeBytes); err != nil {
			log.Err(err).Stringer("import_id", importID).Msg("import job: failed to mark completed; retaining staged archive")
		} else {
			cleanupUpload = true
		}
	}

	// Delete only after the terminal status is durable. If status persistence
	// fails, the staged archive is the only recoverable copy of the operation.
	if cleanupUpload {
		if err := s.deleteUpload(ctx, uploadKey); err != nil {
			log.Warn().Err(err).Str("upload_key", uploadKey).Msg("import job: failed to clean staging upload")
		}
	}

	s.publishImportFinished(gid)
}

func (s *ExportService) runImport(ctx context.Context, gid, userID, importID uuid.UUID, uploadKey string, expectedSize int64) error {
	// setProgress is best-effort: a failed status update is logged but never
	// aborts the import itself — progress is observability, not correctness.
	setProgress := func(pct int) {
		if err := s.repos.Exports.SetProgress(ctx, gid, importID, pct); err != nil {
			log.Warn().Err(err).Stringer("import_id", importID).Int("pct", pct).Msg("import job: failed to update progress")
		}
		s.publishImportFinished(gid)
	}

	// Precondition: no items (non-location entities) in this group. Default
	// seeded locations/tags/entity_types are fine; we wipe them below before
	// restoring.
	ready, err := s.IsGroupReadyForImport(ctx, gid)
	if err != nil {
		return fmt.Errorf("import precondition: %w", err)
	}
	if !ready {
		return errors.New("import requires a collection with no items")
	}

	// Stream the upload to a temp file so we can use archive/zip's seek API.
	bucket, err := blob.OpenBucket(ctx, s.repos.Attachments.GetConnString())
	if err != nil {
		return fmt.Errorf("open bucket: %w", err)
	}
	defer func() { _ = bucket.Close() }()

	r, err := bucket.NewReader(ctx, s.repos.Attachments.GetFullPath(uploadKey), nil)
	if err != nil {
		return fmt.Errorf("open upload: %w", err)
	}
	defer func() { _ = r.Close() }()

	tmp, err := os.CreateTemp("", "homebox-import-*.zip")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	size, err := copyExactSize(tmp, r, expectedSize)
	if err != nil {
		return fmt.Errorf("download upload: %w", err)
	}

	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	if err := enforceZipUncompressedLimit(zr, size); err != nil {
		return err
	}

	mf, err := readManifest(zr)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	if mf.SchemaVersion != ExportSchemaVersion {
		return fmt.Errorf("unsupported schema version %d (this server expects %d)", mf.SchemaVersion, ExportSchemaVersion)
	}
	if err := validateAttachmentBlobArchive(zr); err != nil {
		return fmt.Errorf("validate attachments: %w", err)
	}
	if err := validateTemplatePhotoArchive(zr); err != nil {
		return fmt.Errorf("validate template photos: %w", err)
	}
	// Progress budget: 0–5% download + manifest, ~5–80% reserved for the DB
	// phase (reported once after commit because intermediate setProgress
	// calls would deadlock on SQLite — the write tx holds the single
	// writer lock and ent's pool can't take it), 80–95% per-file blob
	// restore, 95–100% finalization.
	setProgress(5)

	// All DB work — the seed wipe, every row insert, and the deferred FK
	// patches — runs in a single tx so the group never sits in a half-imported
	// state. If anything below fails, the deferred Rollback unwinds the wipe
	// too. Blob uploads and bus notifications run only after Commit because
	// (a) blobs are not transactional, and (b) restoreAttachmentBlobs needs to
	// look up rows via the ent client, which uses its own pool and would not
	// see uncommitted writes under Postgres READ COMMITTED.
	tx, err := s.db.Sql().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Wipe the seeded defaults (locations, tags, entity_types, notifiers,
	// etc.) so the imported collection isn't mixed with the auto-created
	// starter content. The empty-group precondition above guarantees this is
	// safe — there are no user-created items to lose.
	if err := wipeGroup(ctx, tx, s.dialect, gid); err != nil {
		return fmt.Errorf("wipe before import: %w", err)
	}

	idMap, err := s.replayImportRows(ctx, tx, zr, gid, userID, mf.GroupID)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}
	setProgress(80)

	// Restore attachment blobs. The zip names them attachments/{old_uuid};
	// look up the new attachment row through the id map. Must run post-commit
	// because the lookup goes through the ent client, which uses a different
	// connection than our tx.
	blobProgress := func(done, total int) {
		if total <= 0 {
			return
		}
		setProgress(80 + int(float64(done)/float64(total)*15))
	}
	if err := s.restoreAttachmentBlobs(ctx, zr, idMap["attachments"], blobProgress); err != nil {
		// Compensating cleanup. The tx is already committed, so a partial blob
		// restore leaves rows pointing at blobs that don't exist on disk and —
		// because IsGroupReadyForImport rejects non-empty groups — blocks any
		// retry. Wipe the freshly-imported rows so the group goes back to its
		// pre-import (empty) state. Successfully uploaded blobs are left on
		// disk; on retry the same content hashes will write to the same paths.
		if werr := wipeGroup(ctx, s.db.Sql(), s.dialect, gid); werr != nil {
			log.Err(werr).Stringer("gid", gid).Msg("import job: blob restore failed and rollback wipe also failed; group left in partially imported state")
		}
		return fmt.Errorf("restore attachments: %w", err)
	}
	if err := s.restoreTemplatePhotoBlobs(ctx, zr, idMap[entityTemplatesTable]); err != nil {
		if werr := wipeGroup(ctx, s.db.Sql(), s.dialect, gid); werr != nil {
			log.Err(werr).Stringer("gid", gid).Msg("import job: template photo restore failed and rollback wipe also failed")
		}
		return fmt.Errorf("restore template photos: %w", err)
	}
	setProgress(95)

	// Notify the frontend that lots of things just appeared.
	if s.bus != nil {
		s.bus.Publish(eventbus.EventEntityMutation, eventbus.GroupMutationEvent{GID: gid})
		s.bus.Publish(eventbus.EventTagMutation, eventbus.GroupMutationEvent{GID: gid})
	}
	return nil
}

func copyExactSize(dst io.Writer, src io.Reader, expectedSize int64) (int64, error) {
	if expectedSize <= 0 || expectedSize == int64(^uint64(0)>>1) {
		return 0, fmt.Errorf("invalid recorded upload size %d", expectedSize)
	}
	n, err := io.Copy(dst, io.LimitReader(src, expectedSize+1))
	if err != nil {
		return n, err
	}
	if n != expectedSize {
		return n, fmt.Errorf("staged upload size mismatch: recorded %d bytes, read %d", expectedSize, n)
	}
	return n, nil
}

// restoreAttachmentBlobs iterates attachments/* in the zip and writes each
// file to blob storage at the path recorded on the matching attachment row.
// Filenames in the zip use the source-side attachment UUID; idMap translates
// to the new UUID assigned during the row import. The optional onProgress
// callback is invoked after each blob is written so the import row's
// progress field stays current during what can be the slowest phase of a
// restore.
func (s *ExportService) restoreAttachmentBlobs(ctx context.Context, zr *zip.Reader, idMap map[string]string, onProgress func(done, total int)) error {
	bucket, err := blob.OpenBucket(ctx, s.repos.Attachments.GetConnString())
	if err != nil {
		return err
	}
	defer func() { _ = bucket.Close() }()

	// Pre-count blob entries so onProgress can report a meaningful ratio.
	total := 0
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, attachmentsDir) && !f.FileInfo().IsDir() {
			total++
		}
	}
	done := 0

	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, attachmentsDir) || f.FileInfo().IsDir() {
			continue
		}
		oldIDStr := strings.TrimPrefix(f.Name, attachmentsDir)
		newIDStr, ok := idMap[oldIDStr]
		if !ok {
			return fmt.Errorf("attachment blob %q has no remapped row", f.Name)
		}
		id, err := uuid.Parse(newIDStr)
		if err != nil {
			return fmt.Errorf("attachment blob %q has invalid remapped id: %w", f.Name, err)
		}
		att, err := s.db.Attachment.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("load attachment %s for blob %q: %w", id, f.Name, err)
		}
		zf, err := f.Open()
		if err != nil {
			return err
		}
		w, err := bucket.NewWriter(ctx, s.repos.Attachments.GetFullPath(att.Path), &blob.WriterOptions{
			ContentType: att.MimeType,
		})
		if err != nil {
			_ = zf.Close()
			return err
		}
		if err := copyDeclaredZipEntry(w, zf, f.UncompressedSize64); err != nil {
			_ = w.Close()
			_ = zf.Close()
			return fmt.Errorf("restore attachment blob %q: %w", f.Name, err)
		}
		if err := w.Close(); err != nil {
			_ = zf.Close()
			return err
		}
		_ = zf.Close()
		done++
		if onProgress != nil {
			onProgress(done, total)
		}
	}
	return nil
}

func (s *ExportService) restoreTemplatePhotoBlobs(
	ctx context.Context,
	zr *zip.Reader,
	idMap map[string]string,
) error {
	bucket, err := blob.OpenBucket(ctx, s.repos.Attachments.GetConnString())
	if err != nil {
		return err
	}
	defer func() { _ = bucket.Close() }()

	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, templatePhotosDir) || f.FileInfo().IsDir() {
			continue
		}
		oldID := strings.TrimPrefix(f.Name, templatePhotosDir)
		newIDString, ok := idMap[oldID]
		if !ok {
			return fmt.Errorf("template photo %q has no remapped template row", f.Name)
		}
		newID, err := uuid.Parse(newIDString)
		if err != nil {
			return fmt.Errorf("template photo %q has invalid remapped template id: %w", f.Name, err)
		}
		tmpl, err := s.db.EntityTemplate.Get(ctx, newID)
		if err != nil {
			return fmt.Errorf("load template %s for photo: %w", newID, err)
		}
		if tmpl.PhotoPath == "" {
			return fmt.Errorf("template %s has archive photo but no photo path", newID)
		}
		zf, err := f.Open()
		if err != nil {
			return err
		}
		w, err := bucket.NewWriter(ctx, s.repos.Attachments.GetFullPath(tmpl.PhotoPath), &blob.WriterOptions{
			ContentType: tmpl.PhotoMimeType,
		})
		if err != nil {
			_ = zf.Close()
			return err
		}
		if err := copyDeclaredZipEntry(w, zf, f.UncompressedSize64); err != nil {
			_ = w.Close()
			_ = zf.Close()
			return fmt.Errorf("restore template photo %q: %w", f.Name, err)
		}
		if err := w.Close(); err != nil {
			_ = zf.Close()
			return err
		}
		if err := zf.Close(); err != nil {
			return err
		}
	}
	return nil
}

// validateAttachmentBlobArchive requires a one-to-one match between every
// non-link attachment row and attachments/<source-attachment-uuid>. This
// prevents a truncated or crafted backup from committing rows that point at
// absent blobs, and rejects orphan/duplicate blob entries before DB writes.
func validateAttachmentBlobArchive(zr *zip.Reader) error {
	rows, err := readTableJSON(zr, "attachments.json")
	if err != nil {
		return err
	}
	expected := make(map[string]struct{})
	for _, row := range rows {
		if fmt.Sprint(row["mime_type"]) == repo.MimeTypeLinkURL {
			continue
		}
		id := fmt.Sprint(row["id"])
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("file attachment has invalid id %q", id)
		}
		if _, duplicate := expected[id]; duplicate {
			return fmt.Errorf("duplicate attachment row id %q", id)
		}
		expected[id] = struct{}{}
	}

	seen := make(map[string]struct{})
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, attachmentsDir) || f.FileInfo().IsDir() {
			continue
		}
		oldID := strings.TrimPrefix(f.Name, attachmentsDir)
		if _, err := uuid.Parse(oldID); err != nil {
			return fmt.Errorf("invalid attachment blob entry %q", f.Name)
		}
		if _, ok := expected[oldID]; !ok {
			return fmt.Errorf("attachment blob entry %q has no matching row", f.Name)
		}
		if _, duplicate := seen[oldID]; duplicate {
			return fmt.Errorf("duplicate attachment blob entry %q", f.Name)
		}
		seen[oldID] = struct{}{}
	}
	for id := range expected {
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("attachment %s references a file but its blob is missing", id)
		}
	}
	return nil
}

// validateTemplatePhotoArchive requires a one-to-one match between template
// rows with photo_path and template-photos/<source-template-uuid> entries.
// This runs before the destination transaction is opened.
func validateTemplatePhotoArchive(zr *zip.Reader) error {
	rows, err := readTableJSON(zr, "entity_templates.json")
	if err != nil {
		return err
	}
	expected := make(map[string]struct{})
	for _, row := range rows {
		photoPath, _ := row["photo_path"].(string)
		if photoPath == "" {
			continue
		}
		id := fmt.Sprint(row["id"])
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("template with photo has invalid id %q", id)
		}
		expected[id] = struct{}{}
	}

	seen := make(map[string]struct{})
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, templatePhotosDir) || f.FileInfo().IsDir() {
			continue
		}
		oldID := strings.TrimPrefix(f.Name, templatePhotosDir)
		if _, err := uuid.Parse(oldID); err != nil {
			return fmt.Errorf("invalid template photo entry %q", f.Name)
		}
		if _, ok := expected[oldID]; !ok {
			return fmt.Errorf("template photo entry %q has no matching template row", f.Name)
		}
		if _, duplicate := seen[oldID]; duplicate {
			return fmt.Errorf("duplicate template photo entry %q", f.Name)
		}
		seen[oldID] = struct{}{}
	}
	for id := range expected {
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("template %s references a photo but its blob is missing", id)
		}
	}
	return nil
}

// deleteUpload removes the staged import zip from blob storage.
func (s *ExportService) deleteUpload(ctx context.Context, uploadKey string) error {
	bucket, err := blob.OpenBucket(ctx, s.repos.Attachments.GetConnString())
	if err != nil {
		return err
	}
	defer func() { _ = bucket.Close() }()
	return bucket.Delete(ctx, s.repos.Attachments.GetFullPath(uploadKey))
}

func (s *ExportService) publishImportFinished(gid uuid.UUID) {
	if s.bus != nil {
		s.bus.Publish(eventbus.EventImportMutation, eventbus.GroupMutationEvent{GID: gid})
	}
}

// readManifest pulls and parses manifest.json out of the zip.
func readManifest(zr *zip.Reader) (Manifest, error) {
	var mf Manifest
	for _, f := range zr.File {
		if f.Name != manifestFile {
			continue
		}
		return mf, readBoundedZipJSON(f, maxImportManifestEntryBytes, &mf)
	}
	return mf, errors.New("manifest.json missing from zip")
}

// readTableJSON loads a single table file from the zip, tolerating its
// absence (returns an empty slice — exports may legitimately omit a table
// with zero rows in future versions).
func readTableJSON(zr *zip.Reader, name string) ([]map[string]any, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		var out []map[string]any
		if err := readBoundedZipJSON(f, maxImportJSONEntryBytes, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, nil
}

func readBoundedZipJSON(f *zip.File, maxBytes uint64, dst any) error {
	if f.UncompressedSize64 > maxBytes {
		return fmt.Errorf("zip entry %q exceeds JSON size limit %d", f.Name, maxBytes)
	}
	if maxBytes >= uint64(math.MaxInt64) {
		return fmt.Errorf("zip entry %q has unsupported JSON size limit %d", f.Name, maxBytes)
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	limit := int64(maxBytes) + 1
	data, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return err
	}
	if uint64(len(data)) > maxBytes {
		return fmt.Errorf("zip entry %q exceeds JSON size limit %d", f.Name, maxBytes)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decode %s: %w", f.Name, err)
	}
	return nil
}

// copyDeclaredZipEntry streams exactly the size declared in the central
// directory and probes one byte beyond it. The archive-wide preflight caps
// declared sizes; this runtime check also fails closed if a malformed
// decompressor produces more or fewer bytes than the declaration.
func copyDeclaredZipEntry(dst io.Writer, src io.Reader, declared uint64) error {
	if declared >= uint64(math.MaxInt64) {
		return fmt.Errorf("declared zip entry size %d is unsupported", declared)
	}
	want := int64(declared)
	n, err := io.CopyN(dst, src, want+1)
	switch {
	case err == nil:
		return fmt.Errorf("zip entry exceeds declared size %d", declared)
	case !errors.Is(err, io.EOF):
		return err
	case n != want:
		return fmt.Errorf("zip entry size mismatch: declared %d, read %d", declared, n)
	default:
		return nil
	}
}

// sqlExecer is the minimal interface used by the import path so the same
// helpers work against a *sql.DB (auto-commit) and a *sql.Tx (transactional
// import). Both stdlib types implement ExecContext with this signature.
type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertRow builds and runs an INSERT for one row's worth of column-value
// pairs. Self-maintaining: every JSON key becomes a column.
func insertRow(ctx context.Context, db sqlExecer, dialect, table string, row map[string]any) error {
	if len(row) == 0 {
		return nil
	}
	// Reject any attacker-shaped identifiers before they reach the SQL
	// builder. Column names flow from JSON keys in an attacker-controlled
	// zip; quoteIdent also escapes embedded quotes, but rejecting up front
	// gives a clear error and keeps the SQL we generate trivial to audit.
	if !isValidSQLIdent(table) {
		return fmt.Errorf("invalid table identifier %q", table)
	}
	cols := make([]string, 0, len(row))
	for k := range row {
		if !isValidSQLIdent(k) {
			return fmt.Errorf("invalid column identifier %q on table %q", k, table)
		}
		cols = append(cols, k)
	}
	// Stable column order so generated SQL is deterministic in tests/logs.
	sortStrings(cols)

	args := make([]any, 0, len(cols))
	placeholders := make([]string, 0, len(cols))
	for i, c := range cols {
		args = append(args, row[c])
		placeholders = append(placeholders, placeholder(dialect, i+1))
	}

	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoteIdent(dialect, table),
		joinQuoted(dialect, cols),
		strings.Join(placeholders, ", "),
	)
	_, err := db.ExecContext(ctx, q, args...)
	return err
}

// placeholder returns the dialect-specific positional placeholder.
func placeholder(dialect string, n int) string {
	if dialect == "postgres" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// quoteIdent quotes an identifier. Both supported dialects accept double
// quotes around identifiers — including sqlite for reserved words like
// "primary" on the attachments table. Any embedded double-quote is escaped
// per the SQL standard (and shared dialect behavior) by doubling it, so a
// stray quote can never close the identifier and inject SQL. Callers should
// still validate identifiers via isValidSQLIdent for attacker-supplied input;
// this escape is defence-in-depth, not the primary gate.
func quoteIdent(_ string, ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// isValidSQLIdent returns true if s is a syntactically conservative SQL
// identifier: an ASCII letter or underscore followed by letters, digits, or
// underscores. The import path runs JSON map keys through this before they
// are interpolated as column names, so a hostile export zip cannot smuggle
// SQL into a table name or column list. dumpTable populates these keys from
// rows.Columns(), which only ever returns plain identifiers, so every legit
// key satisfies this check.
func isValidSQLIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r == '_':
			// always allowed
		case (r >= '0' && r <= '9') && i > 0:
			// digits allowed anywhere except the first character
		default:
			return false
		}
	}
	return true
}

func joinQuoted(dialect string, cols []string) string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = quoteIdent(dialect, c)
	}
	return strings.Join(out, ", ")
}

// enforceZipUncompressedLimit rejects zip bombs before any member is opened.
// The ratio limit handles small highly-compressible archives, while the
// absolute limit keeps a permitted 1GB upload from declaring 100GB of output.
// JSON entries have a tighter cap because replay materializes one table at a
// time; attachment blobs remain streaming and share only the total cap.
func enforceZipUncompressedLimit(zr *zip.Reader, uploadSize int64) error {
	if uploadSize <= 0 {
		return fmt.Errorf("import rejected: invalid compressed size %d", uploadSize)
	}
	if len(zr.File) > maxImportZipEntries {
		return fmt.Errorf("import rejected: zip contains %d entries, exceeds limit %d", len(zr.File), maxImportZipEntries)
	}

	uploadBytes := uint64(uploadSize)
	maxUncompressed := maxImportUncompressedBytes
	if uploadBytes <= ^uint64(0)/maxZipExpansionRatio {
		byRatio := uploadBytes * maxZipExpansionRatio
		if byRatio < maxUncompressed {
			maxUncompressed = byRatio
		}
	}

	var total uint64
	for _, f := range zr.File {
		if f.UncompressedSize64 > maxUncompressed {
			return fmt.Errorf("import rejected: zip entry %q declares uncompressed size %d, exceeds limit %d", f.Name, f.UncompressedSize64, maxUncompressed)
		}
		if f.UncompressedSize64 > maxUncompressed-total {
			return fmt.Errorf("import rejected: zip cumulative uncompressed size exceeds limit %d", maxUncompressed)
		}
		total += f.UncompressedSize64

		if f.Name == manifestFile && f.UncompressedSize64 > maxImportManifestEntryBytes {
			return fmt.Errorf("import rejected: manifest exceeds limit %d", maxImportManifestEntryBytes)
		}
		if strings.HasSuffix(f.Name, ".json") && f.Name != manifestFile &&
			f.UncompressedSize64 > maxImportJSONEntryBytes {
			return fmt.Errorf("import rejected: JSON entry %q exceeds limit %d", f.Name, maxImportJSONEntryBytes)
		}
	}
	return nil
}

// replayImportRows reads each table file from the zip, regenerates every PK,
// remaps group/user/FK columns, rewrites attachment blob paths from the source
// gid prefix to the destination, and inserts the row into tx. Self-referential
// and forward-circular FKs are stashed and patched in a second pass so the
// first INSERT can succeed before the referenced row exists. Returns
// idMap[table][oldID]=newID so the post-commit blob restore can resolve
// attachment file names back to the just-inserted rows.
func (s *ExportService) replayImportRows(ctx context.Context, tx *sql.Tx, zr *zip.Reader, gid, userID, srcGroupID uuid.UUID) (map[string]map[string]string, error) {
	idMap := make(map[string]map[string]string)
	rememberID := func(table, oldID, newID string) {
		if _, ok := idMap[table]; !ok {
			idMap[table] = make(map[string]string)
		}
		idMap[table][oldID] = newID
	}

	// remapFK fails closed when the archive references a row that was not
	// imported. Passing an unknown source UUID through unchanged is unsafe:
	// a global database FK can resolve it to another tenant's existing row.
	remapFK := func(target string, v any) (any, error) {
		if v == nil {
			return nil, nil
		}
		oldID := fmt.Sprint(v)
		if oldID == "" {
			return nil, nil
		}
		if mapping, ok := idMap[target]; ok {
			if newID, found := mapping[oldID]; found {
				return newID, nil
			}
		}
		return nil, fmt.Errorf("unmapped foreign key %q for table %q", oldID, target)
	}

	type deferredUpdate struct {
		table, col, newID, oldFKValue, targetTable string
	}
	var deferred []deferredUpdate
	type deferredUUIDListUpdate struct {
		table, col, newID, targetTable string
		oldIDs                         []string
	}
	var deferredUUIDLists []deferredUUIDListUpdate

	for _, spec := range exportTables {
		rows, err := readTableJSON(zr, spec.name+".json")
		if err != nil {
			return nil, fmt.Errorf("read %s.json: %w", spec.name, err)
		}
		for _, row := range rows {
			newID, err := remapImportRow(row, spec, gid, userID, srcGroupID, remapFK, rememberID)
			if err != nil {
				return nil, err
			}
			if spec.name == entityTemplatesTable {
				oldTagIDs, err := decodeArchivedUUIDList(row["default_tag_ids"])
				if err != nil {
					return nil, fmt.Errorf("decode entity_templates.default_tag_ids: %w", err)
				}
				if len(oldTagIDs) > 0 {
					deferredUUIDLists = append(deferredUUIDLists, deferredUUIDListUpdate{
						table:       spec.name,
						col:         "default_tag_ids",
						newID:       newID,
						targetTable: "tags",
						oldIDs:      oldTagIDs,
					})
				}
				// Tags are imported after templates, so clear the JSON list for
				// the initial insert and patch it once every tag ID is mapped.
				row["default_tag_ids"] = nil
			}
			for col, target := range spec.deferCols {
				if v, ok := row[col]; ok && v != nil && v != "" {
					if newID != "" {
						deferred = append(deferred, deferredUpdate{
							table:       spec.name,
							col:         col,
							newID:       newID,
							oldFKValue:  fmt.Sprint(v),
							targetTable: target,
						})
					}
					row[col] = nil
				}
			}
			if err := insertRow(ctx, tx, s.dialect, spec.name, row); err != nil {
				return nil, fmt.Errorf("insert %s: %w", spec.name, err)
			}
		}
	}

	for _, d := range deferredUUIDLists {
		mappedIDs := make([]string, 0, len(d.oldIDs))
		for _, oldID := range d.oldIDs {
			mapped, err := remapFK(d.targetTable, oldID)
			if err != nil {
				return nil, fmt.Errorf("deferred update %s.%s: %w", d.table, d.col, err)
			}
			mappedIDs = append(mappedIDs, fmt.Sprint(mapped))
		}
		encoded, err := json.Marshal(mappedIDs)
		if err != nil {
			return nil, fmt.Errorf("encode remapped %s.%s: %w", d.table, d.col, err)
		}
		// #nosec G201 -- table/column identifiers originate in the compile-time
		// import schema and are dialect-quoted; all row values are parameterized.
		q := fmt.Sprintf("UPDATE %s SET %s = %s WHERE id = %s",
			quoteIdent(s.dialect, d.table),
			quoteIdent(s.dialect, d.col),
			placeholder(s.dialect, 1),
			placeholder(s.dialect, 2))
		if _, err := tx.ExecContext(ctx, q, string(encoded), d.newID); err != nil {
			return nil, fmt.Errorf("deferred update %s.%s: %w", d.table, d.col, err)
		}
	}

	// Apply deferred updates (self-referential and forward-circular FKs).
	for _, d := range deferred {
		newFK, err := remapFK(d.targetTable, d.oldFKValue)
		if err != nil {
			return nil, fmt.Errorf("deferred update %s.%s: %w", d.table, d.col, err)
		}
		// #nosec G201 -- table/column identifiers originate in the compile-time
		// import schema and are dialect-quoted; all row values are parameterized.
		q := fmt.Sprintf("UPDATE %s SET %s = %s WHERE id = %s",
			quoteIdent(s.dialect, d.table),
			quoteIdent(s.dialect, d.col),
			placeholder(s.dialect, 1),
			placeholder(s.dialect, 2))
		if _, err := tx.ExecContext(ctx, q, newFK, d.newID); err != nil {
			return nil, fmt.Errorf("deferred update %s.%s: %w", d.table, d.col, err)
		}
	}

	return idMap, nil
}

func decodeArchivedUUIDList(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}

	var raw []any
	switch value := v.(type) {
	case string:
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) == "null" {
			return nil, nil
		}
		if err := json.Unmarshal([]byte(value), &raw); err != nil {
			return nil, err
		}
	case []any:
		raw = value
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(encoded, &raw); err != nil {
			return nil, err
		}
	}

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		idString, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("UUID list contains non-string value %T", item)
		}
		id, err := uuid.Parse(idString)
		if err != nil {
			return nil, fmt.Errorf("invalid UUID %q: %w", idString, err)
		}
		out = append(out, id.String())
	}
	return out, nil
}

// remapImportRow rewrites a single row in place: regenerates its PK, swaps
// group/user/FK columns, and validates+rewrites attachment blob paths from
// the source gid prefix to the destination gid. Returns the new PK (empty
// for junction tables with no pkCol) so the caller can record deferred FK
// updates against it.
func remapImportRow(
	row map[string]any,
	spec tableSpec,
	gid, userID, srcGroupID uuid.UUID,
	remapFK func(target string, v any) (any, error),
	rememberID func(table, oldID, newID string),
) (string, error) {
	var newID string
	if spec.pkCol != "" {
		if v, ok := row[spec.pkCol]; ok && v != nil {
			old := fmt.Sprint(v)
			newID = uuid.NewString()
			row[spec.pkCol] = newID
			rememberID(spec.name, old, newID)
		}
	}
	for _, col := range spec.groupCols {
		if _, ok := row[col]; ok {
			row[col] = gid.String()
		}
	}
	for _, col := range spec.userCols {
		if _, ok := row[col]; ok {
			row[col] = userID.String()
		}
	}
	for col, target := range spec.fkCols {
		if v, ok := row[col]; ok {
			mapped, err := remapFK(target, v)
			if err != nil {
				return "", fmt.Errorf("remap %s.%s: %w", spec.name, col, err)
			}
			row[col] = mapped
		}
	}
	// Attachment paths are "{group_id}/documents/{hash}"; rewrite the source
	// gid prefix to the destination so the row points at where we will
	// actually upload the blob and so cascade-cleanup on group delete sweeps
	// it correctly.
	//
	// The zip is attacker-controlled (an admin imports a file they
	// uploaded). Without validation, a crafted path like
	// "{srcGid}/documents/../../etc/foo" would survive rewriteBlobPath
	// (it only swaps the gid prefix) and reach the blob writer; the
	// fileblob backend doesn't resolve ".." segments. Validate the
	// source shape strictly, then re-validate the result.
	if spec.name == "attachments" {
		if err := rewriteAttachmentPath(row, srcGroupID, gid); err != nil {
			return "", err
		}
	}
	if spec.name == entityTemplatesTable {
		if err := rewriteTemplatePhotoPath(row, srcGroupID, gid); err != nil {
			return "", err
		}
	}
	return newID, nil
}

// rewriteAttachmentPath validates the attachment row's path column, swaps
// the source gid prefix for the destination gid, and re-validates the
// result. Mutates row in place.
func rewriteAttachmentPath(row map[string]any, srcGroupID, dstGroupID uuid.UUID) error {
	v, ok := row["path"]
	if !ok {
		return fmt.Errorf("attachment row missing path column")
	}
	str, ok := v.(string)
	if !ok || str == "" {
		return fmt.Errorf("attachment row has empty/non-string path")
	}
	if fmt.Sprint(row["mime_type"]) == repo.MimeTypeLinkURL {
		u, err := url.ParseRequestURI(str)
		if err != nil || u.Host == "" || u.User != nil ||
			(u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("external link attachment has invalid http/https URL")
		}
		return nil
	}
	newPath, err := remapDocumentBlobPath(str, srcGroupID, dstGroupID)
	if err != nil {
		return fmt.Errorf("attachment %w", err)
	}
	row["path"] = newPath
	return nil
}

func rewriteTemplatePhotoPath(row map[string]any, srcGroupID, dstGroupID uuid.UUID) error {
	v, ok := row["photo_path"]
	if !ok || v == nil || v == "" {
		return nil
	}
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("template photo path is not a string")
	}
	newPath, err := remapDocumentBlobPath(str, srcGroupID, dstGroupID)
	if err != nil {
		return fmt.Errorf("template photo %w", err)
	}
	row["photo_path"] = newPath
	return nil
}

func validateDocumentBlobPath(blobPath string, gid uuid.UUID) error {
	_, err := remapDocumentBlobPath(blobPath, gid, gid)
	return err
}

func remapDocumentBlobPath(blobPath string, srcGroupID, dstGroupID uuid.UUID) (string, error) {
	if blobPath == "" || path.Clean(blobPath) != blobPath || strings.HasPrefix(blobPath, "/") {
		return "", fmt.Errorf("path %q is not a canonical relative blob path", blobPath)
	}
	srcPrefix := srcGroupID.String() + "/documents/"
	if !strings.HasPrefix(blobPath, srcPrefix) {
		return "", fmt.Errorf("path %q does not live under source group's documents prefix", blobPath)
	}
	suffix := strings.TrimPrefix(blobPath, srcPrefix)
	if suffix == "" || strings.Contains(suffix, "/") {
		return "", fmt.Errorf("path %q must identify one document blob", blobPath)
	}
	newPath := dstGroupID.String() + "/documents/" + suffix
	dstPrefix := dstGroupID.String() + "/documents/"
	if !strings.HasPrefix(newPath, dstPrefix) || path.Clean(newPath) != newPath {
		return "", fmt.Errorf("rewritten path %q escapes destination group's documents prefix", newPath)
	}
	return newPath, nil
}

// wipeGroup deletes every group-scoped row in the export table list, in
// reverse dependency order. Used before an import so the seeded
// defaults don't pollute the restored collection.
//
// Reusing exportTables means new tables are wiped automatically once they're
// added to the export schema — no separate list to keep in sync.
func wipeGroup(ctx context.Context, db sqlExecer, dialect string, gid uuid.UUID) error {
	for i := len(exportTables) - 1; i >= 0; i-- {
		spec := exportTables[i]
		if spec.scope == "" {
			continue
		}
		q := "DELETE FROM " + quoteIdent(dialect, spec.name) +
			" WHERE " + rebindPlaceholders(spec.scope, dialect)
		args := make([]any, 0, strings.Count(spec.scope, "?"))
		for j := 0; j < cap(args); j++ {
			args = append(args, gid.String())
		}
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("wipe %s: %w", spec.name, err)
		}
	}
	return nil
}

// sortStrings is a tiny inlined sort to keep the file dependency-light.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
