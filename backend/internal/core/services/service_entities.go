package services

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services/reporting"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func entityServiceTracer() trace.Tracer {
	return otel.Tracer("service")
}

func recordServiceSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

var (
	ErrNotFound     = errors.New("not found")
	ErrFileNotFound = errors.New("file not found")
)

type EntityService struct {
	repo *repo.AllRepos

	filepath string

	autoIncrementAssetID bool
}

func (svc *EntityService) Create(ctx Context, entity repo.EntityCreate) (repo.EntityOut, error) {
	spanCtx, span := entityServiceTracer().Start(ctx.Context, "service.EntityService.Create",
		trace.WithAttributes(
			attribute.String("group.id", ctx.GID.String()),
			attribute.String("entity.name", entity.Name),
			attribute.Bool("svc.auto_increment_asset_id", svc.autoIncrementAssetID),
		))
	defer span.End()
	ctx.Context = spanCtx

	if svc.autoIncrementAssetID {
		highest, err := svc.repo.Entities.GetHighestAssetID(ctx, ctx.GID)
		if err != nil {
			recordServiceSpanError(span, err)
			return repo.EntityOut{}, err
		}

		entity.AssetID = highest + 1
		span.SetAttributes(attribute.Int64("entity.asset_id", int64(entity.AssetID)))
	}

	out, err := svc.repo.Entities.Create(ctx, ctx.GID, entity)
	if err != nil {
		recordServiceSpanError(span, err)
		return out, err
	}
	span.SetAttributes(attribute.String("entity.id", out.ID.String()))
	return out, nil
}

func (svc *EntityService) Duplicate(ctx Context, gid, id uuid.UUID, options repo.DuplicateOptions) (repo.EntityOut, error) {
	spanCtx, span := entityServiceTracer().Start(ctx.Context, "service.EntityService.Duplicate",
		trace.WithAttributes(
			attribute.String("group.id", gid.String()),
			attribute.String("entity.source_id", id.String()),
			attribute.Bool("options.copy_maintenance", options.CopyMaintenance),
			attribute.Bool("options.copy_attachments", options.CopyAttachments),
			attribute.Bool("options.copy_custom_fields", options.CopyCustomFields),
		))
	defer span.End()
	ctx.Context = spanCtx

	out, err := svc.repo.Entities.Duplicate(ctx, gid, id, options)
	if err != nil {
		recordServiceSpanError(span, err)
		return out, err
	}
	span.SetAttributes(attribute.String("entity.id", out.ID.String()))
	return out, nil
}

func (svc *EntityService) EnsureAssetID(ctx context.Context, gid uuid.UUID) (int, error) {
	ctx, span := entityServiceTracer().Start(ctx, "service.EntityService.EnsureAssetID",
		trace.WithAttributes(attribute.String("group.id", gid.String())))
	defer span.End()

	items, err := svc.repo.Entities.GetAllZeroAssetID(ctx, gid)
	if err != nil {
		recordServiceSpanError(span, err)
		return 0, err
	}
	span.SetAttributes(attribute.Int("entities.zero_asset_id.count", len(items)))

	highest, err := svc.repo.Entities.GetHighestAssetID(ctx, gid)
	if err != nil {
		recordServiceSpanError(span, err)
		return 0, err
	}

	_, updateSpan := entityServiceTracer().Start(ctx, "service.EntityService.EnsureAssetID.update",
		trace.WithAttributes(attribute.Int("entities.count", len(items))))
	defer updateSpan.End()

	finished := 0
	for _, item := range items {
		highest++

		err = svc.repo.Entities.SetAssetID(ctx, gid, item.ID, highest)
		if err != nil {
			recordServiceSpanError(updateSpan, err)
			recordServiceSpanError(span, err)
			return 0, err
		}

		finished++
	}

	updateSpan.SetAttributes(attribute.Int("entities.updated.count", finished))
	span.SetAttributes(attribute.Int("entities.updated.count", finished))
	return finished, nil
}

func (svc *EntityService) EnsureImportRef(ctx context.Context, gid uuid.UUID) (int, error) {
	ctx, span := entityServiceTracer().Start(ctx, "service.EntityService.EnsureImportRef",
		trace.WithAttributes(attribute.String("group.id", gid.String())))
	defer span.End()

	ids, err := svc.repo.Entities.GetAllZeroImportRef(ctx, gid)
	if err != nil {
		recordServiceSpanError(span, err)
		return 0, err
	}
	span.SetAttributes(attribute.Int("entities.zero_import_ref.count", len(ids)))

	_, patchSpan := entityServiceTracer().Start(ctx, "service.EntityService.EnsureImportRef.patch",
		trace.WithAttributes(attribute.Int("entities.count", len(ids))))
	defer patchSpan.End()

	finished := 0
	for _, entityID := range ids {
		ref := uuid.New().String()[0:8]
		err = svc.repo.Entities.Patch(ctx, gid, entityID, repo.EntityPatch{ImportRef: &ref})
		if err != nil {
			recordServiceSpanError(patchSpan, err)
			recordServiceSpanError(span, err)
			return 0, err
		}

		finished++
	}

	patchSpan.SetAttributes(attribute.Int("entities.patched.count", finished))
	span.SetAttributes(attribute.Int("entities.patched.count", finished))
	return finished, nil
}

// CsvImport imports entities from a CSV file using the standard defined format.
//
// CsvImport applies the following rules/operations
//
//  1. If the entity does not exist, it is created.
//  2. If the entity has an ImportRef and it exists it is updated.
//  3. Locations and Tags are created if they do not exist.
//
// All writes are committed atomically. A failure in any row or in the final
// parent-reference resolution rolls back the complete import.
func (svc *EntityService) CsvImport(ctx context.Context, gid uuid.UUID, data io.Reader) (int, error) {
	ctx, span := entityServiceTracer().Start(ctx, "service.EntityService.CsvImport",
		trace.WithAttributes(attribute.String("group.id", gid.String())))
	defer span.End()

	_, readSpan := entityServiceTracer().Start(ctx, "service.EntityService.CsvImport.readCsv")
	sheet := reporting.IOSheet{}

	err := sheet.Read(data)
	if err != nil {
		recordServiceSpanError(readSpan, err)
		readSpan.End()
		recordServiceSpanError(span, err)
		return 0, err
	}
	readSpan.SetAttributes(attribute.Int("rows.count", len(sheet.Rows)))
	readSpan.End()
	span.SetAttributes(attribute.Int("rows.count", len(sheet.Rows)))

	rows := lo.Map(sheet.Rows, func(row reporting.ExportCSVRow, _ int) repo.CSVImportRow {
		fields := lo.Map(row.Fields, func(field reporting.ExportItemFields, _ int) repo.EntityFieldData {
			return repo.EntityFieldData{
				Name:      field.Name,
				Type:      "text",
				TextValue: field.Value,
			}
		})

		return repo.CSVImportRow{
			ImportRef:       row.ImportRef,
			ParentImportRef: row.ParentImportRef,
			Location:        append([]string(nil), row.Location...),
			TagNames:        append([]string(nil), row.TagStr...),
			Entity: repo.EntityUpdate{
				Name:        row.Name,
				Description: row.Description,
				AssetID:     row.AssetID,
				Insured:     row.Insured,
				Quantity:    row.Quantity,
				Archived:    row.Archived,

				PurchasePrice: row.PurchasePrice,
				PurchaseFrom:  row.PurchaseFrom,
				PurchaseDate:  row.PurchaseDate,

				Manufacturer: row.Manufacturer,
				ModelNumber:  row.ModelNumber,
				SerialNumber: row.SerialNumber,

				LifetimeWarranty: row.LifetimeWarranty,
				WarrantyExpires:  row.WarrantyExpires,
				WarrantyDetails:  row.WarrantyDetails,

				SoldTo:    row.SoldTo,
				SoldDate:  row.SoldDate,
				SoldPrice: row.SoldPrice,
				SoldNotes: row.SoldNotes,

				Notes:  row.Notes,
				Fields: fields,
			},
		}
	})

	importCtx, importSpan := entityServiceTracer().Start(ctx, "service.EntityService.CsvImport.transaction",
		trace.WithAttributes(attribute.Int("rows.count", len(sheet.Rows))))
	defer importSpan.End()

	finished, err := svc.repo.Entities.ImportCSV(
		importCtx,
		gid,
		rows,
		svc.autoIncrementAssetID,
	)
	if err != nil {
		recordServiceSpanError(importSpan, err)
		recordServiceSpanError(span, err)
		return 0, err
	}

	importSpan.SetAttributes(attribute.Int("rows.imported.count", finished))
	span.SetAttributes(attribute.Int("rows.imported.count", finished))
	return finished, nil
}

func (svc *EntityService) ExportCSV(ctx context.Context, gid uuid.UUID, hbURL string) ([][]string, error) {
	ctx, span := entityServiceTracer().Start(ctx, "service.EntityService.ExportCSV",
		trace.WithAttributes(attribute.String("group.id", gid.String())))
	defer span.End()

	loadCtx, loadSpan := entityServiceTracer().Start(ctx, "service.EntityService.ExportCSV.load")
	items, err := svc.repo.Entities.GetAll(loadCtx, gid)
	if err != nil {
		recordServiceSpanError(loadSpan, err)
		loadSpan.End()
		recordServiceSpanError(span, err)
		return nil, err
	}
	loadSpan.SetAttributes(attribute.Int("entities.count", len(items)))
	loadSpan.End()
	span.SetAttributes(attribute.Int("entities.count", len(items)))

	readCtx, readSpan := entityServiceTracer().Start(ctx, "service.EntityService.ExportCSV.readItems")
	sheet := reporting.IOSheet{}
	err = sheet.ReadItems(readCtx, items, gid, svc.repo, hbURL)
	if err != nil {
		recordServiceSpanError(readSpan, err)
		readSpan.End()
		recordServiceSpanError(span, err)
		return nil, err
	}
	readSpan.End()

	_, csvSpan := entityServiceTracer().Start(ctx, "service.EntityService.ExportCSV.encode")
	defer csvSpan.End()
	rows, err := sheet.CSV()
	if err != nil {
		recordServiceSpanError(csvSpan, err)
		recordServiceSpanError(span, err)
		return nil, err
	}
	csvSpan.SetAttributes(attribute.Int("rows.count", len(rows)))
	return rows, nil
}

func (svc *EntityService) ExportBillOfMaterialsCSV(ctx context.Context, gid uuid.UUID) ([]byte, error) {
	ctx, span := entityServiceTracer().Start(ctx, "service.EntityService.ExportBillOfMaterialsCSV",
		trace.WithAttributes(attribute.String("group.id", gid.String())))
	defer span.End()

	loadCtx, loadSpan := entityServiceTracer().Start(ctx, "service.EntityService.ExportBillOfMaterialsCSV.load")
	items, err := svc.repo.Entities.GetAll(loadCtx, gid)
	if err != nil {
		recordServiceSpanError(loadSpan, err)
		loadSpan.End()
		recordServiceSpanError(span, err)
		return nil, err
	}
	loadSpan.SetAttributes(attribute.Int("entities.count", len(items)))
	loadSpan.End()
	span.SetAttributes(attribute.Int("entities.count", len(items)))

	_, encodeSpan := entityServiceTracer().Start(ctx, "service.EntityService.ExportBillOfMaterialsCSV.encode")
	defer encodeSpan.End()
	out, err := reporting.BillOfMaterialsCSV(items)
	if err != nil {
		recordServiceSpanError(encodeSpan, err)
		recordServiceSpanError(span, err)
		return nil, err
	}
	encodeSpan.SetAttributes(attribute.Int("bytes.size", len(out)))
	return out, nil
}
