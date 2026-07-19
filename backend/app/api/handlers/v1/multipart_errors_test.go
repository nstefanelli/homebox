package v1

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func requestErrorStatus(t *testing.T, err error) int {
	t.Helper()
	var requestErr *validate.RequestError
	require.ErrorAs(t, err, &requestErr)
	return requestErr.Status
}

func TestMultipartErrorClassification(t *testing.T) {
	assert.Equal(t, http.StatusBadRequest,
		requestErrorStatus(t, multipartParseRequestError(errors.New("bad boundary"))))
	assert.Equal(t, http.StatusRequestEntityTooLarge,
		requestErrorStatus(t, multipartParseRequestError(fmt.Errorf("parse: %w", &http.MaxBytesError{Limit: 10}))))
	assert.Equal(t, http.StatusRequestEntityTooLarge,
		requestErrorStatus(t, multipartParseRequestError(multipart.ErrMessageTooLarge)))

	assert.Equal(t, http.StatusBadRequest,
		requestErrorStatus(t, multipartFileRequestError(http.ErrMissingFile, "csv")))
	assert.Equal(t, http.StatusInternalServerError,
		requestErrorStatus(t, multipartFileRequestError(errors.New("temporary file disappeared"), "csv")))
	assert.Equal(t, http.StatusInternalServerError,
		requestErrorStatus(t, multipartContentReadError(errors.New("disk read failed"), "csv")))
}

func TestHandleEntitiesImportClassifiesMultipartFailures(t *testing.T) {
	ctrl := &V1Controller{maxUploadSize: 1}
	handler := ctrl.HandleEntitiesImport()

	t.Run("malformed multipart is 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/entities/import", strings.NewReader("not multipart"))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
		err := handler(httptest.NewRecorder(), req)
		assert.Equal(t, http.StatusBadRequest, requestErrorStatus(t, err))
	})

	t.Run("missing csv field is 400", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.Close())
		req := httptest.NewRequest(http.MethodPost, "/v1/entities/import", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		err := handler(httptest.NewRecorder(), req)
		assert.Equal(t, http.StatusBadRequest, requestErrorStatus(t, err))
	})

	t.Run("body limit overflow is 413", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("csv", "items.csv")
		require.NoError(t, err)
		_, err = part.Write(bytes.Repeat([]byte("x"), 1024))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/entities/import", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Body = http.MaxBytesReader(rec, req.Body, 128)
		err = handler(rec, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, requestErrorStatus(t, err))
	})
}
