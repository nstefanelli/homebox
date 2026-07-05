package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
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
			name: "group-stored secrets redact to sentinel, non-secrets pass through plain",
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
				EnvAIProvider:           "anthropic",
				EnvAIBaseURL:            "https://api.anthropic.com",
				EnvAIModel:              "claude",
			},
		},
		{
			name: "env-fallback-only secrets still redact to sentinel even though the group row is empty",
			raw:  types.GroupIntegrations{}, // group has never set anything
			effectiveAI: config.AIConf{
				Provider: "openai_compatible",
				BaseURL:  "http://env.local/v1",
				APIKey:   "env-api-key", // sourced purely from env fallback
				Model:    "env-model",
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
				EnvAIProvider:           "openai_compatible",
				EnvAIBaseURL:            "http://env.local/v1",
				EnvAIModel:              "env-model",
			},
		},
		{
			name: "disabled provider: AIConfigured false even though a group secret is still stored",
			raw: types.GroupIntegrations{
				AIProvider: "disabled",
				AIAPIKey:   "leftover-key-from-before-disabling",
			},
			effectiveAI: config.AIConf{
				// EffectiveAI zeroes Provider/BaseURL/APIKey/Model when disabled.
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
			got := redactIntegrations(tc.raw, tc.effectiveAI, tc.effectiveBarcode, tc.isOwner)
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
