package v1

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

// multipartParseRequestError classifies failures produced while parsing an
// upload request. MaxBytesError can be wrapped by ParseMultipartForm, so use
// errors.As rather than a direct type assertion.
func multipartParseRequestError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) || errors.Is(err, multipart.ErrMessageTooLarge) {
		return validate.NewRequestError(errors.New("request body too large"), http.StatusRequestEntityTooLarge)
	}
	return validate.NewRequestError(errors.New("malformed multipart request"), http.StatusBadRequest)
}

// multipartFileRequestError distinguishes a missing required field (a client
// error) from a failure opening a parsed multipart temp file (a server I/O
// error). Internal paths and implementation details are logged, not returned.
func multipartFileRequestError(err error, field string) error {
	if errors.Is(err, http.ErrMissingFile) {
		return validate.NewRequestError(errors.New(field+" file is required"), http.StatusBadRequest)
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) || errors.Is(err, multipart.ErrMessageTooLarge) {
		return validate.NewRequestError(errors.New("request body too large"), http.StatusRequestEntityTooLarge)
	}
	log.Error().Err(err).Str("field", field).Msg("failed to open multipart upload")
	return validate.NewRequestError(errors.New("failed to read uploaded file"), http.StatusInternalServerError)
}

func multipartContentReadError(err error, field string) error {
	log.Error().Err(err).Str("field", field).Msg("failed to read multipart upload")
	return validate.NewRequestError(errors.New("failed to read uploaded file"), http.StatusInternalServerError)
}
