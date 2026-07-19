package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/attachment"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var ErrInvalidExternalAttachmentURL = errors.New("external attachment URL must be an absolute http/https URL without credentials")

func canonicalExternalHTTPURL(raw string) (string, error) {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil ||
		(!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) ||
		u.Host == "" || u.Hostname() == "" || u.User != nil {
		return "", ErrInvalidExternalAttachmentURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	return u.String(), nil
}

func redactExternalURLForTrace(raw string) string {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return ""
	}
	if u.Host == "" {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	return u.String()
}

func (svc *EntityService) AttachmentPath(
	ctx context.Context,
	gid, entityID, attachmentID uuid.UUID,
) (*ent.Attachment, error) {
	ctx, span := entityServiceTracer().Start(ctx, "service.EntityService.AttachmentPath",
		trace.WithAttributes(
			attribute.String("group.id", gid.String()),
			attribute.String("attachment.id", attachmentID.String()),
		))
	defer span.End()

	attachment, err := svc.repo.Attachments.GetForEntity(ctx, gid, entityID, attachmentID)
	if err != nil {
		recordServiceSpanError(span, err)
		return nil, err
	}

	return attachment, nil
}

func (svc *EntityService) AttachmentUpdate(ctx Context, gid uuid.UUID, entityID uuid.UUID, data *repo.ItemAttachmentUpdate) (repo.EntityOut, error) {
	spanCtx, span := entityServiceTracer().Start(ctx.Context, "service.EntityService.AttachmentUpdate",
		trace.WithAttributes(
			attribute.String("group.id", gid.String()),
			attribute.String("entity.id", entityID.String()),
			attribute.String("attachment.id", data.ID.String()),
			attribute.String("attachment.type", data.Type),
			attribute.String("attachment.title", data.Title),
			attribute.Bool("attachment.primary", data.Primary),
		))
	defer span.End()
	ctx.Context = spanCtx

	updateCtx, updateSpan := entityServiceTracer().Start(spanCtx, "service.EntityService.AttachmentUpdate.update")
	_, err := svc.repo.Attachments.Update(updateCtx, gid, entityID, data.ID, data)
	if err != nil {
		recordServiceSpanError(updateSpan, err)
		updateSpan.End()
		recordServiceSpanError(span, err)
		return repo.EntityOut{}, err
	}
	updateSpan.End()

	out, err := svc.repo.Entities.GetOneByGroup(ctx, ctx.GID, entityID)
	if err != nil {
		recordServiceSpanError(span, err)
	}
	return out, err
}

// AttachmentAdd adds an attachment to an entity by creating an entry in the Documents table and linking it to the Attachment
// Table and Entities table. The file provided via the reader is stored on the file system based on the provided
// relative path during construction of the service.
func (svc *EntityService) AttachmentAdd(ctx Context, entityID uuid.UUID, filename string, attachmentType attachment.Type, primary bool, file io.Reader) (repo.EntityOut, error) {
	spanCtx, span := entityServiceTracer().Start(ctx.Context, "service.EntityService.AttachmentAdd",
		trace.WithAttributes(
			attribute.String("group.id", ctx.GID.String()),
			attribute.String("entity.id", entityID.String()),
			attribute.String("attachment.filename", filename),
			attribute.String("attachment.type", attachmentType.String()),
			attribute.Bool("attachment.primary", primary),
		))
	defer span.End()
	ctx.Context = spanCtx

	verifyCtx, verifySpan := entityServiceTracer().Start(spanCtx, "service.EntityService.AttachmentAdd.verifyEntity")
	_, err := svc.repo.Entities.GetOneByGroup(verifyCtx, ctx.GID, entityID)
	if err != nil {
		recordServiceSpanError(verifySpan, err)
		verifySpan.End()
		recordServiceSpanError(span, err)
		return repo.EntityOut{}, err
	}
	verifySpan.End()

	createCtx, createSpan := entityServiceTracer().Start(spanCtx, "service.EntityService.AttachmentAdd.create")
	_, err = svc.repo.Attachments.Create(createCtx, entityID, repo.ItemCreateAttachment{Title: filename, Content: file}, attachmentType, primary)
	if err != nil {
		recordServiceSpanError(createSpan, err)
		createSpan.End()
		recordServiceSpanError(span, err)
		log.Err(err).Msg("failed to create attachment")
		return repo.EntityOut{}, err
	}
	createSpan.End()

	out, err := svc.repo.Entities.GetOneByGroup(ctx, ctx.GID, entityID)
	if err != nil {
		recordServiceSpanError(span, err)
	}
	return out, err
}

func (svc *EntityService) AttachmentAddExternalLink(ctx Context, entityID uuid.UUID, sourceType, externalID, title string, attType attachment.Type) (repo.EntityOut, error) {
	sourceType = strings.TrimSpace(sourceType)
	spanCtx, span := entityServiceTracer().Start(ctx.Context, "service.EntityService.AttachmentAddExternalLink",
		trace.WithAttributes(
			attribute.String("group.id", ctx.GID.String()),
			attribute.String("entity.id", entityID.String()),
			attribute.String("integration.source_type", sourceType),
			attribute.String("integration.external_id", redactExternalURLForTrace(externalID)),
		))
	defer span.End()
	ctx.Context = spanCtx

	mimeType, ok := repo.MimeTypeForSourceType(sourceType)
	if !ok {
		err := fmt.Errorf("unknown source_type %q", sourceType)
		recordServiceSpanError(span, err)
		return repo.EntityOut{}, err
	}
	if mimeType == repo.MimeTypeLinkURL {
		canonicalID, err := canonicalExternalHTTPURL(externalID)
		if err != nil {
			recordServiceSpanError(span, err)
			return repo.EntityOut{}, err
		}
		externalID = canonicalID
	}

	verifyCtx, verifySpan := entityServiceTracer().Start(spanCtx, "service.EntityService.AttachmentAddExternalLink.verifyEntity")
	_, err := svc.repo.Entities.GetOneByGroup(verifyCtx, ctx.GID, entityID)
	if err != nil {
		recordServiceSpanError(verifySpan, err)
		verifySpan.End()
		recordServiceSpanError(span, err)
		return repo.EntityOut{}, err
	}
	verifySpan.End()

	createCtx, createSpan := entityServiceTracer().Start(spanCtx, "service.EntityService.AttachmentAddExternalLink.create")
	_, err = svc.repo.Attachments.CreateExternalLink(createCtx, entityID, externalID, title, mimeType, attType)
	if err != nil {
		recordServiceSpanError(createSpan, err)
		createSpan.End()
		recordServiceSpanError(span, err)
		log.Err(err).Msg("failed to create external link attachment")
		return repo.EntityOut{}, err
	}
	createSpan.End()

	out, err := svc.repo.Entities.GetOneByGroup(ctx, ctx.GID, entityID)
	if err != nil {
		recordServiceSpanError(span, err)
	}
	return out, err
}

func (svc *EntityService) AttachmentDelete(
	ctx context.Context,
	gid, entityID, attachmentID uuid.UUID,
) error {
	ctx, span := entityServiceTracer().Start(ctx, "service.EntityService.AttachmentDelete",
		trace.WithAttributes(
			attribute.String("group.id", gid.String()),
			attribute.String("attachment.id", attachmentID.String()),
		))
	defer span.End()

	err := svc.repo.Attachments.DeleteForEntity(ctx, gid, entityID, attachmentID)
	if err != nil {
		recordServiceSpanError(span, err)
		return err
	}

	return nil
}
