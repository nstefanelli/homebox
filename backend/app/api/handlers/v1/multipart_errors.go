package v1

import (
	"errors"
	"math"
	"mime/multipart"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

const bytesPerMiB int64 = 1 << 20

func megabytesToBytes(megabytes int64) int64 {
	if megabytes <= 0 {
		return 0
	}
	if megabytes > math.MaxInt64/bytesPerMiB {
		return math.MaxInt64
	}
	return megabytes * bytesPerMiB
}

// multipartRequestLimit includes one MiB for multipart boundaries and
// metadata, matching the global middleware while also making each handler
// safe when mounted independently in tests or another server.
func multipartRequestLimit(maxContentMB int64) int64 {
	if maxContentMB == math.MaxInt64 {
		return math.MaxInt64
	}
	return megabytesToBytes(maxContentMB + 1)
}

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
