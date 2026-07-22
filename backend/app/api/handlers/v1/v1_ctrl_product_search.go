package v1

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"github.com/sysadminsmedia/homebox/backend/internal/web/adapters"
)

const (
	barcodeHTTPTimeoutSec       = 10
	schemeHTTPS                 = "https"
	maxBarcodeAPIResponseBytes  = int64(4 << 20)
	maxProductImageBytes        = int64(8 << 20)
	maxProductImageRedirectHops = 10
	maxKeywordSearchResults     = 10
)

// upcitemdbBaseURL is a package variable (not a const) so handler tests can
// point both the barcode lookup and the keyword search at a mock HTTP server.
var upcitemdbBaseURL = "https://api.upcitemdb.com"

type imageResolver func(context.Context, string) ([]net.IP, error)
type imageDialer func(context.Context, string, string) (net.Conn, error)

func readBoundedHTTPBody(body io.Reader, contentLength, maxBytes int64) ([]byte, error) {
	if contentLength > maxBytes {
		return nil, fmt.Errorf("response body declares %d bytes, exceeds limit %d", contentLength, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds limit %d", maxBytes)
	}
	return data, nil
}

// flexibleString is a string that can be unmarshaled from either a JSON
// string or a JSON number. upcitemdb.com sometimes returns price fields
// (e.g. list_price, shipping) as numbers rather than strings, which would
// otherwise cause json.Unmarshal to fail on a plain string field.
type flexibleString string

// UnmarshalJSON accepts a JSON string, number, or null and stores the result
// as a Go string. Composite tokens (objects and arrays) are rejected so that
// unexpected shapes surface as clear errors instead of being silently stored
// as their raw representation.
func (f *flexibleString) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		*f = ""
		return nil
	}

	switch c := trimmed[0]; {
	case c == '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		*f = flexibleString(s)
		return nil
	case c == 'n':
		if string(trimmed) != "null" {
			return fmt.Errorf("flexibleString: unexpected token %q", string(trimmed))
		}
		*f = ""
		return nil
	case c == '-' || (c >= '0' && c <= '9'):
		// Validate the token is a well-formed JSON number, then store its literal form.
		var n json.Number
		if err := json.Unmarshal(trimmed, &n); err != nil {
			return err
		}
		*f = flexibleString(n.String())
		return nil
	default:
		return fmt.Errorf("flexibleString: cannot unmarshal %s into string", string(trimmed))
	}
}

type UPCITEMDBResponse struct {
	Code   string `json:"code"`
	Total  int    `json:"total"`
	Offset int    `json:"offset"`
	Items  []struct {
		Ean                  string   `json:"ean"`
		Title                string   `json:"title"`
		Description          string   `json:"description"`
		Upc                  string   `json:"upc"`
		Brand                string   `json:"brand"`
		Model                string   `json:"model"`
		Color                string   `json:"color"`
		Size                 string   `json:"size"`
		Dimension            string   `json:"dimension"`
		Weight               string   `json:"weight"`
		Category             string   `json:"category"`
		LowestRecordedPrice  float64  `json:"lowest_recorded_price"`
		HighestRecordedPrice float64  `json:"highest_recorded_price"`
		Images               []string `json:"images"`
		Offers               []struct {
			Merchant     string         `json:"merchant"`
			Domain       string         `json:"domain"`
			Title        string         `json:"title"`
			Currency     string         `json:"currency"`
			ListPrice    flexibleString `json:"list_price"`
			Price        float64        `json:"price"`
			Shipping     flexibleString `json:"shipping"`
			Condition    string         `json:"condition"`
			Availability string         `json:"availability"`
			Link         string         `json:"link"`
			UpdatedT     int            `json:"updated_t"`
		} `json:"offers"`
		Asin string `json:"asin"`
		Elid string `json:"elid"`
	} `json:"items"`
}

type OpenFactsResponse struct {
	Code    string           `json:"code"`
	Status  int              `json:"status"`
	Product openFactsProduct `json:"product"`
}

// Open Food Facts, Open Beauty Facts, and Open Products Facts share the same
// product response shape and API path, so one mapper can safely serve all three.
type openFactsProduct struct {
	ProductName   string `json:"product_name"`
	Brands        string `json:"brands"`
	Categories    string `json:"categories"`
	ImageFrontURL string `json:"image_front_url"`
	ImageURL      string `json:"image_url"`
	Quantity      string `json:"quantity"`
	GenericName   string `json:"generic_name"`
}

type openFactsSource struct {
	Name    string
	BaseURL string
}

var openFactsSources = []openFactsSource{
	{Name: "openfoodfacts.org", BaseURL: "https://world.openfoodfacts.org"},
	{Name: "openbeautyfacts.org", BaseURL: "https://world.openbeautyfacts.org"},
	{Name: "openproductsfacts.org", BaseURL: "https://world.openproductsfacts.org"},
}

type BARCODESPIDER_COMResponse struct {
	ItemResponse struct {
		Code    int    `json:"code"`
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"item_response"`
	ItemAttributes struct {
		Title          string `json:"title"`
		Upc            string `json:"upc"`
		Ean            string `json:"ean"`
		ParentCategory string `json:"parent_category"`
		Category       string `json:"category"`
		Brand          string `json:"brand"`
		Model          string `json:"model"`
		Mpn            string `json:"mpn"`
		Manufacturer   string `json:"manufacturer"`
		Publisher      string `json:"publisher"`
		Asin           string `json:"asin"`
		Color          string `json:"color"`
		Size           string `json:"size"`
		Weight         string `json:"weight"`
		Image          string `json:"image"`
		IsAdult        string `json:"is_adult"`
		Description    string `json:"description"`
	} `json:"item_attributes"`
	Stores []struct {
		StoreName string `json:"store_name"`
		Title     string `json:"title"`
		Image     string `json:"image"`
		Price     string `json:"price"`
		Currency  string `json:"currency"`
		Link      string `json:"link"`
		Updated   string `json:"updated"`
	} `json:"Stores"`
}

// fetchUPCItemDB performs a GET against a upcitemdb.com endpoint with the
// shared timeout and bounded-body hardening, and decodes the response. Both
// the barcode lookup and the keyword search flow through here.
func fetchUPCItemDB(rawURL string) (result UPCITEMDBResponse, err error) {
	client := &http.Client{Timeout: barcodeHTTPTimeoutSec * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return UPCITEMDBResponse{}, err
	}

	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	if resp.StatusCode != http.StatusOK {
		return UPCITEMDBResponse{}, fmt.Errorf("API returned status code: %d", resp.StatusCode)
	}

	body, err := readBoundedHTTPBody(resp.Body, resp.ContentLength, maxBarcodeAPIResponseBytes)
	if err != nil {
		return UPCITEMDBResponse{}, err
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().Msg("Can not unmarshal JSON from upcitemdb.com")
		return UPCITEMDBResponse{}, err
	}

	return result, nil
}

// mapUPCItemDBItems converts a upcitemdb.com items[] payload into the shared
// BarcodeProduct shape. barcode is "" for keyword searches, where no barcode
// was scanned.
func mapUPCItemDBItems(result UPCITEMDBResponse, barcode string) []repo.BarcodeProduct {
	var res []repo.BarcodeProduct

	for _, it := range result.Items {
		var p repo.BarcodeProduct
		p.SearchEngineName = "upcitemdb.com"
		p.Barcode = barcode

		p.Item.Description = it.Description
		p.Item.Name = it.Title
		p.Manufacturer = it.Brand
		p.ModelNumber = it.Model
		if len(it.Images) != 0 {
			p.ImageURL = it.Images[0]
		}

		res = append(res, p)
	}

	return res
}

func lookupUPCItemDB(iEan string) ([]repo.BarcodeProduct, error) {
	result, err := fetchUPCItemDB(upcitemdbBaseURL + "/prod/trial/lookup?upc=" + url.QueryEscape(iEan))
	if err != nil {
		return nil, err
	}
	return mapUPCItemDBItems(result, iEan), nil
}

func searchUPCItemDBByKeyword(keyword string) ([]repo.BarcodeProduct, error) {
	result, err := fetchUPCItemDB(upcitemdbBaseURL + "/prod/trial/search?s=" + url.QueryEscape(keyword))
	if err != nil {
		return nil, err
	}
	return mapUPCItemDBItems(result, ""), nil
}

func lookupBarcodespider(tokenAPI string, iEan string) ([]repo.BarcodeProduct, error) {
	if len(tokenAPI) == 0 {
		return nil, errors.New("no api token configured for barcodespider. " +
			"Please define the api token in environment variable HBOX_BARCODE_TOKEN_BARCODESPIDER")
	}

	req, err := http.NewRequest(
		"GET", "https://api.barcodespider.com/v1/lookup?upc="+url.QueryEscape(iEan), nil)

	if err != nil {
		return nil, err
	}

	req.Header.Add("token", tokenAPI)

	client := &http.Client{Timeout: barcodeHTTPTimeoutSec * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("barcodespider API returned status code: %d", resp.StatusCode)
	}

	body, err := readBoundedHTTPBody(resp.Body, resp.ContentLength, maxBarcodeAPIResponseBytes)
	if err != nil {
		return nil, err
	}

	var result BARCODESPIDER_COMResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().Msg("Can not unmarshal JSON from barcodespider.com")
		return nil, err
	}

	var p repo.BarcodeProduct
	p.Barcode = iEan
	p.SearchEngineName = "barcodespider.com"
	p.Item.Name = result.ItemAttributes.Title
	p.Item.Description = result.ItemAttributes.Description
	p.Manufacturer = result.ItemAttributes.Brand
	p.ModelNumber = result.ItemAttributes.Model
	p.ImageURL = result.ItemAttributes.Image

	return []repo.BarcodeProduct{p}, nil
}

// sanitizeHeader removes control characters that could cause HTTP header injection.
func sanitizeHeader(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, s)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func isAllowedOpenFactsImageHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	allowedDomains := []string{
		"openfoodfacts.org",
		"openbeautyfacts.org",
		"openproductsfacts.org",
	}

	for _, domain := range allowedDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}

	return false
}

func normalizeOpenFactsImageURL(imageURL string) string {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return ""
	}

	u, err := url.Parse(imageURL)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return ""
	}

	switch u.Scheme {
	case "http":
		u.Scheme = schemeHTTPS
	case schemeHTTPS:
	default:
		return ""
	}

	if !isAllowedOpenFactsImageHost(u.Hostname()) {
		return ""
	}

	return u.String()
}

func buildOpenFactsBarcodeProduct(sourceName string, iEan string, product openFactsProduct) (repo.BarcodeProduct, bool) {
	name := firstNonEmpty(product.ProductName, product.GenericName, product.Brands)
	if name == "" {
		return repo.BarcodeProduct{}, false
	}

	var p repo.BarcodeProduct
	p.Barcode = iEan
	p.SearchEngineName = sourceName
	p.Item.Name = name
	p.Manufacturer = product.Brands

	var descriptionParts []string
	for _, value := range []string{product.GenericName, product.Categories, product.Quantity} {
		value = strings.TrimSpace(value)
		if value != "" && value != name {
			descriptionParts = append(descriptionParts, value)
		}
	}
	p.Item.Description = strings.Join(descriptionParts, " | ")

	p.ImageURL = normalizeOpenFactsImageURL(firstNonEmpty(product.ImageFrontURL, product.ImageURL))

	return p, true
}

func lookupOpenFacts(contact string, source openFactsSource, iEan string) ([]repo.BarcodeProduct, error) {
	client := &http.Client{Timeout: barcodeHTTPTimeoutSec * time.Second}
	req, err := http.NewRequest(
		"GET", strings.TrimRight(source.BaseURL, "/")+"/api/v2/product/"+url.PathEscape(iEan)+".json", nil)
	if err != nil {
		return nil, err
	}
	userAgent := "Homebox/1.0 (https://github.com/sysadminsmedia/homebox)"
	safeContact := sanitizeHeader(strings.TrimSpace(contact))
	if len(safeContact) > 0 {
		userAgent = "Homebox/1.0 (contact: " + safeContact + ")"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s API returned status code: %d", source.Name, resp.StatusCode)
	}

	body, err := readBoundedHTTPBody(resp.Body, resp.ContentLength, maxBarcodeAPIResponseBytes)
	if err != nil {
		return nil, err
	}

	var result OpenFactsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().Msg("Can not unmarshal " + source.Name + " JSON")
		return nil, err
	}

	if result.Status == 0 {
		return nil, nil
	}

	p, ok := buildOpenFactsBarcodeProduct(source.Name, iEan, result.Product)
	if !ok {
		return nil, nil
	}

	return []repo.BarcodeProduct{p}, nil
}

func defaultImageResolver(ctx context.Context, host string) ([]net.IP, error) {
	if literal := net.ParseIP(host); literal != nil {
		return []net.IP{literal}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

func isPublicProductImageIP(ip net.IP) bool {
	if ip == nil ||
		ip.IsUnspecified() ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() {
		return false
	}

	// Go's IsPrivate intentionally excludes carrier-grade NAT. It is still
	// shared internal address space and must not be reachable by an
	// attacker-selected product image URL.
	_, sharedNet, _ := net.ParseCIDR("100.64.0.0/10")
	if sharedNet.Contains(ip) {
		return false
	}

	// A NAT64 address can carry a blocked IPv4 destination while looking like
	// a public IPv6 address. Check the RFC 6052 well-known /96 embedding.
	ip16 := ip.To16()
	_, wellKnownDNS64, _ := net.ParseCIDR("64:ff9b::/96")
	if ip.To4() == nil && ip16 != nil && wellKnownDNS64.Contains(ip) {
		embedded := net.IPv4(ip16[12], ip16[13], ip16[14], ip16[15])
		return isPublicProductImageIP(embedded)
	}

	return ip.IsGlobalUnicast() &&
		!ip.IsUnspecified() &&
		!ip.IsMulticast()
}

func resolvePublicProductImageIPs(
	ctx context.Context,
	host string,
	resolve imageResolver,
) ([]net.IP, error) {
	ips, err := resolve(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve image host: %w", err)
	}
	if len(ips) == 0 {
		return nil, errors.New("image host resolved to no addresses")
	}
	for _, ip := range ips {
		if !isPublicProductImageIP(ip) {
			return nil, fmt.Errorf("image host resolved to blocked address %s", ip)
		}
	}
	return ips, nil
}

func validateProductImageURL(
	ctx context.Context,
	rawURL string,
	resolve imageResolver,
) (*url.URL, error) {
	u, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid image URL: %w", err)
	}
	if u.Scheme != schemeHTTPS || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("image URL must be an absolute HTTPS URL without credentials or fragment")
	}
	if _, err := resolvePublicProductImageIPs(ctx, u.Hostname(), resolve); err != nil {
		return nil, err
	}
	return u, nil
}

func productImageDialContext(resolve imageResolver, dial imageDialer) imageDialer {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid image dial address: %w", err)
		}
		ips, err := resolvePublicProductImageIPs(ctx, host, resolve)
		if err != nil {
			return nil, err
		}

		var dialErrs []error
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErrs = append(dialErrs, err)
		}
		return nil, fmt.Errorf("connect to image host: %w", errors.Join(dialErrs...))
	}
}

func productImageRedirectGuard(resolve imageResolver) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxProductImageRedirectHops {
			return fmt.Errorf("stopped after %d image redirects", maxProductImageRedirectHops)
		}
		if req.URL == nil {
			return errors.New("image redirect has no URL")
		}
		_, err := validateProductImageURL(req.Context(), req.URL.String(), resolve)
		if err != nil {
			return fmt.Errorf("blocked image redirect: %w", err)
		}
		return nil
	}
}

func newProductImageHTTPClient(resolve imageResolver, dial imageDialer) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A proxy would perform its own target resolution after Homebox's
	// validation, defeating the address pinning.
	transport.Proxy = nil
	transport.DialContext = productImageDialContext(resolve, dial)
	return &http.Client{
		Timeout:       barcodeHTTPTimeoutSec * time.Second,
		Transport:     transport,
		CheckRedirect: productImageRedirectGuard(resolve),
	}
}

// fetchImageBase64 fetches a public HTTPS image and returns it as a base64 data URI.
func fetchImageBase64(ctx context.Context, imageURL string) (string, error) {
	resolve := imageResolver(defaultImageResolver)
	u, err := validateProductImageURL(ctx, imageURL, resolve)
	if err != nil {
		return "", err
	}
	dialer := &net.Dialer{}
	client := newProductImageHTTPClient(resolve, dialer.DialContext)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image fetch returned status %d", res.StatusCode)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("non-image content type: %s", contentType)
	}

	imageBytes, err := readBoundedHTTPBody(res.Body, res.ContentLength, maxProductImageBytes)
	if err != nil {
		return "", err
	}

	mimeType := http.DetectContentType(imageBytes)
	var base64Encoding string
	switch mimeType {
	case "image/jpeg":
		base64Encoding = "data:image/jpeg;base64,"
	case "image/png":
		base64Encoding = "data:image/png;base64,"
	default:
		return "", fmt.Errorf("unsupported image type: %s", mimeType)
	}

	return base64Encoding + base64.StdEncoding.EncodeToString(imageBytes), nil
}

// HandleProductSearchFromBarcode godoc
//
//	@Summary	Search EAN from Barcode
//	@Tags		Items
//	@Produce	json
//	@Param		data	query		string	false	"barcode to be searched"
//	@Success	200		{object}	[]repo.BarcodeProduct
//	@Router		/v1/products/search-from-barcode [GET]
//	@Security	Bearer
func (ctrl *V1Controller) HandleProductSearchFromBarcode() errchain.HandlerFunc {
	type query struct {
		// 80 characters is the longest non-2D barcode length (GS1-128)
		EAN string `schema:"productEAN" validate:"required,max=80"`
	}

	return func(w http.ResponseWriter, r *http.Request) error {
		q, err := adapters.DecodeQuery[query](r)
		if err != nil {
			return err
		}

		ctx := services.NewContext(r.Context())
		conf, err := ctrl.svc.Integrations.EffectiveBarcode(ctx, ctx.GID)
		if err != nil {
			return err
		}

		log.Info().Msg("Processing barcode lookup request on: " + q.EAN)

		var products []repo.BarcodeProduct

		// www.ean-search.org/: not free

		// Example code: dewalt 5035048748428

		ps, err := lookupUPCItemDB(q.EAN)
		if err != nil {
			log.Error().Msg("Can not retrieve product from upcitemdb.com: " + err.Error())
		}
		products = append(products, ps...)

		if conf.TokenBarcodespider != "" {
			ps2, err := lookupBarcodespider(conf.TokenBarcodespider, q.EAN)
			if err != nil {
				log.Error().Msg("Can not retrieve product from barcodespider.com: " + err.Error())
			}
			products = append(products, ps2...)
		}

		for _, source := range openFactsSources {
			ps3, err := lookupOpenFacts(conf.OpenFoodFactsContact, source, q.EAN)
			if err != nil {
				log.Error().Msg("Can not retrieve product from " + source.Name + ": " + err.Error())
			}
			products = append(products, ps3...)
		}

		attachProductImages(r.Context(), products)

		if len(products) != 0 {
			return server.JSON(w, http.StatusOK, products)
		}

		return server.JSON(w, http.StatusNoContent, nil)
	}
}

// attachProductImages resolves each product's ImageURL through the hardened
// image fetcher (public-HTTPS-only, size cap, redirect guard) and stores the
// result as a base64 data URI. Failures are logged and skipped so a bad image
// never sinks the whole result set.
func attachProductImages(ctx context.Context, products []repo.BarcodeProduct) {
	for i := range products {
		p := &products[i]

		if len(p.ImageURL) == 0 {
			continue
		}

		base64Img, err := fetchImageBase64(ctx, p.ImageURL)
		if err != nil {
			log.Warn().Str("image_url", redactExternalURLForTrace(p.ImageURL)).Err(err).Msg("cannot fetch product image")
			continue
		}
		p.ImageBase64 = base64Img
	}
}

// keywordSearchProvider is one entry in the keyword lookup chain. Only
// upcitemdb.com offers a comparable free keyword search today; additional
// providers chain here the same way the barcode providers do in
// HandleProductSearchFromBarcode (append another entry consuming conf).
type keywordSearchProvider struct {
	name   string
	search func(keyword string) ([]repo.BarcodeProduct, error)
}

func keywordSearchProviders(_ config.BarcodeAPIConf) []keywordSearchProvider {
	return []keywordSearchProvider{
		{name: "upcitemdb.com", search: searchUPCItemDBByKeyword},
	}
}

// HandleProductSearchFromKeyword godoc
//
//	@Summary	Search Products by Keyword
//	@Tags		Items
//	@Produce	json
//	@Param		keyword	query		string	true	"keyword to search products for"
//	@Success	200		{object}	[]repo.BarcodeProduct
//	@Router		/v1/products/search-from-keyword [GET]
//	@Security	Bearer
func (ctrl *V1Controller) HandleProductSearchFromKeyword() errchain.HandlerFunc {
	type query struct {
		Keyword string `schema:"keyword" validate:"max=200"`
	}

	return func(w http.ResponseWriter, r *http.Request) error {
		q, err := adapters.DecodeQuery[query](r)
		if err != nil {
			return err
		}

		keyword := strings.TrimSpace(q.Keyword)
		if keyword == "" {
			return validate.NewRequestError(errors.New("keyword is required"), http.StatusBadRequest)
		}

		ctx := services.NewContext(r.Context())
		conf, err := ctrl.svc.Integrations.EffectiveBarcode(ctx, ctx.GID)
		if err != nil {
			return err
		}

		log.Info().Msg("Processing keyword product search")

		var products []repo.BarcodeProduct
		var providerErrs []error

		for _, provider := range keywordSearchProviders(conf) {
			ps, err := provider.search(keyword)
			if err != nil {
				log.Error().Msg("Can not retrieve products from " + provider.name + ": " + err.Error())
				providerErrs = append(providerErrs, err)
				continue
			}
			products = append(products, ps...)

			if len(products) >= maxKeywordSearchResults {
				products = products[:maxKeywordSearchResults]
				break
			}
		}

		// Distinguishable failure: when every provider errored and nothing was
		// found, the frontend must be able to tell "search is broken" (502)
		// apart from "no matches" (204).
		if len(products) == 0 && len(providerErrs) > 0 {
			return validate.NewRequestError(errors.New("keyword search provider error"), http.StatusBadGateway)
		}

		attachProductImages(r.Context(), products)

		if len(products) != 0 {
			return server.JSON(w, http.StatusOK, products)
		}

		return server.JSON(w, http.StatusNoContent, nil)
	}
}
