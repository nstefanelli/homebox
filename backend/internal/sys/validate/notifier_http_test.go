package validate

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

func notifierRedirectRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return &http.Request{URL: u}
}

func TestNotifierRedirectGuard(t *testing.T) {
	guard := NotifierRedirectGuard(&config.NotifierConf{BlockLocalhost: true})
	require.Error(t, guard(notifierRedirectRequest(t, "http://127.0.0.1/x"), nil))
	require.NoError(t, guard(notifierRedirectRequest(t, "https://8.8.8.8/x"), nil))
	require.Error(t, guard(notifierRedirectRequest(t, "https://8.8.8.8/x"), make([]*http.Request, maxNotifierRedirects)))
}

func TestNotifierHTTPClientBlocksResolvedDestinationAtDialTime(t *testing.T) {
	dial := notifierDialContext(&config.NotifierConf{BlockLocalhost: true})
	conn, err := dial(context.Background(), "tcp", "127.0.0.1:80")
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestRestrictedHTTPClientBlocksRedirectToPrivateDestination(t *testing.T) {
	resolve := func(_ context.Context, host string) ([]net.IP, error) {
		switch host {
		case "public.example":
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		case "private.example":
			return []net.IP{net.ParseIP("10.0.0.1")}, nil
		default:
			return nil, &net.DNSError{Name: host, Err: "not found"}
		}
	}
	guard := notifierRedirectGuardWithResolver(strictOutboundNetworkPolicy(), resolve)
	require.NoError(t, guard(notifierRedirectRequest(t, "https://public.example/v1"), nil))
	require.Error(t, guard(notifierRedirectRequest(t, "https://private.example/v1"), nil))
	require.Error(t, guard(notifierRedirectRequest(t, "file:///etc/passwd"), nil))
}

func TestRestrictedHTTPClientRevalidatesDNSAtDialTime(t *testing.T) {
	resolutions := 0
	resolve := func(context.Context, string) ([]net.IP, error) {
		resolutions++
		if resolutions == 1 {
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}

	// Redirect-time validation sees a public address.
	guard := notifierRedirectGuardWithResolver(strictOutboundNetworkPolicy(), resolve)
	require.NoError(t, guard(notifierRedirectRequest(t, "https://rebind.example/v1"), nil))

	// Dial-time resolution changes to loopback. It must be blocked before the
	// underlying dialer is invoked, closing the DNS rebinding window.
	dialCalled := false
	dial := notifierDialContextWith(
		strictOutboundNetworkPolicy(),
		resolve,
		func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, io.EOF
		},
	)
	conn, err := dial(context.Background(), "tcp", "rebind.example:443")
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err)
	assert.False(t, dialCalled)
}

func TestNewRestrictedHTTPClientDisablesProxyAndSetsTimeout(t *testing.T) {
	client := NewRestrictedHTTPClient(3 * time.Second)
	assert.Equal(t, 3*time.Second, client.Timeout)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Nil(t, transport.Proxy)
	require.NotNil(t, transport.DialContext)
	require.NotNil(t, client.CheckRedirect)
}
