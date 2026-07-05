package v1

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"github.com/sysadminsmedia/homebox/backend/internal/web/adapters"
	"github.com/sysadminsmedia/homebox/backend/pkgs/ai"
)

// testBarcodeEAN is a fixed, stable, publicly-known EAN used solely to
// exercise the barcodespider API round-trip (auth + connectivity), not to
// look up anything meaningful for the caller's inventory.
const testBarcodeEAN = "0012993441012"

// Coarse, sanitized detail strings for TestConnectionResponse — see
// classifyTestError's doc comment for what each bucket means.
const (
	testDetailConnectionFailed  = "connection failed"
	testDetailAuthFailed        = "authentication failed"
	testDetailProviderError     = "provider error"
	testDetailResponded         = "responded"
	testDetailAINotConfigured   = "ai not configured"
	testDetailNoTokenConfigured = "no token configured"
)

// TestConnectionResponse is the shared shape for both test-connection
// endpoints (design spec §4): 200 either way, ok/detail only. detail is
// always a coarse, sanitized string — never the raw provider error, which
// may contain a target URL, an API key fragment, or other sensitive detail.
// The full error is logged server-side by the caller before it is discarded
// down to one of these buckets.
type TestConnectionResponse struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// testImagePNG is a 1x1 transparent PNG used as the fixed payload for
// POST /v1/groups/integrations/test-ai. It exists purely to exercise the
// configured vision provider's plumbing (auth, reachability, response
// shape) — nothing meaningful is expected to be identified in it.
//
// Generated at package init via image/png rather than embedding a byte
// literal: a hand-copied literal is opaque to review and a single wrong
// byte would silently produce a corrupt fixture, whereas image.NewRGBA +
// png.Encode is self-evidently correct and cannot fail for a 1x1 image.
var testImagePNG = generateTestImagePNG()

func generateTestImagePNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		// png.Encode cannot fail for an in-memory 1x1 RGBA image; a panic
		// here would only ever fire on a Go stdlib regression.
		panic(fmt.Sprintf("failed to generate test-ai fixture image: %v", err))
	}
	return buf.Bytes()
}

// GroupIntegrationsOut is the redacted, owner-aware GET/PUT response shape for
// /v1/groups/integrations (design spec §4). The embedded types.GroupIntegrations
// carries the group's own stored settings with only the two secret fields
// (AIAPIKey, BarcodeTokenBarcodespider) touched — never returned in
// plaintext. AIConfigured/BarcodespiderConfigured and the EnvAI* summary
// fields reflect the *effective* (group-over-env) config, so the frontend can
// gate buttons and render a "server default" hint without ever seeing a key.
type GroupIntegrationsOut struct {
	types.GroupIntegrations
	IsOwner                 bool   `json:"isOwner"`
	AIConfigured            bool   `json:"aiConfigured"`
	BarcodespiderConfigured bool   `json:"barcodespiderConfigured"`
	EnvAIProvider           string `json:"envAiProvider"`
	EnvAIBaseURL            string `json:"envAiBaseUrl"`
	EnvAIModel              string `json:"envAiModel"`
}

// redactIntegrations builds the GET/PUT response shape from the group's raw
// stored settings plus the already-resolved effective AI/barcode config and
// the raw env-only AI fallback (services.IntegrationsService.EnvAI).
//
// Secret redaction considers BOTH sources: a secret field renders as
// config.RedactedValue whenever either the group's own stored value OR the
// effective (env-fallback-merged) value is non-empty. A blank raw-only check
// would show an empty password field in the common case of pure env-fallback
// configuration (no group override at all — acceptance criterion 1), which
// would misleadingly suggest nothing is configured. This is safe to do
// because PUT's write-only secret merge (services.IntegrationsService.Update)
// only inspects the group's own stored row when it sees the echoed sentinel
// come back — so this display choice can never cause an env-sourced secret to
// be copied into group storage.
//
// EnvAI* fields deliberately come from envAI (raw env config), NOT
// effectiveAI: they back the "server default" hint (design spec §5), which
// must keep showing the actual env fallback even after the group configures
// its own override — otherwise the hint would relabel the group's own values
// as "server default" the moment they set anything.
func redactIntegrations(raw types.GroupIntegrations, effectiveAI config.AIConf, envAI config.AIConf, effectiveBarcode config.BarcodeAPIConf, isOwner bool) GroupIntegrationsOut {
	redacted := raw
	redacted.AIAPIKey = redactSecretField(raw.AIAPIKey, effectiveAI.APIKey)
	redacted.BarcodeTokenBarcodespider = redactSecretField(raw.BarcodeTokenBarcodespider, effectiveBarcode.TokenBarcodespider)

	return GroupIntegrationsOut{
		GroupIntegrations:       redacted,
		IsOwner:                 isOwner,
		AIConfigured:            effectiveAI.Provider != "",
		BarcodespiderConfigured: effectiveBarcode.TokenBarcodespider != "",
		EnvAIProvider:           envAI.Provider,
		EnvAIBaseURL:            envAI.BaseURL,
		EnvAIModel:              envAI.Model,
	}
}

// redactSecretField returns "" when neither source has the secret set, or
// config.RedactedValue when either does. Never returns the plaintext value.
func redactSecretField(groupValue, effectiveValue string) string {
	if groupValue != "" || effectiveValue != "" {
		return config.RedactedValue
	}
	return ""
}

// HandleIntegrationsGet godoc
//
//	@Summary	Get Group Integrations
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	GroupIntegrationsOut
//	@Router		/v1/groups/integrations [Get]
//	@Security	Bearer
func (ctrl *V1Controller) HandleIntegrationsGet() errchain.HandlerFunc {
	fn := func(r *http.Request) (GroupIntegrationsOut, error) {
		auth := services.NewContext(r.Context())

		raw, err := ctrl.svc.Integrations.Raw(auth, auth.GID)
		if err != nil {
			return GroupIntegrationsOut{}, err
		}

		effectiveAI, err := ctrl.svc.Integrations.EffectiveAI(auth, auth.GID)
		if err != nil {
			return GroupIntegrationsOut{}, err
		}

		effectiveBarcode, err := ctrl.svc.Integrations.EffectiveBarcode(auth, auth.GID)
		if err != nil {
			return GroupIntegrationsOut{}, err
		}

		isOwner, err := ctrl.repo.Groups.IsOwnerOf(auth, auth.UID, auth.GID)
		if err != nil {
			return GroupIntegrationsOut{}, err
		}

		return redactIntegrations(raw, effectiveAI, ctrl.svc.Integrations.EnvAI(), effectiveBarcode, isOwner), nil
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleIntegrationsUpdate godoc
//
//	@Summary		Update Group Integrations
//	@Description	Owner-only. Body = types.GroupIntegrations. Secret fields use write-only sentinel semantics: incoming "[REDACTED]" keeps the stored value, "" clears it, anything else replaces it.
//	@Tags			Group
//	@Produce		json
//	@Param			payload	body		types.GroupIntegrations	true	"Group Integrations"
//	@Success		200		{object}	GroupIntegrationsOut
//	@Router			/v1/groups/integrations [Put]
//	@Security		Bearer
func (ctrl *V1Controller) HandleIntegrationsUpdate() errchain.HandlerFunc {
	fn := func(r *http.Request, body types.GroupIntegrations) (GroupIntegrationsOut, error) {
		auth := services.NewContext(r.Context())

		// Owner gate — verbatim pattern from HandleWipeInventory (v1_ctrl_actions.go).
		isOwner, err := ctrl.repo.Groups.IsOwnerOf(auth, auth.UID, auth.GID)
		if err != nil {
			return GroupIntegrationsOut{}, validate.NewRequestError(err, http.StatusInternalServerError)
		}
		if !isOwner {
			return GroupIntegrationsOut{}, validate.NewRequestError(errors.New("only group owners can edit integration settings"), http.StatusForbidden)
		}

		if err := ctrl.svc.Integrations.Update(auth, auth.GID, body); err != nil {
			if errors.Is(err, services.ErrInvalidAIProvider) {
				return GroupIntegrationsOut{}, validate.NewRequestError(err, http.StatusBadRequest)
			}
			return GroupIntegrationsOut{}, err
		}

		raw, err := ctrl.svc.Integrations.Raw(auth, auth.GID)
		if err != nil {
			return GroupIntegrationsOut{}, err
		}

		effectiveAI, err := ctrl.svc.Integrations.EffectiveAI(auth, auth.GID)
		if err != nil {
			return GroupIntegrationsOut{}, err
		}

		effectiveBarcode, err := ctrl.svc.Integrations.EffectiveBarcode(auth, auth.GID)
		if err != nil {
			return GroupIntegrationsOut{}, err
		}

		return redactIntegrations(raw, effectiveAI, ctrl.svc.Integrations.EnvAI(), effectiveBarcode, isOwner), nil
	}

	return adapters.Action(fn, http.StatusOK)
}

// requireIntegrationsOwner is the owner gate shared by both test-connection
// handlers — verbatim pattern from HandleIntegrationsUpdate/HandleWipeInventory.
// A non-nil error is always a fully-formed *validate.RequestError (403 or
// 500); callers just propagate it.
func requireIntegrationsOwner(ctrl *V1Controller, r *http.Request) (services.Context, error) {
	auth := services.NewContext(r.Context())

	isOwner, err := ctrl.repo.Groups.IsOwnerOf(auth, auth.UID, auth.GID)
	if err != nil {
		return auth, validate.NewRequestError(err, http.StatusInternalServerError)
	}
	if !isOwner {
		return auth, validate.NewRequestError(errors.New("only group owners can test integration settings"), http.StatusForbidden)
	}
	return auth, nil
}

// testAIResponseForConfig short-circuits test-ai when the effective AI config
// has no provider — this is the "" (never configured, group or env) case.
// Extracted as a pure function (no DB, no network) so it stays testable in
// this package, which has no DB fixtures (see v1_ctrl_integrations_test.go).
func testAIResponseForConfig(conf config.AIConf) (TestConnectionResponse, bool) {
	if conf.Provider == "" {
		return TestConnectionResponse{OK: false, Detail: testDetailAINotConfigured}, true
	}
	return TestConnectionResponse{}, false
}

// buildTestAIResult maps a provider.Analyze outcome to the response shape.
// Pure and testable without a live provider: callers pass in whatever
// Analyze returned. On success, detail prefers the identified item's name
// (proof the round trip produced a sane reply) and falls back to a generic
// "responded" when the model returned an empty name (e.g. the 1x1 blank
// test image gives it nothing to identify). On failure, the raw error is
// classified into a coarse bucket — never returned verbatim.
func buildTestAIResult(res ai.AnalyzeResult, err error) TestConnectionResponse {
	if err != nil {
		return TestConnectionResponse{OK: false, Detail: classifyTestError(err)}
	}
	detail := res.Name
	if detail == "" {
		detail = testDetailResponded
	}
	return TestConnectionResponse{OK: true, Detail: detail}
}

// testBarcodeResponseForConfig short-circuits test-barcode when no
// barcodespider token is configured (group or env). Pure, same rationale as
// testAIResponseForConfig.
func testBarcodeResponseForConfig(conf config.BarcodeAPIConf) (TestConnectionResponse, bool) {
	if conf.TokenBarcodespider == "" {
		return TestConnectionResponse{OK: false, Detail: testDetailNoTokenConfigured}, true
	}
	return TestConnectionResponse{}, false
}

// buildTestBarcodeResult maps a lookupBarcodespider outcome to the response
// shape. Pure and testable without hitting the real API.
func buildTestBarcodeResult(err error) TestConnectionResponse {
	if err != nil {
		return TestConnectionResponse{OK: false, Detail: classifyTestError(err)}
	}
	return TestConnectionResponse{OK: true, Detail: testDetailResponded}
}

// classifyTestError maps a raw provider/lookup error into one of three
// coarse, sanitized buckets safe to return in a test-connection response
// body. The underlying error can contain a target URL, a response snippet,
// or other sensitive detail, and must never reach the client — callers log
// it server-side themselves before calling this.
//
// Buckets:
//   - "connection failed": the request never got a response at all — a
//     context deadline (our own 30s timeout), or a network-level failure
//     (*url.Error / net.Error from the underlying http.Client.Do).
//   - "authentication failed": the response came back with a 401/403 status.
//     Both ai and barcodespider error paths format this as a literal
//     "status code: <n>" substring (see pkgs/ai/*.go, lookupBarcodespider),
//     so a substring match is precise without needing typed HTTP-status
//     errors.
//   - "provider error": anything else (bad status/body, unparseable reply,
//     missing required field, etc).
func classifyTestError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return testDetailConnectionFailed
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return testDetailConnectionFailed
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return testDetailConnectionFailed
	}

	msg := err.Error()
	if strings.Contains(msg, "status code: 401") || strings.Contains(msg, "status code: 403") {
		return testDetailAuthFailed
	}

	return testDetailProviderError
}

// HandleIntegrationsTestAI godoc
//
//	@Summary		Test Group AI Integration
//	@Description	Owner-only. Resolves the effective AI config and attempts a real round-trip against the configured provider using a fixed 1x1 test image, under a 30s timeout. Always 200; ok/detail report the outcome. detail is a sanitized, coarse string — never the raw provider error.
//	@Tags			Group
//	@Produce		json
//	@Success		200	{object}	TestConnectionResponse
//	@Router			/v1/groups/integrations/test-ai [Post]
//	@Security		Bearer
func (ctrl *V1Controller) HandleIntegrationsTestAI() errchain.HandlerFunc {
	fn := func(r *http.Request) (TestConnectionResponse, error) {
		auth, err := requireIntegrationsOwner(ctrl, r)
		if err != nil {
			return TestConnectionResponse{}, err
		}

		effectiveAI, err := ctrl.svc.Integrations.EffectiveAI(auth, auth.GID)
		if err != nil {
			return TestConnectionResponse{}, err
		}

		if resp, handled := testAIResponseForConfig(effectiveAI); handled {
			return resp, nil
		}

		provider, err := ai.NewProvider(effectiveAI)
		if err != nil {
			log.Err(err).Msg("test-ai: failed to construct provider for effective config")
			return TestConnectionResponse{OK: false, Detail: testDetailProviderError}, nil
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		res, analyzeErr := provider.Analyze(ctx, testImagePNG, "image/png")
		if analyzeErr != nil {
			log.Err(analyzeErr).Msg("test-ai: provider analyze failed")
		}
		return buildTestAIResult(res, analyzeErr), nil
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleIntegrationsTestBarcode godoc
//
//	@Summary		Test Group Barcode Integration
//	@Description	Owner-only. Resolves the effective barcode config and attempts a real lookup against barcodespider.com for a fixed, stable EAN. Always 200; ok/detail report the outcome. detail is a sanitized, coarse string — never the raw error.
//	@Tags			Group
//	@Produce		json
//	@Success		200	{object}	TestConnectionResponse
//	@Router			/v1/groups/integrations/test-barcode [Post]
//	@Security		Bearer
func (ctrl *V1Controller) HandleIntegrationsTestBarcode() errchain.HandlerFunc {
	fn := func(r *http.Request) (TestConnectionResponse, error) {
		auth, err := requireIntegrationsOwner(ctrl, r)
		if err != nil {
			return TestConnectionResponse{}, err
		}

		effectiveBarcode, err := ctrl.svc.Integrations.EffectiveBarcode(auth, auth.GID)
		if err != nil {
			return TestConnectionResponse{}, err
		}

		if resp, handled := testBarcodeResponseForConfig(effectiveBarcode); handled {
			return resp, nil
		}

		_, lookupErr := lookupBarcodespider(effectiveBarcode.TokenBarcodespider, testBarcodeEAN)
		if lookupErr != nil {
			log.Err(lookupErr).Msg("test-barcode: barcodespider lookup failed")
		}
		return buildTestBarcodeResult(lookupErr), nil
	}

	return adapters.Command(fn, http.StatusOK)
}
