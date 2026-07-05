package v1

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/pkgs/ai"
)

// Fixture literals shared across Test_redactIntegrations cases (goconst).
const (
	testEnvBaseURL = "http://env.local/v1"
	testEnvModel   = "env-model"
)

// Test_redactIntegrations covers the pure redaction/shaping mapper behind
// GET/PUT /v1/groups/integrations. No DB is involved — this package has no
// DB test fixtures (see helpers_test.go), so owner-gating itself is not
// re-tested here: the handler's gate is a verbatim copy of the
// HandleWipeInventory pattern (already proven), and full owner-gating
// behavior over HTTP is covered by the S9 end-to-end suite.
func Test_redactIntegrations(t *testing.T) {
	tests := []struct {
		name             string
		raw              types.GroupIntegrations
		effectiveAI      config.AIConf
		envAI            config.AIConf
		effectiveBarcode config.BarcodeAPIConf
		isOwner          bool
		want             GroupIntegrationsOut
	}{
		{
			name: "nothing configured anywhere: everything empty/false",
			want: GroupIntegrationsOut{
				GroupIntegrations:       types.GroupIntegrations{},
				IsOwner:                 false,
				AIConfigured:            false,
				BarcodespiderConfigured: false,
				EnvAIProvider:           "",
				EnvAIBaseURL:            "",
				EnvAIModel:              "",
			},
		},
		{
			// Regression for the bug found in the S9 E2E pass: EnvAI* must
			// come from the raw env config (envAI), not the group's own
			// effective override — otherwise the "server default" hint
			// relabels the group's own settings as the server default the
			// moment it configures anything. effectiveAI and envAI are
			// deliberately different providers/models here so a wiring
			// mistake (using effectiveAI for EnvAI*) fails this assertion.
			name: "group overrides the provider: EnvAI* still reflects the raw env fallback, not the group's override",
			raw: types.GroupIntegrations{
				AIProvider:                "anthropic",
				AIBaseURL:                 "https://api.anthropic.com",
				AIAPIKey:                  "sk-super-secret",
				AIModel:                   "claude",
				BarcodeTokenBarcodespider: "bcs-secret-token",
				OpenFoodFactsContact:      "me@example.com",
			},
			effectiveAI: config.AIConf{
				Provider: "anthropic",
				BaseURL:  "https://api.anthropic.com",
				APIKey:   "sk-super-secret",
				Model:    "claude",
			},
			envAI: config.AIConf{
				Provider: services.AIProviderOpenAICompatible,
				BaseURL:  "http://172.27.10.57:11434/v1",
				APIKey:   "env-api-key",
				Model:    "qwen3-vl:32b",
			},
			effectiveBarcode: config.BarcodeAPIConf{
				TokenBarcodespider: "bcs-secret-token",
			},
			isOwner: true,
			want: GroupIntegrationsOut{
				GroupIntegrations: types.GroupIntegrations{
					AIProvider:                "anthropic",
					AIBaseURL:                 "https://api.anthropic.com",
					AIAPIKey:                  config.RedactedValue,
					AIModel:                   "claude",
					BarcodeTokenBarcodespider: config.RedactedValue,
					OpenFoodFactsContact:      "me@example.com",
				},
				IsOwner:                 true,
				AIConfigured:            true,
				BarcodespiderConfigured: true,
				EnvAIProvider:           services.AIProviderOpenAICompatible,
				EnvAIBaseURL:            "http://172.27.10.57:11434/v1",
				EnvAIModel:              "qwen3-vl:32b",
			},
		},
		{
			name: "env-fallback-only secrets still redact to sentinel even though the group row is empty",
			raw:  types.GroupIntegrations{}, // group has never set anything
			effectiveAI: config.AIConf{
				Provider: services.AIProviderOpenAICompatible,
				BaseURL:  testEnvBaseURL,
				APIKey:   "env-api-key", // sourced purely from env fallback
				Model:    testEnvModel,
			},
			envAI: config.AIConf{
				Provider: services.AIProviderOpenAICompatible,
				BaseURL:  testEnvBaseURL,
				APIKey:   "env-api-key",
				Model:    testEnvModel,
			},
			effectiveBarcode: config.BarcodeAPIConf{
				TokenBarcodespider: "env-bcs-token",
			},
			isOwner: false,
			want: GroupIntegrationsOut{
				GroupIntegrations: types.GroupIntegrations{
					AIAPIKey:                  config.RedactedValue,
					BarcodeTokenBarcodespider: config.RedactedValue,
				},
				IsOwner:                 false,
				AIConfigured:            true,
				BarcodespiderConfigured: true,
				EnvAIProvider:           services.AIProviderOpenAICompatible,
				EnvAIBaseURL:            testEnvBaseURL,
				EnvAIModel:              testEnvModel,
			},
		},
		{
			name: "disabled provider: AIConfigured false, but EnvAI* still shows what env alone would provide",
			raw: types.GroupIntegrations{
				AIProvider: "disabled",
				AIAPIKey:   "leftover-key-from-before-disabling",
			},
			effectiveAI: config.AIConf{
				// EffectiveAI zeroes Provider/BaseURL/APIKey/Model when disabled.
			},
			envAI: config.AIConf{
				// The raw env fallback is untouched by the group's disable —
				// the hint should still tell the owner what re-enabling
				// "Inherit" would give them.
				Provider: services.AIProviderOpenAICompatible,
				BaseURL:  testEnvBaseURL,
				Model:    testEnvModel,
			},
			isOwner: true,
			want: GroupIntegrationsOut{
				GroupIntegrations: types.GroupIntegrations{
					AIProvider: "disabled",
					// Still shown as set: the group DOES have a stored secret,
					// independent of whether the current provider is active.
					AIAPIKey: config.RedactedValue,
				},
				IsOwner:                 true,
				AIConfigured:            false,
				BarcodespiderConfigured: false,
				EnvAIProvider:           services.AIProviderOpenAICompatible,
				EnvAIBaseURL:            testEnvBaseURL,
				EnvAIModel:              testEnvModel,
			},
		},
		{
			name: "group secret set but no effective AI provider (edge case): still redacts",
			raw: types.GroupIntegrations{
				AIAPIKey: "some-key",
			},
			effectiveAI: config.AIConf{},
			want: GroupIntegrationsOut{
				GroupIntegrations: types.GroupIntegrations{
					AIAPIKey: config.RedactedValue,
				},
				AIConfigured: false,
			},
		},
		{
			name: "barcode: group token only, no env",
			raw: types.GroupIntegrations{
				BarcodeTokenBarcodespider: "group-token",
			},
			effectiveBarcode: config.BarcodeAPIConf{},
			want: GroupIntegrationsOut{
				GroupIntegrations: types.GroupIntegrations{
					BarcodeTokenBarcodespider: config.RedactedValue,
				},
				BarcodespiderConfigured: false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redactIntegrations(tc.raw, tc.effectiveAI, tc.envAI, tc.effectiveBarcode, tc.isOwner)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Test_redactSecretField pins down the field-level helper in isolation: only
// "both sources empty" yields "", everything else yields the sentinel, and
// the plaintext value is never one of the possible outputs.
func Test_redactSecretField(t *testing.T) {
	tests := []struct {
		name           string
		groupValue     string
		effectiveValue string
		want           string
	}{
		{name: "both empty -> empty", groupValue: "", effectiveValue: "", want: ""},
		{name: "group set only -> redacted", groupValue: "secret", effectiveValue: "", want: config.RedactedValue},
		{name: "effective set only -> redacted", groupValue: "", effectiveValue: "secret", want: config.RedactedValue},
		{name: "both set -> redacted", groupValue: "a", effectiveValue: "b", want: config.RedactedValue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSecretField(tc.groupValue, tc.effectiveValue)
			assert.Equal(t, tc.want, got)
			if tc.groupValue != "" {
				assert.NotEqual(t, tc.groupValue, got, "must never leak the group plaintext value")
			}
			if tc.effectiveValue != "" {
				assert.NotEqual(t, tc.effectiveValue, got, "must never leak the effective plaintext value")
			}
		})
	}
}

// Test_testImagePNG pins the test-ai fixture image down: it must actually be
// a valid, decodable 1x1 PNG (not just non-empty bytes), since the whole
// point is to hand the configured provider a real image.
func Test_testImagePNG(t *testing.T) {
	require.NotEmpty(t, testImagePNG)

	img, err := png.Decode(bytes.NewReader(testImagePNG))
	require.NoError(t, err, "testImagePNG must decode as a valid PNG")

	bounds := img.Bounds()
	assert.Equal(t, 1, bounds.Dx())
	assert.Equal(t, 1, bounds.Dy())
}

// Test_testAIResponseForConfig covers the "" (not configured, no group
// override and no env fallback) short-circuit for POST test-ai. This is
// reachable purely (no DB) because it operates on an already-resolved
// config.AIConf rather than fetching one — see the doc comment on
// testAIResponseForConfig for why this package can't test the DB-backed
// EffectiveAI call itself.
func Test_testAIResponseForConfig(t *testing.T) {
	tests := []struct {
		name        string
		conf        config.AIConf
		wantHandled bool
		want        TestConnectionResponse
	}{
		{
			name:        "no provider anywhere -> not configured, short-circuited",
			conf:        config.AIConf{},
			wantHandled: true,
			want:        TestConnectionResponse{OK: false, Detail: testDetailAINotConfigured},
		},
		{
			name:        "provider configured -> not handled here, caller proceeds to Analyze",
			conf:        config.AIConf{Provider: services.AIProviderOpenAICompatible, BaseURL: "http://localhost:1234"},
			wantHandled: false,
			want:        TestConnectionResponse{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, handled := testAIResponseForConfig(tc.conf)
			assert.Equal(t, tc.wantHandled, handled)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Test_testBarcodeResponseForConfig mirrors Test_testAIResponseForConfig for
// the barcode not-configured branch.
func Test_testBarcodeResponseForConfig(t *testing.T) {
	tests := []struct {
		name        string
		conf        config.BarcodeAPIConf
		wantHandled bool
		want        TestConnectionResponse
	}{
		{
			name:        "no token anywhere -> not configured, short-circuited",
			conf:        config.BarcodeAPIConf{},
			wantHandled: true,
			want:        TestConnectionResponse{OK: false, Detail: testDetailNoTokenConfigured},
		},
		{
			name:        "token configured -> not handled here, caller proceeds to lookup",
			conf:        config.BarcodeAPIConf{TokenBarcodespider: "some-token"},
			wantHandled: false,
			want:        TestConnectionResponse{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, handled := testBarcodeResponseForConfig(tc.conf)
			assert.Equal(t, tc.wantHandled, handled)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Test_buildTestAIResult covers the success/failure mapping for POST
// test-ai's provider.Analyze outcome, without ever invoking a real provider.
func Test_buildTestAIResult(t *testing.T) {
	tests := []struct {
		name string
		res  ai.AnalyzeResult
		err  error
		want TestConnectionResponse
	}{
		{
			name: "success with a name -> ok, detail is the identified name",
			res:  ai.AnalyzeResult{Name: "Mystery Item"},
			want: TestConnectionResponse{OK: true, Detail: "Mystery Item"},
		},
		{
			name: "success with an empty name (blank test image) -> ok, generic detail",
			res:  ai.AnalyzeResult{},
			want: TestConnectionResponse{OK: true, Detail: testDetailResponded},
		},
		{
			name: "failure -> not ok, error classified not echoed",
			err:  errors.New("vision provider returned status code: 401: unauthorized"),
			want: TestConnectionResponse{OK: false, Detail: testDetailAuthFailed},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildTestAIResult(tc.res, tc.err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Test_buildTestBarcodeResult mirrors Test_buildTestAIResult for POST
// test-barcode's lookupBarcodespider outcome.
func Test_buildTestBarcodeResult(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want TestConnectionResponse
	}{
		{
			name: "success -> ok, generic detail",
			want: TestConnectionResponse{OK: true, Detail: testDetailResponded},
		},
		{
			name: "failure -> not ok, error classified not echoed",
			err:  errors.New("barcodespider API returned status code: 500: server error"),
			want: TestConnectionResponse{OK: false, Detail: testDetailProviderError},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildTestBarcodeResult(tc.err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Test_classifyTestError pins down every bucket classifyTestError can
// produce, and — critically — that none of them ever contain the raw
// error's text (the whole point of the sanitization discipline).
func Test_classifyTestError(t *testing.T) {
	sensitive := "https://user:s3cr3t-api-key@internal.example.com/v1/messages"

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "context deadline exceeded (our own 30s timeout) -> connection failed",
			err:  fmt.Errorf("provider call: %w", context.DeadlineExceeded),
			want: testDetailConnectionFailed,
		},
		{
			name: "url.Error (client.Do network failure) -> connection failed",
			err: &url.Error{
				Op:  "Post",
				URL: sensitive,
				Err: errors.New("connection refused"),
			},
			want: testDetailConnectionFailed,
		},
		{
			name: "wrapped url.Error -> connection failed (unwraps through fmt.Errorf)",
			err: fmt.Errorf("vision provider request failed: %w", &url.Error{
				Op:  "Post",
				URL: sensitive,
				Err: errors.New("no such host"),
			}),
			want: testDetailConnectionFailed,
		},
		{
			name: "bare net.Error -> connection failed",
			err:  &net.DNSError{Err: "no such host", Name: "internal.example.com", IsTimeout: true},
			want: testDetailConnectionFailed,
		},
		{
			name: "401 status marker -> authentication failed",
			err:  fmt.Errorf("vision provider returned status code: 401: %s", sensitive),
			want: testDetailAuthFailed,
		},
		{
			name: "403 status marker -> authentication failed",
			err:  errors.New("barcodespider API returned status code: 403: forbidden"),
			want: testDetailAuthFailed,
		},
		{
			name: "other status code -> provider error",
			err:  errors.New("vision provider returned status code: 500: internal error"),
			want: testDetailProviderError,
		},
		{
			name: "unparseable body -> provider error",
			err:  errors.New("vision provider returned unparseable body: unexpected EOF"),
			want: testDetailProviderError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyTestError(tc.err)
			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, "s3cr3t-api-key", "classified detail must never leak the raw error text")
			assert.NotContains(t, got, "internal.example.com", "classified detail must never leak the raw error text")
		})
	}
}
