package mid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func TestErrorsMapsWrappedMaxBytesErrorTo413(t *testing.T) {
	handler := Errors(zerolog.Nop())(errchain.HandlerFunc(func(http.ResponseWriter, *http.Request) error {
		return validate.NewRequestError(&http.MaxBytesError{Limit: 1024}, http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.RequestIDKey, "test-request"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.JSONEq(t, `{"error":"request body too large"}`, rec.Body.String())
}
