package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

// ErrInvalidAIProvider is returned by Update when the incoming AIProvider is
// not one of validAIProviders. Exported (and wrapped, not just formatted) so
// callers such as the v1 handler can distinguish this validation failure from
// other errors via errors.Is and map it to a 400 instead of a 500.
var (
	ErrInvalidAIProvider      = errors.New("invalid AI provider")
	ErrInvalidAIBaseURL       = errors.New("invalid AI base URL")
	ErrInvalidIntegrationData = errors.New("invalid integration field")
)

const (
	maxIntegrationBaseURLBytes = 2048
	maxIntegrationModelBytes   = 255
	maxIntegrationContactBytes = 512
	maxIntegrationSecretBytes  = 8192
)

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

// GroupIntegrationsStore is the narrow persistence seam IntegrationsService
// needs — just the two group-integrations methods off repo.AllRepos.Groups,
// rather than the whole *repo.AllRepos. Exported so callers that can't build
// a full ent-backed AllRepos (e.g. the v1 handlers package, which has no DB
// test fixture) can substitute a trivial fake satisfying just these two
// methods instead of standing up a real database for a handler unit test.
// *repo.GroupRepository satisfies this interface today.
type GroupIntegrationsStore interface {
	IntegrationsGet(ctx context.Context, gid uuid.UUID) (types.GroupIntegrations, error)
	IntegrationsSet(ctx context.Context, gid uuid.UUID, data types.GroupIntegrations) error
}

// IntegrationsService resolves the effective AI/barcode integration config for
// a group — group-stored settings override the server's env-configured
// defaults on a per-field basis — and applies write-only updates to the
// stored settings row (see Update for secret-handling semantics).
type IntegrationsService struct {
	repos           GroupIntegrationsStore
	fallbackAI      config.AIConf
	fallbackBarcode config.BarcodeAPIConf
}

// NewIntegrationsService constructs an IntegrationsService directly from a
// GroupIntegrationsStore, bypassing the full AllServices/AllRepos build path
// in New below. Exported for tests that need an IntegrationsService without a
// real database (see GroupIntegrationsStore's doc comment).
func NewIntegrationsService(store GroupIntegrationsStore, fallbackAI config.AIConf, fallbackBarcode config.BarcodeAPIConf) *IntegrationsService {
	return &IntegrationsService{
		repos:           store,
		fallbackAI:      fallbackAI,
		fallbackBarcode: fallbackBarcode,
	}
}

// EffectiveAI merges the group's stored AI settings over the env-configured
// fallback. Rules:
//   - AIProvider == ""         -> the env AIConf is returned verbatim.
//   - AIProvider == "disabled" -> AI is forced off (Provider/BaseURL/APIKey/Model
//     all zeroed) but TimeoutSeconds still comes from env.
//   - AIProvider == <anything else> -> group fields win; empty fields inherit
//     env values, except an explicit group BaseURL never inherits the env API
//     key because that would disclose an administrator secret to a tenant-
//     selected endpoint.
//
// TimeoutSeconds is never UI-managed — it always comes from the env fallback.
func (svc *IntegrationsService) EffectiveAI(ctx context.Context, gid uuid.UUID) (config.AIConf, error) {
	group, err := svc.repos.IntegrationsGet(ctx, gid)
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
		baseURL := svc.fallbackAI.BaseURL
		baseURLIsUntrusted := false
		apiKey := firstNonEmpty(group.AIAPIKey, svc.fallbackAI.APIKey)
		if group.AIBaseURL != "" {
			baseURL = group.AIBaseURL
			baseURLIsUntrusted = true
			apiKey = group.AIAPIKey
		}
		return config.AIConf{
			Provider:           group.AIProvider,
			BaseURL:            baseURL,
			APIKey:             apiKey,
			Model:              firstNonEmpty(group.AIModel, svc.fallbackAI.Model),
			TimeoutSeconds:     svc.fallbackAI.TimeoutSeconds,
			BaseURLIsUntrusted: baseURLIsUntrusted,
		}, nil
	}
}

// EffectiveBarcode merges the group's stored barcode settings over the
// env-configured fallback, per-field (empty group field -> env value). There
// is no disable sentinel here — an absent token simply means that lookup lane
// is skipped by callers, same as today.
func (svc *IntegrationsService) EffectiveBarcode(ctx context.Context, gid uuid.UUID) (config.BarcodeAPIConf, error) {
	group, err := svc.repos.IntegrationsGet(ctx, gid)
	if err != nil {
		return config.BarcodeAPIConf{}, err
	}

	return config.BarcodeAPIConf{
		TokenBarcodespider:   firstNonEmpty(group.BarcodeTokenBarcodespider, svc.fallbackBarcode.TokenBarcodespider),
		OpenFoodFactsContact: firstNonEmpty(group.OpenFoodFactsContact, svc.fallbackBarcode.OpenFoodFactsContact),
	}, nil
}

// EnvAI returns the server's env-configured AI fallback exactly as loaded at
// startup — no group override merged in. This is distinct from EffectiveAI:
// callers building a "server default" hint (design spec §5 — "what the env
// fallback provides") need the raw env values even when the calling group has
// its own override in effect, whereas EffectiveAI intentionally returns the
// group's values once it overrides. Using EffectiveAI here would relabel the
// group's own override as "server default" the moment it configures anything
// non-empty — no ctx/gid needed since this never touches the group's stored
// row.
func (svc *IntegrationsService) EnvAI() config.AIConf {
	return svc.fallbackAI
}

// Raw returns the group's stored integration settings exactly as persisted
// (secrets included, in plaintext) for the GET/PUT handlers to redact and
// merge respectively.
func (svc *IntegrationsService) Raw(ctx context.Context, gid uuid.UUID) (types.GroupIntegrations, error) {
	return svc.repos.IntegrationsGet(ctx, gid)
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
	incoming.AIBaseURL = strings.TrimSpace(incoming.AIBaseURL)
	incoming.AIModel = strings.TrimSpace(incoming.AIModel)
	incoming.OpenFoodFactsContact = strings.TrimSpace(incoming.OpenFoodFactsContact)
	if err := validateIntegrationFields(incoming); err != nil {
		return err
	}
	if err := validateAIBaseURL(incoming.AIBaseURL); err != nil {
		return err
	}

	stored, err := svc.repos.IntegrationsGet(ctx, gid)
	if err != nil {
		return err
	}

	merged := incoming
	merged.AIAPIKey = mergeSecret(incoming.AIAPIKey, stored.AIAPIKey)
	merged.BarcodeTokenBarcodespider = mergeSecret(incoming.BarcodeTokenBarcodespider, stored.BarcodeTokenBarcodespider)

	return svc.repos.IntegrationsSet(ctx, gid, merged)
}

func validateIntegrationFields(incoming types.GroupIntegrations) error {
	switch {
	case len(incoming.AIBaseURL) > maxIntegrationBaseURLBytes:
		return fmt.Errorf("%w: AI base URL exceeds %d bytes", ErrInvalidIntegrationData, maxIntegrationBaseURLBytes)
	case len(incoming.AIModel) > maxIntegrationModelBytes:
		return fmt.Errorf("%w: AI model exceeds %d bytes", ErrInvalidIntegrationData, maxIntegrationModelBytes)
	case len(incoming.OpenFoodFactsContact) > maxIntegrationContactBytes:
		return fmt.Errorf("%w: Open Food Facts contact exceeds %d bytes", ErrInvalidIntegrationData, maxIntegrationContactBytes)
	case strings.ContainsAny(incoming.OpenFoodFactsContact, "\r\n"):
		return fmt.Errorf("%w: Open Food Facts contact contains a line break", ErrInvalidIntegrationData)
	case len(incoming.AIAPIKey) > maxIntegrationSecretBytes:
		return fmt.Errorf("%w: AI API key exceeds %d bytes", ErrInvalidIntegrationData, maxIntegrationSecretBytes)
	case len(incoming.BarcodeTokenBarcodespider) > maxIntegrationSecretBytes:
		return fmt.Errorf("%w: barcode token exceeds %d bytes", ErrInvalidIntegrationData, maxIntegrationSecretBytes)
	default:
		return nil
	}
}

// validateAIBaseURL accepts only absolute HTTP(S) endpoints without embedded
// credentials. AI API keys have a dedicated write-only field; allowing
// credentials in the URL would make them indistinguishable from displayable
// endpoint metadata and could expose them through settings responses or logs.
func validateAIBaseURL(raw string) error {
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, "#") {
		return fmt.Errorf("%w: query strings and fragments are not supported", ErrInvalidAIBaseURL)
	}

	u, err := url.ParseRequestURI(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return fmt.Errorf("%w: must be an absolute http/https URL without embedded credentials", ErrInvalidAIBaseURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: scheme must be http or https", ErrInvalidAIBaseURL)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: query strings and fragments are not supported", ErrInvalidAIBaseURL)
	}

	return nil
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
