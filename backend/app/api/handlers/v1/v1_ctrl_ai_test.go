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
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
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

func (s stubProvider) IdentifyKeyword(_ context.Context, _ string) (ai.AnalyzeResult, error) {
	return s.res, s.err
}

// fakeIntegrationsStore satisfies services.GroupIntegrationsStore without a
// real database — this package has no DB test fixture (see helpers_test.go
// and v1_ctrl_integrations_test.go's doc comment). It always reports a group
// with no stored settings ("" for every field), so IntegrationsService's
// per-field resolution falls through to whatever fallback conf the test
// wires in via services.NewIntegrationsService.
type fakeIntegrationsStore struct{}

func (fakeIntegrationsStore) IntegrationsGet(_ context.Context, _ uuid.UUID) (types.GroupIntegrations, error) {
	return types.GroupIntegrations{}, nil
}

func (fakeIntegrationsStore) IntegrationsSet(_ context.Context, _ uuid.UUID, _ types.GroupIntegrations) error {
	return nil
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

// testAIController builds a controller with AI "configured" (env fallback
// conf has a provider) and aiProviderFactory wired to always return the
// given stub, regardless of the resolved config.AIConf. This mirrors the
// pre-S5 fixture's behavior of "AI is on, and this is the provider" — the
// only thing that changed mechanically is that the provider is now supplied
// via WithAIProviderFactory at construction time instead of as a per-call
// handler argument.
func testAIController(provider ai.Provider) *V1Controller {
	svc := &services.AllServices{
		Integrations: services.NewIntegrationsService(
			fakeIntegrationsStore{},
			config.AIConf{Provider: "openai_compatible", TimeoutSeconds: 30},
			config.BarcodeAPIConf{},
		),
	}
	return NewControllerV1(svc, nil, nil, &config.Config{}, WithMaxUploadSize(10),
		WithAIProviderFactory(func(config.AIConf) (ai.Provider, error) { return provider, nil }))
}

// testAIControllerUnconfigured builds a controller where neither group
// settings nor env configure AI at all (empty fallback AIConf) — the runtime
// gating's "not configured" path, which now 503s at request time rather than
// leaving the route unmounted.
func testAIControllerUnconfigured() *V1Controller {
	svc := &services.AllServices{
		Integrations: services.NewIntegrationsService(
			fakeIntegrationsStore{},
			config.AIConf{},
			config.BarcodeAPIConf{},
		),
	}
	return NewControllerV1(svc, nil, nil, &config.Config{}, WithMaxUploadSize(10))
}

func TestHandleAnalyzePhoto_Success(t *testing.T) {
	stub := stubProvider{res: ai.AnalyzeResult{
		Name: "DeWalt 20V Drill", Description: "Yellow cordless drill.",
		Manufacturer: "DeWalt", ModelNumber: "DCD771",
		CategoryHints: []string{"power tool"}, Confidence: 0.9,
	}}
	ctrl := testAIController(stub)

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzePhoto()(rec, multipartPhotoRequest(t, "file", tinyPNG(t)))
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
	ctrl := testAIController(stubProvider{})

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/v1/actions/analyze-photo", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzePhoto()(rec, req)
	require.Error(t, err)
}

func TestHandleAnalyzePhoto_NonImageRejected(t *testing.T) {
	ctrl := testAIController(stubProvider{})

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzePhoto()(rec, multipartPhotoRequest(t, "file", []byte("plain text, not an image")))
	require.Error(t, err)
}

func TestHandleAnalyzePhoto_ProviderErrorIsBadGateway(t *testing.T) {
	stub := stubProvider{err: errors.New("model exploded")}
	ctrl := testAIController(stub)

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzePhoto()(rec, multipartPhotoRequest(t, "file", tinyPNG(t)))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "model exploded", "provider internals must not leak to the client-facing error")
}

// TestHandleAnalyzePhoto_Unconfigured503 covers the new runtime-gating path
// (design spec §3, acceptance criterion 6): with no group override and no
// env fallback configuring AI at all, the route (now always mounted) 503s
// instead of the old registration-time 404 (route not mounted at all).
func TestHandleAnalyzePhoto_Unconfigured503(t *testing.T) {
	ctrl := testAIControllerUnconfigured()

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzePhoto()(rec, multipartPhotoRequest(t, "file", tinyPNG(t)))
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusServiceUnavailable, reqErr.Status)
	assert.Equal(t, "ai not configured", reqErr.Error())
}

func bulkRequest(t *testing.T, content []byte) *http.Request {
	t.Helper()
	req := multipartPhotoRequest(t, "file", content)
	req.URL.Path = "/v1/actions/analyze-photo-bulk"
	return req
}

func TestHandleAnalyzeBulk_Success(t *testing.T) {
	stub := stubProvider{res: ai.AnalyzeResult{
		Name: "Camping Stove", Description: "Green stove.", Manufacturer: "Coleman",
		Quantity: 1, CategoryHints: []string{"camping"}, Confidence: 0.9,
	}}
	ctrl := testAIController(stub)

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzeBulk()(rec, bulkRequest(t, tinyPNG(t)))
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
	ctrl := testAIController(emptyBulkStub{})

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzeBulk()(rec, bulkRequest(t, tinyPNG(t)))
	require.NoError(t, err)

	var resp AnalyzeBulkResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Candidates)
	assert.Empty(t, resp.Candidates)
}

func TestHandleAnalyzeBulk_ProviderErrorIsBadGateway(t *testing.T) {
	stub := stubProvider{err: errors.New("model exploded")}
	ctrl := testAIController(stub)

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzeBulk()(rec, bulkRequest(t, tinyPNG(t)))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "model exploded")
}

// TestHandleAnalyzeBulk_Unconfigured503 mirrors
// TestHandleAnalyzePhoto_Unconfigured503 for the bulk-analyze route.
func TestHandleAnalyzeBulk_Unconfigured503(t *testing.T) {
	ctrl := testAIControllerUnconfigured()

	rec := httptest.NewRecorder()
	err := ctrl.HandleAnalyzeBulk()(rec, bulkRequest(t, tinyPNG(t)))
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusServiceUnavailable, reqErr.Status)
}

// --- identify-from-keyword action ---

func identifyKeywordRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/actions/identify-from-keyword", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestHandleIdentifyFromKeyword_Success(t *testing.T) {
	stub := stubProvider{res: ai.AnalyzeResult{
		Name: "DeWalt 20V Drill", Description: "Yellow cordless drill.",
		Manufacturer: "DeWalt", ModelNumber: "DCD771",
		CategoryHints: []string{"power tool"}, Confidence: 0.8,
	}}
	ctrl := testAIController(stub)

	rec := httptest.NewRecorder()
	err := ctrl.HandleIdentifyFromKeyword()(rec, identifyKeywordRequest(`{"keyword":"dewalt drill"}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp IdentifyFromKeywordResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.AIGuess, "keyword identification must be flagged as an AI guess")

	p := resp.Product
	assert.Equal(t, "ai-keyword", p.SearchEngineName)
	assert.Equal(t, "DeWalt", p.Manufacturer)
	assert.Equal(t, "DCD771", p.ModelNumber)
	assert.Equal(t, "DeWalt 20V Drill", p.Item.Name)
	assert.Equal(t, "Yellow cordless drill.", p.Item.Description)
	assert.Equal(t, "DeWalt", p.Item.Manufacturer)
	assert.Equal(t, "DCD771", p.Item.ModelNumber)
	assert.InDelta(t, float64(1), p.Item.Quantity, 0)
	assert.Empty(t, p.Barcode)
	assert.Empty(t, p.ImageURL, "text-only guess carries no product image")
	assert.Empty(t, p.ImageBase64)
}

// TestHandleIdentifyFromKeyword_UncertainFieldsStayEmpty pins the contract
// that empty manufacturer/model from the provider (the prompt's "never
// invent" instruction) pass through as empty strings, not fabrications.
func TestHandleIdentifyFromKeyword_UncertainFieldsStayEmpty(t *testing.T) {
	stub := stubProvider{res: ai.AnalyzeResult{Name: "Garden Hose", Description: "A green hose."}}
	ctrl := testAIController(stub)

	rec := httptest.NewRecorder()
	err := ctrl.HandleIdentifyFromKeyword()(rec, identifyKeywordRequest(`{"keyword":"hose"}`))
	require.NoError(t, err)

	var resp IdentifyFromKeywordResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp.Product.Manufacturer)
	assert.Empty(t, resp.Product.ModelNumber)
	assert.Empty(t, resp.Product.Item.Manufacturer)
	assert.Empty(t, resp.Product.Item.ModelNumber)
}

func TestHandleIdentifyFromKeyword_EmptyKeyword400(t *testing.T) {
	ctrl := testAIController(stubProvider{})

	for name, body := range map[string]string{
		"missing":    `{}`,
		"whitespace": `{"keyword":"   "}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			err := ctrl.HandleIdentifyFromKeyword()(rec, identifyKeywordRequest(body))
			require.Error(t, err)

			var reqErr *validate.RequestError
			require.ErrorAs(t, err, &reqErr)
			assert.Equal(t, http.StatusBadRequest, reqErr.Status)
		})
	}
}

// TestHandleIdentifyFromKeyword_ProviderErrorIsBadGateway also covers
// malformed AI output: an unparseable model reply surfaces from the provider
// as an error (see ai.parseAnalyzeResult), which the handler maps to a clean
// 502 without leaking provider internals.
func TestHandleIdentifyFromKeyword_ProviderErrorIsBadGateway(t *testing.T) {
	stub := stubProvider{err: errors.New("model reply is not valid JSON: gibberish")}
	ctrl := testAIController(stub)

	rec := httptest.NewRecorder()
	err := ctrl.HandleIdentifyFromKeyword()(rec, identifyKeywordRequest(`{"keyword":"dewalt drill"}`))
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusBadGateway, reqErr.Status)
	assert.NotContains(t, err.Error(), "gibberish", "provider internals must not leak to the client-facing error")
}

func TestHandleIdentifyFromKeyword_Unconfigured503(t *testing.T) {
	ctrl := testAIControllerUnconfigured()

	rec := httptest.NewRecorder()
	err := ctrl.HandleIdentifyFromKeyword()(rec, identifyKeywordRequest(`{"keyword":"dewalt drill"}`))
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusServiceUnavailable, reqErr.Status)
	assert.Equal(t, "ai not configured", reqErr.Error())
}
