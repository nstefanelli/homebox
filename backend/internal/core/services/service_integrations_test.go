package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

// groupBaseURL is a placeholder group-configured BaseURL reused across the
// EffectiveAI table cases below.
const groupBaseURL = "http://group.local/v1"

// integrationsTestGroup creates a throwaway group for a single test so table
// cases don't stomp on each other's stored settings via the shared tGroup.
func integrationsTestGroup(t *testing.T) uuid.UUID {
	t.Helper()
	g, err := tRepos.Groups.GroupCreate(context.Background(), fk.Str(10), uuid.Nil)
	require.NoError(t, err)
	return g.ID
}

func newIntegrationsSvc(ai config.AIConf, barcode config.BarcodeAPIConf) *IntegrationsService {
	return &IntegrationsService{
		repos:           tRepos,
		fallbackAI:      ai,
		fallbackBarcode: barcode,
	}
}

func Test_IntegrationsService_EffectiveAI(t *testing.T) {
	envAI := config.AIConf{
		Provider:       AIProviderOpenAICompatible,
		BaseURL:        "http://env.local/v1",
		APIKey:         "env-api-key",
		Model:          "env-model",
		TimeoutSeconds: 120,
	}

	tests := []struct {
		name    string
		group   types.GroupIntegrations
		want    config.AIConf
		explain string
	}{
		{
			name:    "empty provider inherits env verbatim",
			group:   types.GroupIntegrations{},
			want:    envAI,
			explain: "AIProvider == \"\" must return the env AIConf byte-identical, including TimeoutSeconds",
		},
		{
			name: "disabled forces provider off but keeps env timeout",
			group: types.GroupIntegrations{
				AIProvider: AIProviderDisabled,
				AIBaseURL:  "http://should-be-ignored/v1",
				AIAPIKey:   "should-be-ignored",
				AIModel:    "should-be-ignored",
			},
			want: config.AIConf{
				Provider:       "",
				BaseURL:        "",
				APIKey:         "",
				Model:          "",
				TimeoutSeconds: envAI.TimeoutSeconds,
			},
			explain: "disabled must zero all AI fields except TimeoutSeconds, regardless of what's stored",
		},
		{
			name: "provider set with all fields populated uses group values",
			group: types.GroupIntegrations{
				AIProvider: AIProviderAnthropic,
				AIBaseURL:  groupBaseURL,
				AIAPIKey:   "group-api-key",
				AIModel:    "group-model",
			},
			want: config.AIConf{
				Provider:       AIProviderAnthropic,
				BaseURL:        groupBaseURL,
				APIKey:         "group-api-key",
				Model:          "group-model",
				TimeoutSeconds: envAI.TimeoutSeconds,
			},
			explain: "when provider set, group fields win outright",
		},
		{
			name: "provider set with empty fields falls back to env per-field",
			group: types.GroupIntegrations{
				AIProvider: AIProviderOpenAICompatible,
			},
			want: config.AIConf{
				Provider:       AIProviderOpenAICompatible,
				BaseURL:        envAI.BaseURL,
				APIKey:         envAI.APIKey,
				Model:          envAI.Model,
				TimeoutSeconds: envAI.TimeoutSeconds,
			},
			explain: "empty BaseURL/APIKey/Model must individually fall back to env, but Provider stays group's",
		},
		{
			name: "provider set with partial fields mixes group and env per-field",
			group: types.GroupIntegrations{
				AIProvider: AIProviderAnthropic,
				AIBaseURL:  groupBaseURL,
				// AIAPIKey and AIModel left empty
			},
			want: config.AIConf{
				Provider:       AIProviderAnthropic,
				BaseURL:        groupBaseURL,
				APIKey:         envAI.APIKey,
				Model:          envAI.Model,
				TimeoutSeconds: envAI.TimeoutSeconds,
			},
			explain: "per-field fallback is independent per field, not all-or-nothing",
		},
		{
			name: "timeout is always env even when provider and fields are group-set",
			group: types.GroupIntegrations{
				AIProvider: AIProviderAnthropic,
				AIBaseURL:  groupBaseURL,
				AIAPIKey:   "group-api-key",
				AIModel:    "group-model",
			},
			want: config.AIConf{
				Provider:       AIProviderAnthropic,
				BaseURL:        groupBaseURL,
				APIKey:         "group-api-key",
				Model:          "group-model",
				TimeoutSeconds: envAI.TimeoutSeconds,
			},
			explain: "TimeoutSeconds is never UI-managed; it's always the env value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gid := integrationsTestGroup(t)
			svc := newIntegrationsSvc(envAI, config.BarcodeAPIConf{})

			require.NoError(t, tRepos.Groups.IntegrationsSet(context.Background(), gid, tt.group))

			got, err := svc.EffectiveAI(context.Background(), gid)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got, tt.explain)
		})
	}
}

func Test_IntegrationsService_EffectiveBarcode(t *testing.T) {
	envBarcode := config.BarcodeAPIConf{
		TokenBarcodespider:   "env-token",
		OpenFoodFactsContact: "env@example.com",
	}

	tests := []struct {
		name  string
		group types.GroupIntegrations
		want  config.BarcodeAPIConf
	}{
		{
			name:  "both empty falls back to env for both fields",
			group: types.GroupIntegrations{},
			want:  envBarcode,
		},
		{
			name: "group token set, contact empty falls back",
			group: types.GroupIntegrations{
				BarcodeTokenBarcodespider: "group-token",
			},
			want: config.BarcodeAPIConf{
				TokenBarcodespider:   "group-token",
				OpenFoodFactsContact: envBarcode.OpenFoodFactsContact,
			},
		},
		{
			name: "group contact set, token empty falls back",
			group: types.GroupIntegrations{
				OpenFoodFactsContact: "group@example.com",
			},
			want: config.BarcodeAPIConf{
				TokenBarcodespider:   envBarcode.TokenBarcodespider,
				OpenFoodFactsContact: "group@example.com",
			},
		},
		{
			name: "both set uses group values",
			group: types.GroupIntegrations{
				BarcodeTokenBarcodespider: "group-token",
				OpenFoodFactsContact:      "group@example.com",
			},
			want: config.BarcodeAPIConf{
				TokenBarcodespider:   "group-token",
				OpenFoodFactsContact: "group@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gid := integrationsTestGroup(t)
			svc := newIntegrationsSvc(config.AIConf{}, envBarcode)

			require.NoError(t, tRepos.Groups.IntegrationsSet(context.Background(), gid, tt.group))

			got, err := svc.EffectiveBarcode(context.Background(), gid)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_IntegrationsService_Raw(t *testing.T) {
	gid := integrationsTestGroup(t)
	svc := newIntegrationsSvc(config.AIConf{}, config.BarcodeAPIConf{})

	stored := types.GroupIntegrations{
		AIProvider: AIProviderAnthropic,
		AIAPIKey:   "secret",
	}
	require.NoError(t, tRepos.Groups.IntegrationsSet(context.Background(), gid, stored))

	got, err := svc.Raw(context.Background(), gid)
	require.NoError(t, err)
	assert.Equal(t, stored, got)
}

func Test_IntegrationsService_Update_ProviderValidation(t *testing.T) {
	valid := []string{"", AIProviderDisabled, AIProviderOpenAICompatible, AIProviderAnthropic}
	for _, provider := range valid {
		t.Run("valid_"+provider, func(t *testing.T) {
			gid := integrationsTestGroup(t)
			svc := newIntegrationsSvc(config.AIConf{}, config.BarcodeAPIConf{})
			err := svc.Update(context.Background(), gid, types.GroupIntegrations{AIProvider: provider})
			assert.NoError(t, err)
		})
	}

	invalid := []string{"openai", "ANTHROPIC", "azure_openai", "not-a-real-provider"}
	for _, provider := range invalid {
		t.Run("invalid_"+provider, func(t *testing.T) {
			gid := integrationsTestGroup(t)
			svc := newIntegrationsSvc(config.AIConf{}, config.BarcodeAPIConf{})
			err := svc.Update(context.Background(), gid, types.GroupIntegrations{AIProvider: provider})
			assert.Error(t, err)
		})
	}
}

func Test_IntegrationsService_Update_SecretSemantics(t *testing.T) {
	type testCase struct {
		name     string
		stored   string
		incoming string
		want     string
	}

	// Shared table for both secret fields — the rules are identical for
	// aiApiKey and barcodeTokenBarcodespider.
	cases := []testCase{
		{
			name:     "redacted sentinel keeps stored value",
			stored:   "existing-secret",
			incoming: config.RedactedValue,
			want:     "existing-secret",
		},
		{
			name:     "empty string clears stored value",
			stored:   "existing-secret",
			incoming: "",
			want:     "",
		},
		{
			name:     "any other value replaces stored value",
			stored:   "existing-secret",
			incoming: "new-secret",
			want:     "new-secret",
		},
		{
			name:     "redacted sentinel with nothing stored keeps empty",
			stored:   "",
			incoming: config.RedactedValue,
			want:     "",
		},
		{
			name:     "replace when nothing was stored before",
			stored:   "",
			incoming: "brand-new-secret",
			want:     "brand-new-secret",
		},
	}

	t.Run("aiApiKey", func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				gid := integrationsTestGroup(t)
				svc := newIntegrationsSvc(config.AIConf{}, config.BarcodeAPIConf{})

				require.NoError(t, tRepos.Groups.IntegrationsSet(context.Background(), gid, types.GroupIntegrations{
					AIProvider: AIProviderAnthropic,
					AIAPIKey:   tc.stored,
				}))

				err := svc.Update(context.Background(), gid, types.GroupIntegrations{
					AIProvider: AIProviderAnthropic,
					AIAPIKey:   tc.incoming,
				})
				require.NoError(t, err)

				got, err := tRepos.Groups.IntegrationsGet(context.Background(), gid)
				require.NoError(t, err)
				assert.Equal(t, tc.want, got.AIAPIKey)
			})
		}
	})

	t.Run("barcodeTokenBarcodespider", func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				gid := integrationsTestGroup(t)
				svc := newIntegrationsSvc(config.AIConf{}, config.BarcodeAPIConf{})

				require.NoError(t, tRepos.Groups.IntegrationsSet(context.Background(), gid, types.GroupIntegrations{
					BarcodeTokenBarcodespider: tc.stored,
				}))

				err := svc.Update(context.Background(), gid, types.GroupIntegrations{
					BarcodeTokenBarcodespider: tc.incoming,
				})
				require.NoError(t, err)

				got, err := tRepos.Groups.IntegrationsGet(context.Background(), gid)
				require.NoError(t, err)
				assert.Equal(t, tc.want, got.BarcodeTokenBarcodespider)
			})
		}
	})

	// Both secrets are merged in the same Update call, independently.
	t.Run("both secrets merged independently in one call", func(t *testing.T) {
		gid := integrationsTestGroup(t)
		svc := newIntegrationsSvc(config.AIConf{}, config.BarcodeAPIConf{})

		require.NoError(t, tRepos.Groups.IntegrationsSet(context.Background(), gid, types.GroupIntegrations{
			AIProvider:                AIProviderAnthropic,
			AIAPIKey:                  "old-ai-key",
			BarcodeTokenBarcodespider: "old-bs-token",
		}))

		err := svc.Update(context.Background(), gid, types.GroupIntegrations{
			AIProvider:                AIProviderAnthropic,
			AIAPIKey:                  config.RedactedValue, // keep
			BarcodeTokenBarcodespider: "",                   // clear
		})
		require.NoError(t, err)

		got, err := tRepos.Groups.IntegrationsGet(context.Background(), gid)
		require.NoError(t, err)
		assert.Equal(t, "old-ai-key", got.AIAPIKey)
		assert.Empty(t, got.BarcodeTokenBarcodespider)
	})
}

func Test_IntegrationsService_Update_NonSecretFieldsOverwritePlainly(t *testing.T) {
	gid := integrationsTestGroup(t)
	svc := newIntegrationsSvc(config.AIConf{}, config.BarcodeAPIConf{})

	require.NoError(t, tRepos.Groups.IntegrationsSet(context.Background(), gid, types.GroupIntegrations{
		AIProvider:           AIProviderAnthropic,
		AIBaseURL:            "http://old.local/v1",
		AIModel:              "old-model",
		OpenFoodFactsContact: "old@example.com",
	}))

	err := svc.Update(context.Background(), gid, types.GroupIntegrations{
		AIProvider:           AIProviderOpenAICompatible,
		AIBaseURL:            "http://new.local/v1",
		AIModel:              "new-model",
		OpenFoodFactsContact: "new@example.com",
	})
	require.NoError(t, err)

	got, err := tRepos.Groups.IntegrationsGet(context.Background(), gid)
	require.NoError(t, err)
	assert.Equal(t, AIProviderOpenAICompatible, got.AIProvider)
	assert.Equal(t, "http://new.local/v1", got.AIBaseURL)
	assert.Equal(t, "new-model", got.AIModel)
	assert.Equal(t, "new@example.com", got.OpenFoodFactsContact)
}

func Test_IntegrationsService_WiredIntoAllServices(t *testing.T) {
	require.NotNil(t, tSvc.Integrations)
}
