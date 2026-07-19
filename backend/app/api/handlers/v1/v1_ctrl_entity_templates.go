package v1

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"net/url"

	"github.com/gen2brain/webp"
	"github.com/google/uuid"
	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/samber/lo"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"github.com/sysadminsmedia/homebox/backend/internal/web/adapters"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/memblob"
	_ "gocloud.dev/blob/s3blob"
	"gocloud.dev/gcerrors"
)

func validateTemplatePhotoContent(content []byte) (string, error) {
	if len(content) == 0 {
		return "", errors.New("photo is empty")
	}

	mimeType := http.DetectContentType(content)
	var err error
	switch mimeType {
	case "image/jpeg", "image/png":
		_, _, err = image.DecodeConfig(bytes.NewReader(content))
	case "image/webp":
		_, err = webp.DecodeConfig(bytes.NewReader(content))
	default:
		return "", errors.New("file is not a supported image (jpeg/png/webp)")
	}
	if err != nil {
		return "", errors.New("file is not a valid image")
	}
	return mimeType, nil
}

// HandleEntityTemplatesGetAll godoc
//
//	@Summary	Get All Entity Templates
//	@Tags		Entity Templates
//	@Produce	json
//	@Success	200	{array}	repo.EntityTemplateSummary
//	@Router		/v1/templates [GET]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatesGetAll() errchain.HandlerFunc {
	fn := func(r *http.Request) ([]repo.EntityTemplateSummary, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.EntityTemplates.GetAll(r.Context(), auth.GID)
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleEntityTemplatesGet godoc
//
//	@Summary	Get Entity Template
//	@Tags		Entity Templates
//	@Produce	json
//	@Param		id	path		string	true	"Template ID"
//	@Success	200	{object}	repo.EntityTemplateOut
//	@Router		/v1/templates/{id} [GET]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatesGet() errchain.HandlerFunc {
	fn := func(r *http.Request, ID uuid.UUID) (repo.EntityTemplateOut, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.EntityTemplates.GetOne(r.Context(), auth.GID, ID)
	}

	return adapters.CommandID("id", fn, http.StatusOK)
}

// HandleEntityTemplatesCreate godoc
//
//	@Summary	Create Entity Template
//	@Tags		Entity Templates
//	@Produce	json
//	@Param		payload	body		repo.EntityTemplateCreate	true	"Template Data"
//	@Success	201		{object}	repo.EntityTemplateOut
//	@Router		/v1/templates [POST]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatesCreate() errchain.HandlerFunc {
	fn := func(r *http.Request, body repo.EntityTemplateCreate) (repo.EntityTemplateOut, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.EntityTemplates.Create(r.Context(), auth.GID, body)
	}

	return adapters.Action(fn, http.StatusCreated)
}

// HandleEntityTemplatesUpdate godoc
//
//	@Summary	Update Entity Template
//	@Tags		Entity Templates
//	@Produce	json
//	@Param		id		path		string						true	"Template ID"
//	@Param		payload	body		repo.EntityTemplateUpdate	true	"Template Data"
//	@Success	200		{object}	repo.EntityTemplateOut
//	@Router		/v1/templates/{id} [PUT]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatesUpdate() errchain.HandlerFunc {
	fn := func(r *http.Request, ID uuid.UUID, body repo.EntityTemplateUpdate) (repo.EntityTemplateOut, error) {
		auth := services.NewContext(r.Context())
		body.ID = ID
		return ctrl.repo.EntityTemplates.Update(r.Context(), auth.GID, body)
	}

	return adapters.ActionID("id", fn, http.StatusOK)
}

// HandleEntityTemplatesDelete godoc
//
//	@Summary	Delete Entity Template
//	@Tags		Entity Templates
//	@Produce	json
//	@Param		id	path	string	true	"Template ID"
//	@Success	204
//	@Router		/v1/templates/{id} [DELETE]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatesDelete() errchain.HandlerFunc {
	fn := func(r *http.Request, ID uuid.UUID) (any, error) {
		auth := services.NewContext(r.Context())
		err := ctrl.repo.EntityTemplates.Delete(r.Context(), auth.GID, ID)
		return nil, err
	}

	return adapters.CommandID("id", fn, http.StatusNoContent)
}

type EntityTemplateCreateItemRequest struct {
	Name        string    `json:"name"        validate:"required,min=1,max=255"`
	Description string    `json:"description" validate:"max=1000"`
	ParentID    uuid.UUID `json:"parentId"    validate:"required"`
	// EntityTypeID is the entity type selected by the user. When set it takes
	// precedence; when empty the repository falls back to the group's default.
	EntityTypeID uuid.UUID   `json:"entityTypeId"`
	TagIDs       []uuid.UUID `json:"tagIds"`
	Quantity     *float64    `json:"quantity"`
}

// HandleEntityTemplatesCreateItem godoc
//
//	@Summary	Create Entity from Template
//	@Tags		Entity Templates
//	@Produce	json
//	@Param		id		path		string							true	"Template ID"
//	@Param		payload	body		EntityTemplateCreateItemRequest	true	"Entity Data"
//	@Success	201		{object}	repo.EntityOut
//	@Router		/v1/templates/{id}/create-item [POST]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatesCreateItem() errchain.HandlerFunc {
	fn := func(r *http.Request, templateID uuid.UUID, body EntityTemplateCreateItemRequest) (repo.EntityOut, error) {
		auth := services.NewContext(r.Context())

		template, err := ctrl.repo.EntityTemplates.GetOne(r.Context(), auth.GID, templateID)
		if err != nil {
			return repo.EntityOut{}, err
		}

		quantity := template.DefaultQuantity
		if body.Quantity != nil {
			quantity = *body.Quantity
		}

		// Build custom fields from template
		fields := lo.Map(template.Fields, func(f repo.TemplateField, _ int) repo.EntityFieldData {
			return repo.EntityFieldData{
				Type:         f.Type,
				Name:         f.Name,
				TextValue:    f.TextValue,
				NumberValue:  f.NumberValue,
				BooleanValue: f.BooleanValue,
				TimeValue:    f.TimeValue,
			}
		})

		// Create entity with all template data in a single transaction
		return ctrl.repo.Entities.CreateFromTemplate(r.Context(), auth.GID, repo.EntityCreateFromTemplate{
			Name:             body.Name,
			Description:      body.Description,
			Quantity:         quantity,
			ParentID:         body.ParentID,
			EntityTypeID:     body.EntityTypeID,
			TagIDs:           body.TagIDs,
			Insured:          template.DefaultInsured,
			Manufacturer:     template.DefaultManufacturer,
			ModelNumber:      template.DefaultModelNumber,
			LifetimeWarranty: template.DefaultLifetimeWarranty,
			WarrantyDetails:  template.DefaultWarrantyDetails,
			Fields:           fields,
			PhotoPath:        template.PhotoPath,
			PhotoMimeType:    template.PhotoMimeType,
		})
	}

	return adapters.ActionID("id", fn, http.StatusCreated)
}

type EntityTemplateBatchCreateRequest struct {
	Count        int         `json:"count"        validate:"required,min=1,max=100"`
	NamePrefix   string      `json:"namePrefix"   validate:"omitempty,max=240"`
	StartNumber  int         `json:"startNumber"  validate:"omitempty,min=1"`
	ParentID     uuid.UUID   `json:"parentId"`
	EntityTypeID uuid.UUID   `json:"entityTypeId"`
	TagIDs       []uuid.UUID `json:"tagIds"`
}

// HandleEntityTemplatesBatchCreate godoc
//
//	@Summary	Batch Create Entities from Template
//	@Tags		Entity Templates
//	@Produce	json
//	@Param		id		path		string								true	"Template ID"
//	@Param		payload	body		EntityTemplateBatchCreateRequest	true	"Batch options"
//	@Success	201		{object}	[]repo.EntityOut
//	@Failure	400		{object}	validate.ErrorResponse
//	@Router		/v1/templates/{id}/batch-create [POST]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatesBatchCreate() errchain.HandlerFunc {
	fn := func(r *http.Request, templateID uuid.UUID, body EntityTemplateBatchCreateRequest) ([]repo.EntityOut, error) {
		auth := services.NewContext(r.Context())

		template, err := ctrl.repo.EntityTemplates.GetOne(r.Context(), auth.GID, templateID)
		if err != nil {
			return nil, err
		}

		prefix := body.NamePrefix
		if prefix == "" {
			prefix = template.DefaultName
		}
		if prefix == "" {
			prefix = template.Name
		}

		// Build custom fields from template
		fields := lo.Map(template.Fields, func(f repo.TemplateField, _ int) repo.EntityFieldData {
			return repo.EntityFieldData{
				Type:         f.Type,
				Name:         f.Name,
				TextValue:    f.TextValue,
				NumberValue:  f.NumberValue,
				BooleanValue: f.BooleanValue,
				TimeValue:    f.TimeValue,
			}
		})

		return ctrl.repo.Entities.CreateFromTemplateBatch(r.Context(), auth.GID, repo.EntityBatchCreateFromTemplate{
			Template: repo.EntityCreateFromTemplate{
				Quantity:         template.DefaultQuantity,
				ParentID:         body.ParentID,
				EntityTypeID:     body.EntityTypeID,
				TagIDs:           body.TagIDs,
				Insured:          template.DefaultInsured,
				Manufacturer:     template.DefaultManufacturer,
				ModelNumber:      template.DefaultModelNumber,
				LifetimeWarranty: template.DefaultLifetimeWarranty,
				WarrantyDetails:  template.DefaultWarrantyDetails,
				Fields:           fields,
				PhotoPath:        template.PhotoPath,
				PhotoMimeType:    template.PhotoMimeType,
			},
			Count:       body.Count,
			NamePrefix:  prefix,
			StartNumber: body.StartNumber,
		})
	}

	return adapters.ActionID("id", fn, http.StatusCreated)
}

// HandleEntityTemplatePhotoUpload godoc
//
//	@Summary	Upload Template Photo
//	@Tags		Entity Templates
//	@Produce	json
//	@Param		id		path		string	true	"Template ID"
//	@Param		file	formData	file	true	"Photo file"
//	@Success	201		{object}	repo.EntityTemplateOut
//	@Failure	400		{object}	validate.ErrorResponse
//	@Router		/v1/templates/{id}/photo [POST]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatePhotoUpload() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		err := r.ParseMultipartForm(ctrl.maxUploadSize << 20)
		if err != nil {
			return multipartParseRequestError(err)
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			return multipartFileRequestError(err, "photo")
		}
		defer func() { _ = file.Close() }()

		maxBytes := ctrl.maxUploadSize << 20
		content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
		if err != nil {
			return multipartContentReadError(err, "photo")
		}
		if int64(len(content)) > maxBytes {
			return validate.NewRequestError(errors.New("photo exceeds upload size limit"), http.StatusRequestEntityTooLarge)
		}
		if _, err := validateTemplatePhotoContent(content); err != nil {
			return validate.NewRequestError(err, http.StatusBadRequest)
		}

		id, err := ctrl.routeID(r)
		if err != nil {
			return err
		}

		auth := services.NewContext(r.Context())

		// Ensure the template exists in this group before uploading the blob.
		if _, err := ctrl.repo.EntityTemplates.GetOne(r.Context(), auth.GID, id); err != nil {
			return err
		}

		res, err := ctrl.repo.Attachments.UploadFileByGroupID(r.Context(), auth.GID, repo.ItemCreateAttachment{
			Title:   sanitizeAttachmentName(header.Filename),
			Content: bytes.NewReader(content),
		})
		if err != nil {
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}

		if err := ctrl.repo.EntityTemplates.SetPhoto(r.Context(), auth.GID, id, res.Path, res.ContentType); err != nil {
			return err
		}

		out, err := ctrl.repo.EntityTemplates.GetOne(r.Context(), auth.GID, id)
		if err != nil {
			return err
		}
		return server.JSON(w, http.StatusCreated, out)
	}
}

// HandleEntityTemplatePhotoGet godoc
//
//	@Summary	Get Template Photo
//	@Tags		Entity Templates
//	@Produce	octet-stream
//	@Param		id	path	string	true	"Template ID"
//	@Success	200
//	@Failure	400	{object}	validate.ErrorResponse
//	@Failure	404	{object}	validate.ErrorResponse
//	@Router		/v1/templates/{id}/photo [GET]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatePhotoGet() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := ctrl.routeID(r)
		if err != nil {
			return err
		}
		auth := services.NewContext(r.Context())

		tmpl, err := ctrl.repo.EntityTemplates.GetOne(r.Context(), auth.GID, id)
		if err != nil {
			return err
		}
		if tmpl.PhotoPath == "" {
			return validate.NewRequestError(errors.New("template has no photo"), http.StatusNotFound)
		}

		bucket, err := blob.OpenBucket(r.Context(), ctrl.repo.Attachments.GetConnString())
		if err != nil {
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}
		defer func() { _ = bucket.Close() }()

		fileReader, err := bucket.NewReader(r.Context(), ctrl.repo.Attachments.GetFullPath(tmpl.PhotoPath), nil)
		if err != nil {
			if gcerrors.Code(err) == gcerrors.NotFound {
				return validate.NewRequestError(err, http.StatusNotFound)
			}
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}
		defer func() { _ = fileReader.Close() }()

		filename := "photo"
		if exts, extErr := mime.ExtensionsByType(tmpl.PhotoMimeType); extErr == nil && len(exts) > 0 {
			filename += exts[0]
		}

		disposition := "attachment"
		if isSafeInlineType(tmpl.PhotoMimeType) {
			disposition = "inline"
		}
		disposition += "; filename*=UTF-8''" + url.QueryEscape(filename)
		w.Header().Set("Content-Disposition", disposition)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Download-Options", "noopen")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox;")

		http.ServeContent(w, r, filename, tmpl.UpdatedAt, fileReader)
		return nil
	}
}

// HandleEntityTemplatePhotoDelete godoc
//
//	@Summary	Delete Template Photo
//	@Tags		Entity Templates
//	@Produce	json
//	@Param		id	path	string	true	"Template ID"
//	@Success	204
//	@Failure	400	{object}	validate.ErrorResponse
//	@Router		/v1/templates/{id}/photo [DELETE]
//	@Security	Bearer
func (ctrl *V1Controller) HandleEntityTemplatePhotoDelete() errchain.HandlerFunc {
	fn := func(r *http.Request, ID uuid.UUID) (any, error) {
		auth := services.NewContext(r.Context())
		return nil, ctrl.repo.EntityTemplates.ClearPhoto(r.Context(), auth.GID, ID)
	}
	return adapters.CommandID("id", fn, http.StatusNoContent)
}
