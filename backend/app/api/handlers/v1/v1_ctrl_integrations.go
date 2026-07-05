package v1

import (
	"errors"
	"net/http"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"github.com/sysadminsmedia/homebox/backend/internal/web/adapters"
)

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
// stored settings plus the already-resolved effective AI/barcode config.
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
func redactIntegrations(raw types.GroupIntegrations, effectiveAI config.AIConf, effectiveBarcode config.BarcodeAPIConf, isOwner bool) GroupIntegrationsOut {
	redacted := raw
	redacted.AIAPIKey = redactSecretField(raw.AIAPIKey, effectiveAI.APIKey)
	redacted.BarcodeTokenBarcodespider = redactSecretField(raw.BarcodeTokenBarcodespider, effectiveBarcode.TokenBarcodespider)

	return GroupIntegrationsOut{
		GroupIntegrations:       redacted,
		IsOwner:                 isOwner,
		AIConfigured:            effectiveAI.Provider != "",
		BarcodespiderConfigured: effectiveBarcode.TokenBarcodespider != "",
		EnvAIProvider:           effectiveAI.Provider,
		EnvAIBaseURL:            effectiveAI.BaseURL,
		EnvAIModel:              effectiveAI.Model,
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

		return redactIntegrations(raw, effectiveAI, effectiveBarcode, isOwner), nil
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

		return redactIntegrations(raw, effectiveAI, effectiveBarcode, isOwner), nil
	}

	return adapters.Action(fn, http.StatusOK)
}
