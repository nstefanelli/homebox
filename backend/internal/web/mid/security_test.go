package mid

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityHeadersUsesStandardCrossOriginHeaderNames(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	SecurityHeaders()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(recorder, request)

	assert.Equal(t, "require-corp", recorder.Header().Get("Cross-Origin-Embedder-Policy"))
	assert.Equal(t, "same-origin", recorder.Header().Get("Cross-Origin-Opener-Policy"))
	assert.Equal(t, "same-site", recorder.Header().Get("Cross-Origin-Resource-Policy"))
	assert.Empty(t, recorder.Header().Get("Content-Origin-Embedder-Policy"))
}
