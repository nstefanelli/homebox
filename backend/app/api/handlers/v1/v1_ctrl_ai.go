package v1

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog/log"

	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
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
func (ctrl *V1Controller) HandleAnalyzePhoto(provider ai.Provider) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		err := r.ParseMultipartForm(ctrl.maxUploadSize << 20)
		if err != nil {
			return validate.NewRequestError(errors.New("failed to parse multipart form"), http.StatusBadRequest)
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			return validate.NewRequestError(errors.New("no image provided"), http.StatusBadRequest)
		}
		defer func() { _ = file.Close() }()

		imageBytes, err := io.ReadAll(io.LimitReader(file, ctrl.maxUploadSize<<20))
		if err != nil {
			return validate.NewRequestError(errors.New("failed to read image"), http.StatusBadRequest)
		}

		mimeType := http.DetectContentType(imageBytes)
		switch mimeType {
		case "image/jpeg", "image/png", "image/webp":
		default:
			return validate.NewRequestError(errors.New("file is not a supported image (jpeg/png/webp)"), http.StatusBadRequest)
		}

		result, err := provider.Analyze(r.Context(), imageBytes, mimeType)
		if err != nil {
			log.Err(err).Msg("vision provider analyze failed")
			return validate.NewRequestError(errors.New("vision provider error"), http.StatusBadGateway)
		}

		return server.JSON(w, http.StatusOK, analyzePhotoResponse(result, imageBytes, mimeType))
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
