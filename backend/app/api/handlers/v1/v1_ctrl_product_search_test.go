package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func TestUPCITEMDBResponseUnmarshalNumericListPrice(t *testing.T) {
	body := []byte(`{
		"code": "OK",
		"total": 1,
		"offset": 0,
		"items": [{
			"title": "Example",
			"offers": [{
				"merchant": "ExampleStore",
				"list_price": 19.99,
				"price": 14.5,
				"shipping": 4.25
			}]
		}]
	}`)

	var result UPCITEMDBResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if len(result.Items) != 1 || len(result.Items[0].Offers) != 1 {
		t.Fatalf("expected one item with one offer, got items=%d", len(result.Items))
	}

	offer := result.Items[0].Offers[0]
	if offer.ListPrice != "19.99" {
		t.Fatalf("expected list_price %q, got %q", "19.99", offer.ListPrice)
	}
	if offer.Shipping != "4.25" {
		t.Fatalf("expected shipping %q, got %q", "4.25", offer.Shipping)
	}
}

func TestUPCITEMDBResponseUnmarshalStringListPrice(t *testing.T) {
	body := []byte(`{
		"items": [{
			"offers": [{
				"list_price": "19.99",
				"shipping": "Free"
			}]
		}]
	}`)

	var result UPCITEMDBResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if len(result.Items) == 0 || len(result.Items[0].Offers) == 0 {
		t.Fatalf("expected at least one item with one offer, got items=%d", len(result.Items))
	}

	offer := result.Items[0].Offers[0]
	if offer.ListPrice != "19.99" {
		t.Fatalf("expected list_price %q, got %q", "19.99", offer.ListPrice)
	}
	if offer.Shipping != "Free" {
		t.Fatalf("expected shipping %q, got %q", "Free", offer.Shipping)
	}
}

func TestUPCITEMDBResponseUnmarshalNullListPrice(t *testing.T) {
	body := []byte(`{
		"items": [{
			"offers": [{
				"list_price": null,
				"shipping": null
			}]
		}]
	}`)

	var result UPCITEMDBResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if len(result.Items) == 0 || len(result.Items[0].Offers) == 0 {
		t.Fatalf("expected at least one item with one offer, got items=%d", len(result.Items))
	}

	offer := result.Items[0].Offers[0]
	if offer.ListPrice != "" {
		t.Fatalf("expected empty list_price, got %q", offer.ListPrice)
	}
	if offer.Shipping != "" {
		t.Fatalf("expected empty shipping, got %q", offer.Shipping)
	}
}

func TestFlexibleStringRejectsCompositeTypes(t *testing.T) {
	cases := map[string]string{
		"object": `{"foo":"bar"}`,
		"array":  `[1,2,3]`,
		"bool":   `true`,
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			var f flexibleString
			if err := f.UnmarshalJSON([]byte(payload)); err == nil {
				t.Fatalf("expected error for %s payload, got nil (value=%q)", name, f)
			}
		})
	}
}

func TestFlexibleStringHandlesLeadingWhitespace(t *testing.T) {
	var f flexibleString
	if err := f.UnmarshalJSON([]byte("   12.50")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != "12.50" {
		t.Fatalf("expected %q, got %q", "12.50", f)
	}
}

func TestBuildOpenFactsBarcodeProduct(t *testing.T) {
	product, ok := buildOpenFactsBarcodeProduct("openbeautyfacts.org", "3600522058124", openFactsProduct{
		ProductName:   "Shower Gel",
		Brands:        "Example Brand",
		GenericName:   "Body wash",
		Categories:    "Hygiene, Shower",
		Quantity:      "250 ml",
		ImageFrontURL: "http://images.openbeautyfacts.org/images/products/360/052/205/8124/front_en.1.400.jpg",
	})

	if !ok {
		t.Fatal("expected product to be built")
	}
	if product.Barcode != "3600522058124" {
		t.Fatalf("unexpected barcode: %s", product.Barcode)
	}
	if product.SearchEngineName != "openbeautyfacts.org" {
		t.Fatalf("unexpected source name: %s", product.SearchEngineName)
	}
	if product.Item.Name != "Shower Gel" {
		t.Fatalf("unexpected product name: %s", product.Item.Name)
	}
	if product.Manufacturer != "Example Brand" {
		t.Fatalf("unexpected manufacturer: %s", product.Manufacturer)
	}
	if product.Item.Description != "Body wash | Hygiene, Shower | 250 ml" {
		t.Fatalf("unexpected description: %s", product.Item.Description)
	}
	if product.ImageURL != "https://images.openbeautyfacts.org/images/products/360/052/205/8124/front_en.1.400.jpg" {
		t.Fatalf("unexpected image URL: %s", product.ImageURL)
	}
}

func TestBuildOpenFactsBarcodeProductFallsBackToGenericName(t *testing.T) {
	product, ok := buildOpenFactsBarcodeProduct("openproductsfacts.org", "1234567890123", openFactsProduct{
		GenericName: "Replacement filter",
		Categories:  "Appliance parts",
	})

	if !ok {
		t.Fatal("expected product to be built")
	}
	if product.Item.Name != "Replacement filter" {
		t.Fatalf("unexpected product name: %s", product.Item.Name)
	}
	if product.Item.Description != "Appliance parts" {
		t.Fatalf("unexpected description: %s", product.Item.Description)
	}
}

func TestBuildOpenFactsBarcodeProductRequiresName(t *testing.T) {
	_, ok := buildOpenFactsBarcodeProduct("openfoodfacts.org", "1234567890123", openFactsProduct{})
	if ok {
		t.Fatal("expected empty product to be ignored")
	}
}

func TestBuildOpenFactsBarcodeProductRejectsUntrustedImageHost(t *testing.T) {
	product, ok := buildOpenFactsBarcodeProduct("openfoodfacts.org", "1234567890123", openFactsProduct{
		ProductName: "Example Product",
		ImageURL:    "https://example.com/image.jpg",
	})

	if !ok {
		t.Fatal("expected product to be built")
	}
	if product.ImageURL != "" {
		t.Fatalf("expected untrusted image URL to be cleared, got %q", product.ImageURL)
	}
}

func TestBuildOpenFactsBarcodeProductRejectsUnsupportedImageScheme(t *testing.T) {
	product, ok := buildOpenFactsBarcodeProduct("openfoodfacts.org", "1234567890123", openFactsProduct{
		ProductName: "Example Product",
		ImageURL:    "ftp://images.openfoodfacts.org/image.jpg",
	})

	if !ok {
		t.Fatal("expected product to be built")
	}
	if product.ImageURL != "" {
		t.Fatalf("expected unsupported image URL to be cleared, got %q", product.ImageURL)
	}
}

func TestSanitizeHeaderRemovesControlCharacters(t *testing.T) {
	got := sanitizeHeader("owner@example.com\r\nInjected: value\t")
	if got != "owner@example.comInjected: value" {
		t.Fatalf("unexpected sanitized header: %q", got)
	}
}

func TestReadBoundedHTTPBodyRejectsDeclaredAndActualOverflow(t *testing.T) {
	body, err := readBoundedHTTPBody(strings.NewReader("1234"), 4, 4)
	if err != nil || string(body) != "1234" {
		t.Fatalf("exact limit should pass, body=%q err=%v", body, err)
	}

	if _, err := readBoundedHTTPBody(strings.NewReader("1234"), 5, 4); err == nil {
		t.Fatal("declared overflow should be rejected before reading")
	}
	if _, err := readBoundedHTTPBody(strings.NewReader("12345"), -1, 4); err == nil {
		t.Fatal("streamed overflow should be rejected")
	}
}

func productImageTestResolver(_ context.Context, host string) ([]net.IP, error) {
	switch host {
	case "public.example":
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	case "private.example":
		return []net.IP{net.ParseIP("10.0.0.1")}, nil
	case "mixed.example":
		return []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("127.0.0.1")}, nil
	default:
		if ip := net.ParseIP(host); ip != nil {
			return []net.IP{ip}, nil
		}
		return nil, &net.DNSError{Name: host, Err: "not found"}
	}
}

func TestValidateProductImageURLRequiresPublicHTTPSDestination(t *testing.T) {
	if _, err := validateProductImageURL(context.Background(), "https://public.example/image.png", productImageTestResolver); err != nil {
		t.Fatalf("public HTTPS URL should pass: %v", err)
	}

	for _, rawURL := range []string{
		"http://public.example/image.png",
		"https://user:secret@public.example/image.png",
		"https://private.example/image.png",
		"https://mixed.example/image.png",
		"https://127.0.0.1/image.png",
		"https://169.254.169.254/latest/meta-data",
		"https://100.64.0.1/image.png",
		"https://[64:ff9b::7f00:1]/image.png",
	} {
		if _, err := validateProductImageURL(context.Background(), rawURL, productImageTestResolver); err == nil {
			t.Errorf("expected URL to be blocked: %s", rawURL)
		}
	}
}

func TestProductImageDialContextBlocksDNSRebindingBeforeDial(t *testing.T) {
	called := false
	resolvePrivate := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	dial := productImageDialContext(resolvePrivate, func(context.Context, string, string) (net.Conn, error) {
		called = true
		return nil, io.EOF
	})

	conn, err := dial(context.Background(), "tcp", "rebind.example:443")
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("private rebinding result must be blocked")
	}
	if called {
		t.Fatal("underlying dialer must not be called for a blocked address")
	}
}

func TestProductImageRedirectGuardBlocksDowngradeAndPrivateTarget(t *testing.T) {
	guard := productImageRedirectGuard(productImageTestResolver)
	for _, rawURL := range []string{
		"http://public.example/image.png",
		"https://private.example/image.png",
	} {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		req := &http.Request{URL: u}
		if err := guard(req, nil); err == nil {
			t.Errorf("redirect should be blocked: %s", rawURL)
		}
	}
}

// --- keyword search handler ---

// withUPCItemDBServer points the shared upcitemdb base URL at a mock server
// for the duration of one test. Tests using it must not run in parallel.
func withUPCItemDBServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	orig := upcitemdbBaseURL
	upcitemdbBaseURL = srv.URL
	t.Cleanup(func() {
		upcitemdbBaseURL = orig
		srv.Close()
	})
}

// testKeywordSearchController builds a controller whose Integrations service
// resolves an all-defaults barcode config (fakeIntegrationsStore returns no
// stored group settings; env fallback is empty).
func testKeywordSearchController() *V1Controller {
	svc := &services.AllServices{
		Integrations: services.NewIntegrationsService(
			fakeIntegrationsStore{},
			config.AIConf{},
			config.BarcodeAPIConf{},
		),
	}
	return NewControllerV1(svc, nil, nil, &config.Config{})
}

func keywordSearchRequest(keyword string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/v1/products/search-from-keyword?keyword="+url.QueryEscape(keyword), nil)
}

func upcitemdbSearchItemsJSON(n int) string {
	items := make([]string, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, fmt.Sprintf(`{
			"title": "Product %d",
			"brand": "Brand %d",
			"model": "M-%d",
			"description": "Description %d",
			"images": []
		}`, i, i, i, i))
	}
	return `{"code":"OK","total":` + fmt.Sprint(n) + `,"offset":0,"items":[` + strings.Join(items, ",") + `]}`
}

func TestHandleProductSearchFromKeyword_Success(t *testing.T) {
	var gotPath, gotKeyword string
	withUPCItemDBServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKeyword = r.URL.Query().Get("s")
		_, _ = w.Write([]byte(`{"code":"OK","total":2,"offset":0,"items":[
			{"title":"DeWalt 20V Drill","brand":"DeWalt","model":"DCD771","description":"Cordless drill.","images":["https://127.0.0.1/img.jpg"]},
			{"title":"DeWalt Impact Driver","brand":"DeWalt","model":"DCF885","description":"Impact driver.","images":[]}
		]}`))
	})
	ctrl := testKeywordSearchController()

	rec := httptest.NewRecorder()
	err := ctrl.HandleProductSearchFromKeyword()(rec, keywordSearchRequest("dewalt drill"))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, "/prod/trial/search", gotPath)
	assert.Equal(t, "dewalt drill", gotKeyword)

	var products []repo.BarcodeProduct
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &products))
	require.Len(t, products, 2)

	p := products[0]
	assert.Equal(t, "upcitemdb.com", p.SearchEngineName)
	assert.Empty(t, p.Barcode, "keyword search has no scanned barcode")
	assert.Equal(t, "DeWalt 20V Drill", p.Item.Name)
	assert.Equal(t, "Cordless drill.", p.Item.Description)
	assert.Equal(t, "DeWalt", p.Manufacturer)
	assert.Equal(t, "DCD771", p.ModelNumber)
	assert.Equal(t, "https://127.0.0.1/img.jpg", p.ImageURL)
	// The hardened image fetcher must have refused the loopback destination,
	// proving keyword results flow through the same guarded path as barcode
	// results rather than a naive fetch.
	assert.Empty(t, p.ImageBase64)

	assert.Equal(t, "DCF885", products[1].ModelNumber)
	assert.Empty(t, products[1].ImageURL)
}

func TestHandleProductSearchFromKeyword_CapsResults(t *testing.T) {
	withUPCItemDBServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(upcitemdbSearchItemsJSON(15)))
	})
	ctrl := testKeywordSearchController()

	rec := httptest.NewRecorder()
	err := ctrl.HandleProductSearchFromKeyword()(rec, keywordSearchRequest("prolific keyword"))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var products []repo.BarcodeProduct
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &products))
	assert.Len(t, products, maxKeywordSearchResults)
}

func TestHandleProductSearchFromKeyword_EmptyKeyword400(t *testing.T) {
	// Provider must never be called; a hit would fail the test via a bogus
	// non-JSON body making the handler 502 instead of 400.
	withUPCItemDBServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("provider must not be called for an empty keyword")
	})
	ctrl := testKeywordSearchController()

	for name, target := range map[string]string{
		"missing":    "/v1/products/search-from-keyword",
		"whitespace": "/v1/products/search-from-keyword?keyword=%20%20",
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			err := ctrl.HandleProductSearchFromKeyword()(rec, httptest.NewRequest(http.MethodGet, target, nil))
			require.Error(t, err)

			var reqErr *validate.RequestError
			require.ErrorAs(t, err, &reqErr)
			assert.Equal(t, http.StatusBadRequest, reqErr.Status)
		})
	}
}

func TestHandleProductSearchFromKeyword_ProviderError502(t *testing.T) {
	withUPCItemDBServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	ctrl := testKeywordSearchController()

	rec := httptest.NewRecorder()
	err := ctrl.HandleProductSearchFromKeyword()(rec, keywordSearchRequest("dewalt drill"))
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusBadGateway, reqErr.Status, "provider failure must be distinguishable from empty results")
}

func TestHandleProductSearchFromKeyword_OversizedBody502(t *testing.T) {
	withUPCItemDBServer(t, func(w http.ResponseWriter, r *http.Request) {
		// One byte past the bounded-body limit; content is irrelevant because
		// the reader must reject before unmarshaling.
		_, _ = w.Write([]byte(strings.Repeat("a", int(maxBarcodeAPIResponseBytes)+1)))
	})
	ctrl := testKeywordSearchController()

	rec := httptest.NewRecorder()
	err := ctrl.HandleProductSearchFromKeyword()(rec, keywordSearchRequest("dewalt drill"))
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusBadGateway, reqErr.Status)
}

func TestHandleProductSearchFromKeyword_NoResults204(t *testing.T) {
	withUPCItemDBServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"OK","total":0,"offset":0,"items":[]}`))
	})
	ctrl := testKeywordSearchController()

	rec := httptest.NewRecorder()
	err := ctrl.HandleProductSearchFromKeyword()(rec, keywordSearchRequest("nothing matches this"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
