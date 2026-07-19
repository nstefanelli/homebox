package v1

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog/log"

	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"github.com/sysadminsmedia/homebox/backend/pkgs/ai"
)

const aiSearchEngineName = "ai-vision"

type AnalyzePhotoResponse struct {
	Lane          string                `json:"lane"`
	Confidence    *float64              `json:"confidence,omitempty"`
	CategoryHints []string              `json:"categoryHints"`
	Products      []repo.BarcodeProduct `json:"products"`
}

// resolveAIProvider resolves the effective (group-over-env) AI config for the
// requesting group's tenant and constructs a provider from it — the runtime
// gating this whole task rewires analyze-photo/analyze-photo-bulk around
// (design spec §3). A "" effective provider (nothing configures AI, neither
// group settings nor env) now yields a 503 at request time instead of the
// route simply not existing (the old registration-time gating in routes.go).
func (ctrl *V1Controller) resolveAIProvider(r *http.Request) (ai.Provider, config.AIConf, error) {
	ctx := services.NewContext(r.Context())

	conf, err := ctrl.svc.Integrations.EffectiveAI(ctx, ctx.GID)
	if err != nil {
		return nil, config.AIConf{}, err
	}

	if conf.Provider == "" {
		return nil, config.AIConf{}, validate.NewRequestError(errors.New("ai not configured"), http.StatusServiceUnavailable)
	}

	provider, err := ctrl.aiProviderFactory(conf)
	if err != nil {
		log.Err(err).Msg("failed to construct AI provider from effective config")
		return nil, config.AIConf{}, validate.NewRequestError(errors.New("ai provider configuration error"), http.StatusInternalServerError)
	}

	return provider, conf, nil
}

// HandleAnalyzePhoto godoc
//
//	@Summary		Analyze Item Photo
//	@Description	Identifies a physical item from a photo using the configured vision AI provider
//	@Tags			Actions
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Photo of a single item (JPEG/PNG/WebP)"
//	@Success		200		{object}	AnalyzePhotoResponse
//	@Router			/v1/actions/analyze-photo [Post]
//	@Security		Bearer
func (ctrl *V1Controller) HandleAnalyzePhoto() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		provider, conf, err := ctrl.resolveAIProvider(r)
		if err != nil {
			return err
		}

		imageBytes, mimeType, err := ctrl.readPhotoUpload(w, r, conf.TimeoutSeconds)
		if err != nil {
			return err
		}

		result, err := provider.Analyze(r.Context(), imageBytes, mimeType)
		if err != nil {
			log.Err(err).Msg("vision provider analyze failed")
			return validate.NewRequestError(errors.New("vision provider error"), http.StatusBadGateway)
		}

		return server.JSON(w, http.StatusOK, analyzePhotoResponse(result, imageBytes, mimeType))
	}
}

// readPhotoUpload extends the request deadline past the global server
// timeouts (cold vision-model loads take 30-60s+), parses the multipart
// form, and returns the validated image bytes + mime type. timeoutSeconds is
// the effective (group-over-env) AI config's TimeoutSeconds for this request.
func (ctrl *V1Controller) readPhotoUpload(w http.ResponseWriter, r *http.Request, timeoutSeconds int) ([]byte, string, error) {
	// The global http.Server write/read timeouts (default 10s, see
	// internal/sys/config Web.WriteTimeout) are sized for ordinary API
	// requests. A cold vision-model load on the configured AI provider
	// can legitimately take 30-60s+ (see spec Q5), so routes using this
	// helper need a deadline that covers the provider's own timeout plus
	// margin for upload/JSON overhead — otherwise the server aborts the
	// connection mid-request long before the provider ever times out.
	deadline := time.Now().Add(time.Duration(timeoutSeconds+30) * time.Second)
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(deadline); err != nil {
		log.Warn().Err(err).Msg("failed to extend response deadline for analyze-photo")
	}
	if err := rc.SetReadDeadline(deadline); err != nil {
		log.Warn().Err(err).Msg("failed to extend response deadline for analyze-photo")
	}

	if err := r.ParseMultipartForm(ctrl.maxUploadSize << 20); err != nil {
		return nil, "", multipartParseRequestError(err)
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, "", multipartFileRequestError(err, "image")
	}
	defer func() { _ = file.Close() }()

	maxBytes := ctrl.maxUploadSize << 20
	imageBytes, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, "", multipartContentReadError(err, "image")
	}
	if int64(len(imageBytes)) > maxBytes {
		return nil, "", validate.NewRequestError(errors.New("image exceeds upload size limit"), http.StatusRequestEntityTooLarge)
	}
	mimeType := http.DetectContentType(imageBytes)
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return nil, "", validate.NewRequestError(errors.New("file is not a supported image (jpeg/png/webp)"), http.StatusBadRequest)
	}
	return imageBytes, mimeType, nil
}

type BulkItemCandidate struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Manufacturer  string   `json:"manufacturer"`
	ModelNumber   string   `json:"modelNumber"`
	Quantity      float64  `json:"quantity"`
	CategoryHints []string `json:"categoryHints"`
	Confidence    float64  `json:"confidence"`
}

type AnalyzeBulkResponse struct {
	Lane       string              `json:"lane"`
	Candidates []BulkItemCandidate `json:"candidates"`
}

// HandleAnalyzeBulk godoc
//
//	@Summary		Analyze Container Contents Photo
//	@Description	Identifies every distinct item in a photo of an open container using the configured vision AI provider
//	@Tags			Actions
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Photo of an open container/shelf (JPEG/PNG/WebP)"
//	@Success		200		{object}	AnalyzeBulkResponse
//	@Router			/v1/actions/analyze-photo-bulk [Post]
//	@Security		Bearer
func (ctrl *V1Controller) HandleAnalyzeBulk() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		provider, conf, err := ctrl.resolveAIProvider(r)
		if err != nil {
			return err
		}

		imageBytes, mimeType, err := ctrl.readPhotoUpload(w, r, conf.TimeoutSeconds)
		if err != nil {
			return err
		}

		results, err := provider.AnalyzeContents(r.Context(), imageBytes, mimeType)
		if err != nil {
			log.Err(err).Msg("vision provider bulk analyze failed")
			return validate.NewRequestError(errors.New("vision provider error"), http.StatusBadGateway)
		}

		candidates := make([]BulkItemCandidate, 0, len(results))
		for _, res := range results {
			hints := res.CategoryHints
			if hints == nil {
				hints = []string{}
			}
			candidates = append(candidates, BulkItemCandidate{
				Name: res.Name, Description: res.Description,
				Manufacturer: res.Manufacturer, ModelNumber: res.ModelNumber,
				Quantity: res.Quantity, CategoryHints: hints, Confidence: res.Confidence,
			})
		}
		return server.JSON(w, http.StatusOK, AnalyzeBulkResponse{Lane: "vision-bulk", Candidates: candidates})
	}
}

func analyzePhotoResponse(res ai.AnalyzeResult, imageBytes []byte, mimeType string) AnalyzePhotoResponse {
	dataURI := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(imageBytes)
	confidence := res.Confidence
	hints := res.CategoryHints
	if hints == nil {
		hints = []string{}
	}

	return AnalyzePhotoResponse{
		Lane:          "vision",
		Confidence:    &confidence,
		CategoryHints: hints,
		Products: []repo.BarcodeProduct{{
			SearchEngineName: aiSearchEngineName,
			ModelNumber:      res.ModelNumber,
			Manufacturer:     res.Manufacturer,
			// ImageURL mirrors ImageBase64 because CreateModal's prefill guard
			// checks imageURL truthiness before attaching imageBase64.
			ImageURL:    dataURI,
			ImageBase64: dataURI,
			Item: repo.EntityCreate{
				Name:         res.Name,
				Description:  res.Description,
				Manufacturer: res.Manufacturer,
				ModelNumber:  res.ModelNumber,
				Quantity:     1,
			},
		}},
	}
}
