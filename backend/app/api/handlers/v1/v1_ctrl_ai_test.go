package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/pkgs/ai"
)

type stubProvider struct {
	res ai.AnalyzeResult
	err error
}

func (s stubProvider) Analyze(_ context.Context, _ []byte, _ string) (ai.AnalyzeResult, error) {
	return s.res, s.err
}

func (s stubProvider) AnalyzeContents(_ context.Context, _ []byte, _ string) ([]ai.AnalyzeResult, error) {
	return []ai.AnalyzeResult{s.res}, s.err
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	return buf.Bytes()
}

func multipartPhotoRequest(t *testing.T, field string, content []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile(field, "item.png")
	require.NoError(t, err)
	_, err = fw.Write(content)
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/v1/actions/analyze-photo", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func testAIController() *V1Controller {
	return NewControllerV1(nil, nil, nil, &config.Config{}, WithMaxUploadSize(10))
}

func TestHandleAnalyzePhoto_Success(t *testing.T) {
	ctrl := testAIController()
	stub := stubProvider{res: ai.AnalyzeResult{
		Name: "DeWalt 20V Drill", Description: "Yellow cordless drill.",
		Manufacturer: "DeWalt", ModelNumber: "DCD771",
		CategoryHints: []string{"power tool"}, Confidence: 0.9,
	}}

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzePhoto(stub)(rec, multipartPhotoRequest(t, "file", tinyPNG(t)))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp AnalyzePhotoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "vision", resp.Lane)
	assert.Equal(t, []string{"power tool"}, resp.CategoryHints)
	require.Len(t, resp.Products, 1)

	p := resp.Products[0]
	assert.Equal(t, "ai-vision", p.SearchEngineName)
	assert.Equal(t, "DeWalt", p.Manufacturer)
	assert.Equal(t, "DCD771", p.ModelNumber)
	assert.Equal(t, "DeWalt 20V Drill", p.Item.Name)
	assert.Equal(t, "DeWalt", p.Item.Manufacturer)
	assert.Equal(t, "DCD771", p.Item.ModelNumber)
	assert.InDelta(t, float64(1), p.Item.Quantity, 0)
	assert.Empty(t, p.Barcode)
	assert.Contains(t, p.ImageBase64, "data:image/png;base64,")
	assert.Equal(t, p.ImageBase64, p.ImageURL, "prefill guard in CreateModal checks imageURL truthiness")
}

func TestHandleAnalyzePhoto_MissingFile(t *testing.T) {
	ctrl := testAIController()

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/v1/actions/analyze-photo", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzePhoto(stubProvider{})(rec, req)
	require.Error(t, err)
}

func TestHandleAnalyzePhoto_NonImageRejected(t *testing.T) {
	ctrl := testAIController()

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzePhoto(stubProvider{})(rec, multipartPhotoRequest(t, "file", []byte("plain text, not an image")))
	require.Error(t, err)
}

func TestHandleAnalyzePhoto_ProviderErrorIsBadGateway(t *testing.T) {
	ctrl := testAIController()
	stub := stubProvider{err: errors.New("model exploded")}

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzePhoto(stub)(rec, multipartPhotoRequest(t, "file", tinyPNG(t)))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "model exploded", "provider internals must not leak to the client-facing error")
}

func bulkRequest(t *testing.T, content []byte) *http.Request {
	t.Helper()
	req := multipartPhotoRequest(t, "file", content)
	req.URL.Path = "/v1/actions/analyze-photo-bulk"
	return req
}

func TestHandleAnalyzeBulk_Success(t *testing.T) {
	ctrl := testAIController()
	stub := stubProvider{res: ai.AnalyzeResult{
		Name: "Camping Stove", Description: "Green stove.", Manufacturer: "Coleman",
		Quantity: 1, CategoryHints: []string{"camping"}, Confidence: 0.9,
	}}

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzeBulk(stub)(rec, bulkRequest(t, tinyPNG(t)))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp AnalyzeBulkResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "vision-bulk", resp.Lane)
	require.Len(t, resp.Candidates, 1)
	assert.Equal(t, "Camping Stove", resp.Candidates[0].Name)
	assert.InDelta(t, float64(1), resp.Candidates[0].Quantity, 0.001)
	assert.Equal(t, []string{"camping"}, resp.Candidates[0].CategoryHints)
}

// emptyBulkStub returns zero candidates from AnalyzeContents. stubProvider
// always wraps its single res in a one-element slice, so a dedicated stub is
// needed here to exercise the "provider found nothing" path.
type emptyBulkStub struct{ stubProvider }

func (emptyBulkStub) AnalyzeContents(_ context.Context, _ []byte, _ string) ([]ai.AnalyzeResult, error) {
	return []ai.AnalyzeResult{}, nil
}

func TestHandleAnalyzeBulk_EmptyResultIsOK(t *testing.T) {
	ctrl := testAIController()

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzeBulk(emptyBulkStub{})(rec, bulkRequest(t, tinyPNG(t)))
	require.NoError(t, err)

	var resp AnalyzeBulkResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Candidates)
	assert.Empty(t, resp.Candidates)
}

func TestHandleAnalyzeBulk_ProviderErrorIsBadGateway(t *testing.T) {
	ctrl := testAIController()
	stub := stubProvider{err: errors.New("model exploded")}

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzeBulk(stub)(rec, bulkRequest(t, tinyPNG(t)))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "model exploded")
}
