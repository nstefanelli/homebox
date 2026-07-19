package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services/reporting/eventbus"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/entitytemplate"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/group"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/tag"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/templatefield"
)

type EntityTemplatesRepository struct {
	db          *ent.Client
	bus         *eventbus.EventBus
	attachments *AttachmentRepo
}

type (
	TemplateField struct {
		ID           *uuid.UUID `json:"id,omitempty"`
		Type         string     `json:"type"`
		Name         string     `json:"name"`
		TextValue    string     `json:"textValue"`
		NumberValue  int        `json:"numberValue"`
		BooleanValue bool       `json:"booleanValue"`
		TimeValue    time.Time  `json:"timeValue"`
	}

	TemplateTagSummary struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}

	TemplateLocationSummary struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}

	EntityTemplateCreate struct {
		Name        string `json:"name"        validate:"required,min=1,max=255"`
		Description string `json:"description" validate:"max=1000"`
		Notes       string `json:"notes"       validate:"max=1000"`

		// Default values for entities
		DefaultQuantity         *float64 `json:"defaultQuantity,omitempty"        extensions:"x-nullable"`
		DefaultInsured          bool     `json:"defaultInsured"`
		DefaultName             *string  `json:"defaultName,omitempty"            validate:"omitempty,max=255"  extensions:"x-nullable"`
		DefaultDescription      *string  `json:"defaultDescription,omitempty"     validate:"omitempty,max=1000" extensions:"x-nullable"`
		DefaultManufacturer     *string  `json:"defaultManufacturer,omitempty"    validate:"omitempty,max=255"  extensions:"x-nullable"`
		DefaultModelNumber      *string  `json:"defaultModelNumber,omitempty"     validate:"omitempty,max=255"  extensions:"x-nullable"`
		DefaultLifetimeWarranty bool     `json:"defaultLifetimeWarranty"`
		DefaultWarrantyDetails  *string  `json:"defaultWarrantyDetails,omitempty" validate:"omitempty,max=1000" extensions:"x-nullable"`

		// Default location and tags
		DefaultLocationID uuid.UUID    `json:"defaultLocationId,omitempty" extensions:"x-nullable"`
		DefaultTagIDs     *[]uuid.UUID `json:"defaultTagIds,omitempty"     extensions:"x-nullable"`

		// Metadata flags
		IncludeWarrantyFields bool `json:"includeWarrantyFields"`
		IncludePurchaseFields bool `json:"includePurchaseFields"`
		IncludeSoldFields     bool `json:"includeSoldFields"`

		// Custom fields
		Fields []TemplateField `json:"fields"`
	}

	EntityTemplateUpdate struct {
		ID          uuid.UUID `json:"id"`
		Name        string    `json:"name"        validate:"required,min=1,max=255"`
		Description string    `json:"description" validate:"max=1000"`
		Notes       string    `json:"notes"       validate:"max=1000"`

		// Default values for entities
		DefaultQuantity         *float64 `json:"defaultQuantity,omitempty"        extensions:"x-nullable"`
		DefaultInsured          bool     `json:"defaultInsured"`
		DefaultName             *string  `json:"defaultName,omitempty"            validate:"omitempty,max=255"  extensions:"x-nullable"`
		DefaultDescription      *string  `json:"defaultDescription,omitempty"     validate:"omitempty,max=1000" extensions:"x-nullable"`
		DefaultManufacturer     *string  `json:"defaultManufacturer,omitempty"    validate:"omitempty,max=255"  extensions:"x-nullable"`
		DefaultModelNumber      *string  `json:"defaultModelNumber,omitempty"     validate:"omitempty,max=255"  extensions:"x-nullable"`
		DefaultLifetimeWarranty bool     `json:"defaultLifetimeWarranty"`
		DefaultWarrantyDetails  *string  `json:"defaultWarrantyDetails,omitempty" validate:"omitempty,max=1000" extensions:"x-nullable"`

		// Default location and tags
		DefaultLocationID uuid.UUID    `json:"defaultLocationId,omitempty" extensions:"x-nullable"`
		DefaultTagIDs     *[]uuid.UUID `json:"defaultTagIds,omitempty"     extensions:"x-nullable"`

		// Metadata flags
		IncludeWarrantyFields bool `json:"includeWarrantyFields"`
		IncludePurchaseFields bool `json:"includePurchaseFields"`
		IncludeSoldFields     bool `json:"includeSoldFields"`

		// Custom fields
		Fields []TemplateField `json:"fields"`
	}

	EntityTemplateSummary struct {
		ID          uuid.UUID `json:"id"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		CreatedAt   time.Time `json:"createdAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}

	EntityTemplateOut struct {
		ID          uuid.UUID `json:"id"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Notes       string    `json:"notes"`

		// Template photo (copied as the primary photo to entities created from this template)
		PhotoPath     string `json:"photoPath"`
		PhotoMimeType string `json:"photoMimeType"`

		CreatedAt time.Time `json:"createdAt"`
		UpdatedAt time.Time `json:"updatedAt"`

		// Default values for entities
		DefaultQuantity         float64 `json:"defaultQuantity"`
		DefaultInsured          bool    `json:"defaultInsured"`
		DefaultName             string  `json:"defaultName"`
		DefaultDescription      string  `json:"defaultDescription"`
		DefaultManufacturer     string  `json:"defaultManufacturer"`
		DefaultModelNumber      string  `json:"defaultModelNumber"`
		DefaultLifetimeWarranty bool    `json:"defaultLifetimeWarranty"`
		DefaultWarrantyDetails  string  `json:"defaultWarrantyDetails"`

		// Default location and tags
		DefaultLocation *TemplateLocationSummary `json:"defaultLocation"`
		DefaultTags     []TemplateTagSummary     `json:"defaultTags"`

		// Metadata flags
		IncludeWarrantyFields bool `json:"includeWarrantyFields"`
		IncludePurchaseFields bool `json:"includePurchaseFields"`
		IncludeSoldFields     bool `json:"includeSoldFields"`

		// Custom fields
		Fields []TemplateField `json:"fields"`
	}
)

func mapTemplateField(field *ent.TemplateField) TemplateField {
	return TemplateField{
		ID:           lo.ToPtr(field.ID),
		Type:         string(field.Type),
		Name:         field.Name,
		TextValue:    field.TextValue,
		NumberValue:  field.NumberValue,
		BooleanValue: field.BooleanValue,
		TimeValue:    field.TimeValue,
	}
}

func mapTemplateFieldSlice(fields []*ent.TemplateField) []TemplateField {
	return lo.Map(fields, func(field *ent.TemplateField, _ int) TemplateField {
		return mapTemplateField(field)
	})
}

func mapEntityTemplateSummary(template *ent.EntityTemplate) EntityTemplateSummary {
	return EntityTemplateSummary{
		ID:          template.ID,
		Name:        template.Name,
		Description: template.Description,
		CreatedAt:   template.CreatedAt,
		UpdatedAt:   template.UpdatedAt,
	}
}

func (r *EntityTemplatesRepository) mapTemplateOut(ctx context.Context, gid uuid.UUID, template *ent.EntityTemplate) EntityTemplateOut {
	fields := make([]TemplateField, 0)
	if template.Edges.Fields != nil {
		fields = mapTemplateFieldSlice(template.Edges.Fields)
	}

	// Map location if present
	var location *TemplateLocationSummary
	if template.Edges.Location != nil {
		location = &TemplateLocationSummary{
			ID:   template.Edges.Location.ID,
			Name: template.Edges.Location.Name,
		}
	}

	// Fetch tags from database using stored IDs
	tags := make([]TemplateTagSummary, 0)
	if len(template.DefaultTagIds) > 0 {
		tagEntities, err := r.db.Tag.Query().
			Where(
				tag.IDIn(template.DefaultTagIds...),
				tag.HasGroupWith(group.ID(gid)),
			).
			All(ctx)
		if err == nil {
			tags = lo.Map(tagEntities, func(l *ent.Tag, _ int) TemplateTagSummary {
				return TemplateTagSummary{
					ID:   l.ID,
					Name: l.Name,
				}
			})
		}
	}

	return EntityTemplateOut{
		ID:                      template.ID,
		Name:                    template.Name,
		Description:             template.Description,
		Notes:                   template.Notes,
		PhotoPath:               template.PhotoPath,
		PhotoMimeType:           template.PhotoMimeType,
		CreatedAt:               template.CreatedAt,
		UpdatedAt:               template.UpdatedAt,
		DefaultQuantity:         template.DefaultQuantity,
		DefaultInsured:          template.DefaultInsured,
		DefaultName:             template.DefaultName,
		DefaultDescription:      template.DefaultDescription,
		DefaultManufacturer:     template.DefaultManufacturer,
		DefaultModelNumber:      template.DefaultModelNumber,
		DefaultLifetimeWarranty: template.DefaultLifetimeWarranty,
		DefaultWarrantyDetails:  template.DefaultWarrantyDetails,
		DefaultLocation:         location,
		DefaultTags:             tags,
		IncludeWarrantyFields:   template.IncludeWarrantyFields,
		IncludePurchaseFields:   template.IncludePurchaseFields,
		IncludeSoldFields:       template.IncludeSoldFields,
		Fields:                  fields,
	}
}

func (r *EntityTemplatesRepository) publishMutationEvent(gid uuid.UUID) {
	if r.bus != nil {
		r.bus.Publish(eventbus.EventEntityMutation, eventbus.GroupMutationEvent{GID: gid})
	}
}

// GetAll returns all templates for a group
func (r *EntityTemplatesRepository) GetAll(ctx context.Context, gid uuid.UUID) ([]EntityTemplateSummary, error) {
	templates, err := r.db.EntityTemplate.Query().
		Where(entitytemplate.HasGroupWith(group.ID(gid))).
		Order(ent.Asc(entitytemplate.FieldName)).
		All(ctx)

	if err != nil {
		return nil, err
	}

	result := lo.Map(templates, func(template *ent.EntityTemplate, _ int) EntityTemplateSummary {
		return mapEntityTemplateSummary(template)
	})

	return result, nil
}

// GetOne returns a single template by ID, verified to belong to the specified group
func (r *EntityTemplatesRepository) GetOne(ctx context.Context, gid uuid.UUID, id uuid.UUID) (EntityTemplateOut, error) {
	template, err := r.db.EntityTemplate.Query().
		Where(
			entitytemplate.ID(id),
			entitytemplate.HasGroupWith(group.ID(gid)),
		).
		WithFields().
		WithLocation().
		Only(ctx)

	if err != nil {
		return EntityTemplateOut{}, err
	}

	return r.mapTemplateOut(ctx, gid, template), nil
}

// Create creates a new template
func (r *EntityTemplatesRepository) Create(ctx context.Context, gid uuid.UUID, data EntityTemplateCreate) (EntityTemplateOut, error) {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return EntityTemplateOut{}, err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Warn().Err(rollbackErr).Msg("failed to rollback transaction during template create")
			}
		}
	}()

	if err := assertEntityInGroup(ctx, tx.Entity, gid, data.DefaultLocationID); err != nil {
		return EntityTemplateOut{}, err
	}
	if data.DefaultTagIDs != nil {
		if err := assertTagsInGroup(ctx, tx.Tag, gid, *data.DefaultTagIDs); err != nil {
			return EntityTemplateOut{}, err
		}
	}

	q := tx.EntityTemplate.Create().
		SetName(data.Name).
		SetDescription(data.Description).
		SetNotes(data.Notes).
		SetNillableDefaultQuantity(data.DefaultQuantity).
		SetDefaultInsured(data.DefaultInsured).
		SetNillableDefaultName(data.DefaultName).
		SetNillableDefaultDescription(data.DefaultDescription).
		SetNillableDefaultManufacturer(data.DefaultManufacturer).
		SetNillableDefaultModelNumber(data.DefaultModelNumber).
		SetDefaultLifetimeWarranty(data.DefaultLifetimeWarranty).
		SetNillableDefaultWarrantyDetails(data.DefaultWarrantyDetails).
		SetIncludeWarrantyFields(data.IncludeWarrantyFields).
		SetIncludePurchaseFields(data.IncludePurchaseFields).
		SetIncludeSoldFields(data.IncludeSoldFields).
		SetGroupID(gid)

	if data.DefaultLocationID != uuid.Nil {
		q.SetLocationID(data.DefaultLocationID)
	}
	if data.DefaultTagIDs != nil && len(*data.DefaultTagIDs) > 0 {
		q.SetDefaultTagIds(*data.DefaultTagIDs)
	}

	template, err := q.Save(ctx)
	if err != nil {
		return EntityTemplateOut{}, err
	}

	// Create template fields
	for _, field := range data.Fields {
		fieldBuilder := tx.TemplateField.Create().
			SetEntityTemplateID(template.ID).
			SetType(templatefield.Type(field.Type)).
			SetName(field.Name).
			SetTextValue(field.TextValue).
			SetNumberValue(field.NumberValue).
			SetBooleanValue(field.BooleanValue)
		if !field.TimeValue.IsZero() {
			fieldBuilder.SetTimeValue(field.TimeValue)
		}
		_, err = fieldBuilder.Save(ctx)

		if err != nil {
			log.Err(err).Msg("failed to create template field")
			return EntityTemplateOut{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return EntityTemplateOut{}, err
	}
	committed = true

	r.publishMutationEvent(gid)
	return r.GetOne(ctx, gid, template.ID)
}

// Update updates an existing template
func (r *EntityTemplatesRepository) Update(ctx context.Context, gid uuid.UUID, data EntityTemplateUpdate) (EntityTemplateOut, error) {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return EntityTemplateOut{}, err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Warn().Err(rollbackErr).Msg("failed to rollback transaction during template update")
			}
		}
	}()

	if err := assertEntityInGroup(ctx, tx.Entity, gid, data.DefaultLocationID); err != nil {
		return EntityTemplateOut{}, err
	}
	if data.DefaultTagIDs != nil {
		if err := assertTagsInGroup(ctx, tx.Tag, gid, *data.DefaultTagIDs); err != nil {
			return EntityTemplateOut{}, err
		}
	}

	// Verify template belongs to group
	template, err := tx.EntityTemplate.Query().
		Where(
			entitytemplate.ID(data.ID),
			entitytemplate.HasGroupWith(group.ID(gid)),
		).
		Only(ctx)

	if err != nil {
		return EntityTemplateOut{}, err
	}

	// Update template
	updateQ := template.Update().
		SetName(data.Name).
		SetDescription(data.Description).
		SetNotes(data.Notes).
		SetNillableDefaultQuantity(data.DefaultQuantity).
		SetDefaultInsured(data.DefaultInsured).
		SetNillableDefaultName(data.DefaultName).
		SetNillableDefaultDescription(data.DefaultDescription).
		SetNillableDefaultManufacturer(data.DefaultManufacturer).
		SetNillableDefaultModelNumber(data.DefaultModelNumber).
		SetDefaultLifetimeWarranty(data.DefaultLifetimeWarranty).
		SetNillableDefaultWarrantyDetails(data.DefaultWarrantyDetails).
		SetIncludeWarrantyFields(data.IncludeWarrantyFields).
		SetIncludePurchaseFields(data.IncludePurchaseFields).
		SetIncludeSoldFields(data.IncludeSoldFields)

	// Update location: set when provided (not uuid.Nil), otherwise clear
	if data.DefaultLocationID != uuid.Nil {
		updateQ.SetLocationID(data.DefaultLocationID)
	} else {
		updateQ.ClearLocation()
	}

	// Update default tag IDs (stored as JSON)
	if data.DefaultTagIDs != nil && len(*data.DefaultTagIDs) > 0 {
		updateQ.SetDefaultTagIds(*data.DefaultTagIDs)
	} else {
		updateQ.ClearDefaultTagIds()
	}

	_, err = updateQ.Save(ctx)
	if err != nil {
		return EntityTemplateOut{}, err
	}

	// Get existing fields
	existingFields, err := tx.TemplateField.Query().
		Where(templatefield.HasEntityTemplateWith(entitytemplate.ID(data.ID))).
		All(ctx)

	if err != nil {
		return EntityTemplateOut{}, err
	}

	// Track which fields are being updated
	updatedFieldIDs := make(map[uuid.UUID]bool)

	// Create or update fields
	for _, field := range data.Fields {
		if field.ID == nil || *field.ID == uuid.Nil {
			// Create new field
			fieldBuilder := tx.TemplateField.Create().
				SetEntityTemplateID(data.ID).
				SetType(templatefield.Type(field.Type)).
				SetName(field.Name).
				SetTextValue(field.TextValue).
				SetNumberValue(field.NumberValue).
				SetBooleanValue(field.BooleanValue)
			if !field.TimeValue.IsZero() {
				fieldBuilder.SetTimeValue(field.TimeValue)
			}
			_, err = fieldBuilder.Save(ctx)

			if err != nil {
				log.Err(err).Msg("failed to create template field")
				return EntityTemplateOut{}, err
			}
		} else {
			// Update existing field
			updatedFieldIDs[*field.ID] = true
			var updated int
			fieldUpdate := tx.TemplateField.Update().
				Where(
					templatefield.ID(*field.ID),
					templatefield.HasEntityTemplateWith(entitytemplate.ID(data.ID)),
				).
				SetType(templatefield.Type(field.Type)).
				SetName(field.Name).
				SetTextValue(field.TextValue).
				SetNumberValue(field.NumberValue).
				SetBooleanValue(field.BooleanValue)
			if !field.TimeValue.IsZero() {
				fieldUpdate.SetTimeValue(field.TimeValue)
			}
			updated, err = fieldUpdate.Save(ctx)

			if err != nil {
				log.Err(err).Msg("failed to update template field")
				return EntityTemplateOut{}, err
			}
			if updated != 1 {
				return EntityTemplateOut{}, &ent.NotFoundError{}
			}
		}
	}

	// Delete fields that are no longer present
	for _, field := range existingFields {
		if !updatedFieldIDs[field.ID] {
			err = tx.TemplateField.DeleteOne(field).Exec(ctx)
			if err != nil {
				log.Err(err).Msg("failed to delete template field")
				return EntityTemplateOut{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return EntityTemplateOut{}, err
	}
	committed = true

	r.publishMutationEvent(gid)
	return r.GetOne(ctx, gid, template.ID)
}

func (r *EntityTemplatesRepository) cleanupPhotoBlob(
	ctx context.Context,
	path string,
	mimeType string,
) {
	if r.attachments == nil || path == "" {
		return
	}
	r.attachments.cleanupUnreferencedBlobs(ctx, []attachmentBlobCandidate{{
		Path:     path,
		MimeType: mimeType,
	}})
}

// SetPhoto records a new template photo reference transactionally. The newly
// uploaded path is cleaned on any database failure, while a replaced path is
// considered for deletion only after the new reference commits. Global
// attachment/template checks preserve content-addressed blobs that are shared.
func (r *EntityTemplatesRepository) SetPhoto(ctx context.Context, gid uuid.UUID, id uuid.UUID, path string, mimeType string) error {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		r.cleanupPhotoBlob(ctx, path, mimeType)
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		_ = tx.Rollback()
		// Upload precedes this repository call. If the row update loses a
		// delete race or fails, remove the unreferenced upload after rollback.
		r.cleanupPhotoBlob(ctx, path, mimeType)
	}()

	template, err := tx.EntityTemplate.Query().
		Where(
			entitytemplate.ID(id),
			entitytemplate.HasGroupWith(group.ID(gid)),
		).
		Only(ctx)
	if err != nil {
		return err
	}
	oldPath, oldMimeType := template.PhotoPath, template.PhotoMimeType

	if _, err := template.Update().
		SetPhotoPath(path).
		SetPhotoMimeType(mimeType).
		Save(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	if oldPath != path {
		r.cleanupPhotoBlob(ctx, oldPath, oldMimeType)
	}
	r.publishMutationEvent(gid)
	return nil
}

// ClearPhoto commits removal of the database reference before attempting
// best-effort blob deletion. A rollback therefore always leaves the referenced
// blob intact.
func (r *EntityTemplatesRepository) ClearPhoto(ctx context.Context, gid uuid.UUID, id uuid.UUID) error {
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

	template, err := tx.EntityTemplate.Query().
		Where(
			entitytemplate.ID(id),
			entitytemplate.HasGroupWith(group.ID(gid)),
		).
		Only(ctx)
	if err != nil {
		return err
	}
	oldPath, oldMimeType := template.PhotoPath, template.PhotoMimeType

	if _, err := template.Update().
		ClearPhotoPath().
		ClearPhotoMimeType().
		Save(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	r.cleanupPhotoBlob(ctx, oldPath, oldMimeType)
	r.publishMutationEvent(gid)
	return nil
}

// Delete removes a template and its fields in one transaction, then globally
// reference-checks its former photo path before deleting the blob.
func (r *EntityTemplatesRepository) Delete(ctx context.Context, gid uuid.UUID, id uuid.UUID) error {
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

	template, err := tx.EntityTemplate.Query().
		Where(
			entitytemplate.ID(id),
			entitytemplate.HasGroupWith(group.ID(gid)),
		).
		Only(ctx)
	if err != nil {
		return err
	}
	oldPath, oldMimeType := template.PhotoPath, template.PhotoMimeType

	if err := tx.EntityTemplate.DeleteOneID(id).Exec(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	r.cleanupPhotoBlob(ctx, oldPath, oldMimeType)
	r.publishMutationEvent(gid)
	return nil
}
