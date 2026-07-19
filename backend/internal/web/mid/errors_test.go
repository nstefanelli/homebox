package mid

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func requestWithID(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.RequestIDKey, "test-request"))
}

func TestErrorsMapsWrappedMaxBytesErrorTo413(t *testing.T) {
	handler := Errors(zerolog.Nop())(errchain.HandlerFunc(func(http.ResponseWriter, *http.Request) error {
		return validate.NewRequestError(&http.MaxBytesError{Limit: 1024}, http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	req = requestWithID(req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.JSONEq(t, `{"error":"request body too large"}`, rec.Body.String())
}

func TestErrorsDoesNotExposeInternalRequestErrorCause(t *testing.T) {
	handler := Errors(zerolog.Nop())(errchain.HandlerFunc(func(http.ResponseWriter, *http.Request) error {
		return validate.NewRequestError(errors.New("sql: no such column secret_table.internal_value"), http.StatusInternalServerError)
	}))

	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"error":"Internal Server Error"}`, rec.Body.String())
}

func TestErrorsHandlesMissingRequestID(t *testing.T) {
	handler := Errors(zerolog.Nop())(errchain.HandlerFunc(func(http.ResponseWriter, *http.Request) error {
		return errors.New("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	assert.NotPanics(t, func() { handler.ServeHTTP(rec, req) })
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
