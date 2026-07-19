package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func TestHandleEntityAttachmentExternalCreateRejectsUnsafeURL(t *testing.T) {
	for _, raw := range []string{
		"javascript:alert(document.domain)",
		"https://user:secret@example.com/document",
		"//example.com/document",
	} {
		t.Run(raw, func(t *testing.T) {
			body, err := json.Marshal(externalAttachmentRequest{
				SourceType: "link",
				ExternalID: raw,
				Title:      "Unsafe",
			})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/entities/id/attachments/external", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			routeCtx := chi.NewRouteContext()
			routeCtx.URLParams.Add("id", uuid.NewString())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

			err = (&V1Controller{}).HandleEntityAttachmentExternalCreate()(httptest.NewRecorder(), req)
			var requestErr *validate.RequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, http.StatusBadRequest, requestErr.Status)
		})
	}
}
