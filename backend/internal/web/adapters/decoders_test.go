package adapters

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func requireRequestStatus(t *testing.T, err error, status int) {
	t.Helper()
	var requestErr *validate.RequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, status, requestErr.Status)
}

func TestDecodeBodyMapsMalformedJSONToBadRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":`))
	_, err := DecodeBody[struct {
		Name string `json:"name"`
	}](req)
	requireRequestStatus(t, err, http.StatusBadRequest)
}

func TestDecodeQueryMapsTypeErrorsToBadRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/?page=not-an-integer", nil)
	_, err := DecodeQuery[struct {
		Page int `schema:"page"`
	}](req)
	requireRequestStatus(t, err, http.StatusBadRequest)
}

type rejectingBody struct{}

func (rejectingBody) Validate() error { return errors.New("rejected") }

func TestDecodeBodyMapsCustomValidationToBadRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	_, err := DecodeBody[rejectingBody](req)
	requireRequestStatus(t, err, http.StatusBadRequest)
}
