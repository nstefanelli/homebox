package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionCookiesAreHostOnly(t *testing.T) {
	controller := &V1Controller{cookieSecure: true}
	recorder := httptest.NewRecorder()

	controller.setCookies(recorder, "session-token", time.Now().Add(time.Hour), true, "attachment-token")

	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 4)
	for _, cookie := range cookies {
		assert.Empty(t, cookie.Domain)
		assert.True(t, cookie.Secure)
		assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	}
	assert.True(t, cookies[0].HttpOnly)
	assert.True(t, cookies[1].HttpOnly)
}
