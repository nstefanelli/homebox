package providers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"golang.org/x/oauth2"
)

func TestOIDCBaseURLUsesOnlyValidatedTrustedSources(t *testing.T) {
	tests := []struct {
		name        string
		hostname    string
		trustProxy  bool
		requestHost string
		tls         bool
		xfHost      string
		xfProto     string
		want        string
		wantErr     bool
	}{
		{name: "absolute configured origin", hostname: "https://app.example.com", requestHost: "internal:7745", want: "https://app.example.com"},
		{name: "bare configured host uses TLS", hostname: "app.example.com:8443", requestHost: "internal:7745", tls: true, want: "https://app.example.com:8443"},
		{name: "trusted first forwarded values", trustProxy: true, requestHost: "internal:7745", xfHost: "app.example.com, proxy.internal", xfProto: "https, http", want: "https://app.example.com"},
		{name: "untrusted forwarded values ignored", requestHost: "app.example.com", xfHost: "evil.example", xfProto: "https", want: "http://app.example.com"},
		{name: "bracketed IPv6", requestHost: "[::1]:7745", want: "http://[::1]:7745"},
		{name: "configured userinfo rejected", hostname: "https://user:secret@app.example.com", requestHost: "internal", wantErr: true},
		{name: "configured path rejected", hostname: "https://app.example.com/homebox", requestHost: "internal", wantErr: true},
		{name: "forwarded path rejected", trustProxy: true, requestHost: "internal", xfHost: "evil.example/path", wantErr: true},
		{name: "invalid forwarded protocol rejected", trustProxy: true, requestHost: "internal", xfHost: "app.example.com", xfProto: "javascript", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://"+tc.requestHost+"/api/v1/users/login/oidc", nil)
			request.Host = tc.requestHost
			if tc.tls {
				request.TLS = &tls.ConnectionState{}
			}
			request.Header.Set("X-Forwarded-Host", tc.xfHost)
			request.Header.Set("X-Forwarded-Proto", tc.xfProto)

			provider := &OIDCProvider{options: &config.Options{Hostname: tc.hostname, TrustProxy: tc.trustProxy}}
			got, err := provider.getBaseURL(request)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOIDCTransientCookiesAreHostOnly(t *testing.T) {
	provider := &OIDCProvider{
		config: &config.OIDCConf{
			ClientID:    "client",
			Scope:       "openid email",
			StateExpiry: time.Minute,
		},
		options:      &config.Options{Hostname: "https://app.example.com"},
		cookieSecure: true,
		endpoint:     oauth2.Endpoint{AuthURL: "https://idp.example.com/authorize"},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://app.example.com/api/v1/users/login/oidc", nil)

	_, err := provider.initiateOIDCFlow(recorder, request)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, recorder.Code)
	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 3)
	for _, cookie := range cookies {
		assert.Empty(t, cookie.Domain)
		assert.True(t, cookie.HttpOnly)
		assert.True(t, cookie.Secure)
		assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	}
}

func TestOIDCSecurityClaimsComeOnlyFromVerifiedIDToken(t *testing.T) {
	idClaims := map[string]interface{}{
		"sub":   "verified-subject",
		"iss":   "https://issuer.example.com",
		"nonce": "verified-nonce",
		"email": "id-token@example.com",
	}
	userInfo := map[string]interface{}{
		"sub":   "verified-subject",
		"iss":   "https://attacker.example.com",
		"nonce": "attacker-controlled-nonce",
		"email": "fresh-profile@example.com",
	}

	require.NoError(t, verifyOIDCNonce(idClaims, "verified-nonce"))
	require.Error(t, verifyOIDCNonce(idClaims, "attacker-controlled-nonce"))

	merged, err := mergeVerifiedOIDCClaims(idClaims, userInfo)
	require.NoError(t, err)
	assert.Equal(t, "verified-subject", merged["sub"])
	assert.Equal(t, "https://issuer.example.com", merged["iss"])
	assert.Equal(t, "verified-nonce", merged["nonce"])
	assert.Equal(t, "fresh-profile@example.com", merged["email"])

	userInfo["sub"] = "different-subject"
	_, err = mergeVerifiedOIDCClaims(idClaims, userInfo)
	require.Error(t, err)
}
