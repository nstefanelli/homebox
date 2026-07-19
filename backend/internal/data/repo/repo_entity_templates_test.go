package repo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func templateFactory() EntityTemplateCreate {
	return EntityTemplateCreate{
		Name:                    fk.Str(10),
		Description:             fk.Str(100),
		Notes:                   fk.Str(50),
		DefaultQuantity:         lo.ToPtr(1.0),
		DefaultInsured:          false,
		DefaultName:             lo.ToPtr(fk.Str(20)),
		DefaultDescription:      lo.ToPtr(fk.Str(50)),
		DefaultManufacturer:     lo.ToPtr(fk.Str(15)),
		DefaultModelNumber:      lo.ToPtr(fk.Str(10)),
		DefaultLifetimeWarranty: false,
		DefaultWarrantyDetails:  lo.ToPtr(""),
		IncludeWarrantyFields:   false,
		IncludePurchaseFields:   false,
		IncludeSoldFields:       false,
		Fields:                  []TemplateField{},
	}
}

func TestEntityTemplatesRepository_Create(t *testing.T) {
	data := templateFactory()

	result, err := tRepos.EntityTemplates.Create(context.Background(), tGroup.ID, data)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.ID)
	assert.Equal(t, data.Name, result.Name)
	assert.Equal(t, data.Description, result.Description)

	// Cleanup
	err = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, result.ID)
	require.NoError(t, err)
}

func TestEntityTemplatesRepository_GetAll(t *testing.T) {
	data := templateFactory()

	created, err := tRepos.EntityTemplates.Create(context.Background(), tGroup.ID, data)
	require.NoError(t, err)

	results, err := tRepos.EntityTemplates.GetAll(context.Background(), tGroup.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)

	found := false
	for _, r := range results {
		if r.ID == created.ID {
			found = true
			assert.Equal(t, data.Name, r.Name)
		}
	}
	assert.True(t, found)

	// Cleanup
	err = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, created.ID)
	require.NoError(t, err)
}

func TestEntityTemplatesRepository_GetOne(t *testing.T) {
	data := templateFactory()

	created, err := tRepos.EntityTemplates.Create(context.Background(), tGroup.ID, data)
	require.NoError(t, err)

	result, err := tRepos.EntityTemplates.GetOne(context.Background(), tGroup.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, result.ID)
	assert.Equal(t, data.Name, result.Name)
	assert.Equal(t, data.Description, result.Description)

	// Cleanup
	err = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, created.ID)
	require.NoError(t, err)
}

func TestEntityTemplatesRepository_Update(t *testing.T) {
	data := templateFactory()

	created, err := tRepos.EntityTemplates.Create(context.Background(), tGroup.ID, data)
	require.NoError(t, err)

	updateData := EntityTemplateUpdate{
		ID:          created.ID,
		Name:        fk.Str(10),
		Description: fk.Str(100),
		Notes:       fk.Str(50),
	}

	result, err := tRepos.EntityTemplates.Update(context.Background(), tGroup.ID, updateData)
	require.NoError(t, err)
	assert.Equal(t, created.ID, result.ID)
	assert.Equal(t, updateData.Name, result.Name)
	assert.Equal(t, updateData.Description, result.Description)

	// Cleanup
	err = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, created.ID)
	require.NoError(t, err)
}

func TestEntityTemplatesRepository_TypedFieldValuesRoundTrip(t *testing.T) {
	ctx := context.Background()
	firstTime := time.Date(2025, time.March, 4, 5, 6, 7, 0, time.UTC)
	data := templateFactory()
	data.Fields = []TemplateField{
		{Type: "text", Name: "text", TextValue: "alpha"},
		{Type: testFieldTypeNumber, Name: "number", NumberValue: 42},
		{Type: testFieldTypeBoolean, Name: "boolean", BooleanValue: true},
		{Type: "time", Name: "time", TimeValue: firstTime},
	}

	created, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, data)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, created.ID)
	})
	require.Len(t, created.Fields, 4)

	byName := lo.SliceToMap(created.Fields, func(field TemplateField) (string, TemplateField) {
		return field.Name, field
	})
	assert.Equal(t, "alpha", byName["text"].TextValue)
	assert.Equal(t, 42, byName["number"].NumberValue)
	assert.True(t, byName["boolean"].BooleanValue)
	assert.WithinDuration(t, firstTime, byName["time"].TimeValue, time.Second)

	secondTime := time.Date(2026, time.April, 8, 9, 10, 11, 0, time.UTC)
	fields := created.Fields
	for i := range fields {
		switch fields[i].Name {
		case "text":
			fields[i].TextValue = "omega"
		case "number":
			fields[i].NumberValue = 84
		case "boolean":
			fields[i].BooleanValue = false
		case "time":
			fields[i].TimeValue = secondTime
		}
	}

	updated, err := tRepos.EntityTemplates.Update(ctx, tGroup.ID, EntityTemplateUpdate{
		ID:          created.ID,
		Name:        data.Name,
		Description: data.Description,
		Notes:       data.Notes,
		Fields:      fields,
	})
	require.NoError(t, err)
	require.Len(t, updated.Fields, 4)

	byName = lo.SliceToMap(updated.Fields, func(field TemplateField) (string, TemplateField) {
		return field.Name, field
	})
	assert.Equal(t, "omega", byName["text"].TextValue)
	assert.Equal(t, 84, byName["number"].NumberValue)
	assert.False(t, byName["boolean"].BooleanValue)
	assert.WithinDuration(t, secondTime, byName["time"].TimeValue, time.Second)
}

func TestEntityTemplatesRepository_Create_RollsBackOnFieldFailure(t *testing.T) {
	ctx := context.Background()
	before, err := tRepos.EntityTemplates.GetAll(ctx, tGroup.ID)
	require.NoError(t, err)

	data := templateFactory()
	data.Name = "template-create-must-roll-back"
	data.Fields = []TemplateField{{
		Type: "not-a-valid-field-type",
		Name: "invalid",
	}}

	_, err = tRepos.EntityTemplates.Create(ctx, tGroup.ID, data)
	require.Error(t, err)

	after, err := tRepos.EntityTemplates.GetAll(ctx, tGroup.ID)
	require.NoError(t, err)
	assert.Len(t, after, len(before), "template row must roll back when a field fails")
}

func TestEntityTemplatesRepository_Update_RollsBackOnFieldFailure(t *testing.T) {
	ctx := context.Background()
	created, err := tRepos.EntityTemplates.Create(ctx, tGroup.ID, templateFactory())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, created.ID)
	})

	_, err = tRepos.EntityTemplates.Update(ctx, tGroup.ID, EntityTemplateUpdate{
		ID:   created.ID,
		Name: "template-update-must-roll-back",
		Fields: []TemplateField{{
			Type: "not-a-valid-field-type",
			Name: "invalid",
		}},
	})
	require.Error(t, err)

	got, err := tRepos.EntityTemplates.GetOne(ctx, tGroup.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Name, got.Name)
	assert.Empty(t, got.Fields)
}

func TestEntityTemplatesRepository_Delete(t *testing.T) {
	data := templateFactory()

	created, err := tRepos.EntityTemplates.Create(context.Background(), tGroup.ID, data)
	require.NoError(t, err)

	err = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, created.ID)
	require.NoError(t, err)

	_, err = tRepos.EntityTemplates.GetOne(context.Background(), tGroup.ID, created.ID)
	require.Error(t, err)
}

func TestEntityTemplatesRepository_SetAndClearPhoto(t *testing.T) {
	created, err := tRepos.EntityTemplates.Create(context.Background(), tGroup.ID, templateFactory())
	require.NoError(t, err)
	defer func() { _ = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, created.ID) }()

	err = tRepos.EntityTemplates.SetPhoto(context.Background(), tGroup.ID, created.ID, "grp/documents/abc123", "image/jpeg")
	require.NoError(t, err)

	got, err := tRepos.EntityTemplates.GetOne(context.Background(), tGroup.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "grp/documents/abc123", got.PhotoPath)
	assert.Equal(t, "image/jpeg", got.PhotoMimeType)

	err = tRepos.EntityTemplates.ClearPhoto(context.Background(), tGroup.ID, created.ID)
	require.NoError(t, err)

	got, err = tRepos.EntityTemplates.GetOne(context.Background(), tGroup.ID, created.ID)
	require.NoError(t, err)
	assert.Empty(t, got.PhotoPath)
}

// TestEntityRepository_Delete_PreservesBlobReferencedByTemplatePhoto is a
// regression test: deleting the last entity attachment row for a
// content-addressed blob must not delete the underlying file when an
// entity_templates.photo_path row still references that same path.
func TestEntityRepository_Delete_PreservesBlobReferencedByTemplatePhoto(t *testing.T) {
	itemET := useItemEntityType(t)

	uploadRes, err := tRepos.Attachments.UploadFileByGroupID(context.Background(), tGroup.ID, ItemCreateAttachment{
		Title:   "template-photo.jpg",
		Content: bytes.NewReader([]byte("fake-template-photo-bytes")),
	})
	require.NoError(t, err)

	template, err := tRepos.EntityTemplates.Create(context.Background(), tGroup.ID, templateFactory())
	require.NoError(t, err)
	defer func() { _ = tRepos.EntityTemplates.Delete(context.Background(), tGroup.ID, template.ID) }()

	err = tRepos.EntityTemplates.SetPhoto(context.Background(), tGroup.ID, template.ID, uploadRes.Path, uploadRes.ContentType)
	require.NoError(t, err)

	out, err := tRepos.Entities.CreateFromTemplate(context.Background(), tGroup.ID, EntityCreateFromTemplate{
		Name:          fk.Str(10),
		Description:   fk.Str(20),
		Quantity:      1,
		EntityTypeID:  itemET.ID,
		PhotoPath:     uploadRes.Path,
		PhotoMimeType: uploadRes.ContentType,
	})
	require.NoError(t, err)

	// Sanity check the photo threaded through to the new entity's attachment.
	require.Len(t, out.Attachments, 1)
	assert.Equal(t, uploadRes.Path, out.Attachments[0].Path)

	// Delete the entity. This removes the last attachment row for this blob
	// path, but the template still references it via photo_path.
	err = tRepos.Entities.Delete(context.Background(), out.ID)
	require.NoError(t, err)

	// The blob must still be on disk because entity_templates.photo_path
	// still points at it.
	onDiskPath := filepath.Join(os.TempDir(), uploadRes.Path)
	_, statErr := os.Stat(onDiskPath)
	require.NoError(t, statErr, "expected blob at %s to still exist because template %s still references it", onDiskPath, template.ID)
}

func TestAttachmentRepo_UploadFileByGroupID(t *testing.T) {
	res, err := tRepos.Attachments.UploadFileByGroupID(context.Background(), tGroup.ID, ItemCreateAttachment{
		Title:   "tote.jpg",
		Content: bytes.NewReader([]byte("fake-image-bytes")),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Path)
	assert.NotEmpty(t, res.ContentType)
}
