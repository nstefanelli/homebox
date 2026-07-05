package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

// ErrInvalidAIProvider is returned by Update when the incoming AIProvider is
// not one of validAIProviders. Exported (and wrapped, not just formatted) so
// callers such as the v1 handler can distinguish this validation failure from
// other errors via errors.Is and map it to a 400 instead of a 500.
var ErrInvalidAIProvider = errors.New("invalid AI provider")

// AI provider identifiers accepted in GroupIntegrations.AIProvider. Exported
// so callers outside this package (handlers, tests) share one spelling rather
// than repeating the literals.
const (
	AIProviderDisabled         = "disabled"
	AIProviderOpenAICompatible = "openai_compatible"
	AIProviderAnthropic        = "anthropic"
)

// validAIProviders are the only values IntegrationsService.Update accepts for
// GroupIntegrations.AIProvider. "" inherits env, AIProviderDisabled forces AI
// off regardless of env, and the rest select a concrete provider
// implementation.
var validAIProviders = map[string]bool{
	"":                         true,
	AIProviderDisabled:         true,
	AIProviderOpenAICompatible: true,
	AIProviderAnthropic:        true,
}

// IntegrationsService resolves the effective AI/barcode integration config for
// a group — group-stored settings override the server's env-configured
// defaults on a per-field basis — and applies write-only updates to the
// stored settings row (see Update for secret-handling semantics).
type IntegrationsService struct {
	repos           *repo.AllRepos
	fallbackAI      config.AIConf
	fallbackBarcode config.BarcodeAPIConf
}

// EffectiveAI merges the group's stored AI settings over the env-configured
// fallback. Rules:
//   - AIProvider == ""         -> the env AIConf is returned verbatim.
//   - AIProvider == "disabled" -> AI is forced off (Provider/BaseURL/APIKey/Model
//     all zeroed) but TimeoutSeconds still comes from env.
//   - AIProvider == <anything else> -> group fields win; any of BaseURL/APIKey/Model
//     left empty falls back to the corresponding env value, independently per field.
//
// TimeoutSeconds is never UI-managed — it always comes from the env fallback.
func (svc *IntegrationsService) EffectiveAI(ctx context.Context, gid uuid.UUID) (config.AIConf, error) {
	group, err := svc.repos.Groups.IntegrationsGet(ctx, gid)
	if err != nil {
		return config.AIConf{}, err
	}

	switch group.AIProvider {
	case "":
		return svc.fallbackAI, nil
	case AIProviderDisabled:
		return config.AIConf{
			TimeoutSeconds: svc.fallbackAI.TimeoutSeconds,
		}, nil
	default:
		return config.AIConf{
			Provider:       group.AIProvider,
			BaseURL:        firstNonEmpty(group.AIBaseURL, svc.fallbackAI.BaseURL),
			APIKey:         firstNonEmpty(group.AIAPIKey, svc.fallbackAI.APIKey),
			Model:          firstNonEmpty(group.AIModel, svc.fallbackAI.Model),
			TimeoutSeconds: svc.fallbackAI.TimeoutSeconds,
		}, nil
	}
}

// EffectiveBarcode merges the group's stored barcode settings over the
// env-configured fallback, per-field (empty group field -> env value). There
// is no disable sentinel here — an absent token simply means that lookup lane
// is skipped by callers, same as today.
func (svc *IntegrationsService) EffectiveBarcode(ctx context.Context, gid uuid.UUID) (config.BarcodeAPIConf, error) {
	group, err := svc.repos.Groups.IntegrationsGet(ctx, gid)
	if err != nil {
		return config.BarcodeAPIConf{}, err
	}

	return config.BarcodeAPIConf{
		TokenBarcodespider:   firstNonEmpty(group.BarcodeTokenBarcodespider, svc.fallbackBarcode.TokenBarcodespider),
		OpenFoodFactsContact: firstNonEmpty(group.OpenFoodFactsContact, svc.fallbackBarcode.OpenFoodFactsContact),
	}, nil
}

// Raw returns the group's stored integration settings exactly as persisted
// (secrets included, in plaintext) for the GET/PUT handlers to redact and
// merge respectively.
func (svc *IntegrationsService) Raw(ctx context.Context, gid uuid.UUID) (types.GroupIntegrations, error) {
	return svc.repos.Groups.IntegrationsGet(ctx, gid)
}

// Update applies a write-only merge of incoming onto the group's stored
// integration settings.
//
// AIProvider is validated against validAIProviders; an unrecognized value is
// rejected rather than silently stored.
//
// The two secret fields (AIAPIKey, BarcodeTokenBarcodespider) use three-way
// semantics against the currently stored row, since the frontend never
// receives plaintext secrets back and so cannot round-trip them normally:
//   - incoming == config.RedactedValue -> keep the currently stored secret
//     (the user didn't touch the field; the UI echoed back the sentinel).
//   - incoming == ""                   -> clear the stored secret.
//   - incoming == <anything else>      -> replace the stored secret.
//
// All other fields are overwritten directly from incoming (no merge).
func (svc *IntegrationsService) Update(ctx context.Context, gid uuid.UUID, incoming types.GroupIntegrations) error {
	if !validAIProviders[incoming.AIProvider] {
		return fmt.Errorf("%w: %q", ErrInvalidAIProvider, incoming.AIProvider)
	}

	stored, err := svc.repos.Groups.IntegrationsGet(ctx, gid)
	if err != nil {
		return err
	}

	merged := incoming
	merged.AIAPIKey = mergeSecret(incoming.AIAPIKey, stored.AIAPIKey)
	merged.BarcodeTokenBarcodespider = mergeSecret(incoming.BarcodeTokenBarcodespider, stored.BarcodeTokenBarcodespider)

	return svc.repos.Groups.IntegrationsSet(ctx, gid, merged)
}

// mergeSecret applies the write-only secret semantics described on Update for
// a single field.
func mergeSecret(incoming, stored string) string {
	if incoming == config.RedactedValue {
		return stored
	}
	// "" (clear) and any other literal (replace) both just take incoming as-is.
	return incoming
}

// firstNonEmpty returns primary unless it's empty, in which case it returns
// fallback. Used throughout the per-field group-over-env resolution above.
func firstNonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
