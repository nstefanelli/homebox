package validate

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequestErrorNeverStoresNilCause(t *testing.T) {
	t.Parallel()

	err := NewRequestError(nil, http.StatusForbidden)
	var requestErr *RequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, http.StatusForbidden, requestErr.Status)
	assert.NotNil(t, requestErr.Err)
	assert.NotPanics(t, func() { _ = requestErr.Error() })
}

func TestNilRequestErrorReceiverIsSafe(t *testing.T) {
	t.Parallel()

	var requestErr *RequestError
	assert.Equal(t, "request failed", requestErr.Error())
	assert.NoError(t, requestErr.Unwrap())
}
