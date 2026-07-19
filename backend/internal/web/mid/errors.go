package mid

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

type ErrorResponse struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields,omitempty"`
}

func Errors(log zerolog.Logger) errchain.ErrorHandler {
	return func(h errchain.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := h.ServeHTTP(w, r)
			if err != nil {
				var resp ErrorResponse
				var code int

				traceID, _ := r.Context().Value(middleware.RequestIDKey).(string)
				log.Err(err).
					Ctx(r.Context()).
					Stack().
					Str("req_id", traceID).
					Msg("ERROR occurred")

				switch {
				case errors.As(err, new(*http.MaxBytesError)):
					code = http.StatusRequestEntityTooLarge
					resp = ErrorResponse{
						Error: "request body too large",
					}
				case validate.IsUnauthorizedError(err):
					code = http.StatusUnauthorized
					resp = ErrorResponse{
						Error: "unauthorized",
					}
				case validate.IsInvalidRouteKeyError(err):
					code = http.StatusBadRequest
					resp = ErrorResponse{
						Error: err.Error(),
					}
				case validate.IsFieldError(err):
					code = http.StatusUnprocessableEntity

					var fieldErrors validate.FieldErrors
					errors.As(err, &fieldErrors) // nolint
					resp.Error = "Validation Error"
					resp.Fields = map[string]string{}

					for _, fieldError := range fieldErrors {
						resp.Fields[fieldError.Field] = fieldError.Error
					}
				case validate.IsRequestError(err):
					var requestError *validate.RequestError
					errors.As(err, &requestError) // nolint

					if requestError.Status == 0 {
						code = http.StatusBadRequest
					} else {
						code = requestError.Status
					}
					// Expected 4xx failures may contain useful validation detail.
					// Never serialize the wrapped cause of a 5xx response: those
					// causes commonly contain SQL, filesystem, or upstream details
					// that belong only in server logs.
					if code >= http.StatusInternalServerError {
						resp.Error = http.StatusText(code)
					} else {
						resp.Error = requestError.Error()
					}
				case ent.IsNotFound(err):
					resp.Error = "Not Found"
					code = http.StatusNotFound
				default:
					resp.Error = "Unknown Error"
					code = http.StatusInternalServerError
				}

				if err := server.JSON(w, code, resp); err != nil {
					log.Err(err).Ctx(r.Context()).Msg("failed to write response")
				}
			}
		})
	}
}
