package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func TestDecodeWipeInventoryOptions(t *testing.T) {
	t.Parallel()

	t.Run("empty body uses conservative defaults", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		got, err := decodeWipeInventoryOptions(req)
		require.NoError(t, err)
		assert.Equal(t, WipeInventoryOptions{}, got)
	})

	t.Run("valid body is honored", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"wipeTags":true}`))
		got, err := decodeWipeInventoryOptions(req)
		require.NoError(t, err)
		assert.True(t, got.WipeTags)
	})

	t.Run("malformed body is rejected", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"wipeTags":`))
		_, err := decodeWipeInventoryOptions(req)
		var requestErr *validate.RequestError
		require.ErrorAs(t, err, &requestErr)
		assert.Equal(t, http.StatusBadRequest, requestErr.Status)
	})
}
