package validate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

const maxNotifierRedirects = 10

type outboundResolver func(context.Context, string) ([]net.IP, error)
type outboundDialer func(context.Context, string, string) (net.Conn, error)

func defaultOutboundResolver(ctx context.Context, host string) ([]net.IP, error) {
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(resolved))
	for _, addr := range resolved {
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

func strictOutboundNetworkPolicy() *config.NotifierConf {
	return &config.NotifierConf{
		BlockLocalhost:     true,
		BlockLocalNets:     true,
		BlockBogonNets:     true,
		BlockCloudMetadata: true,
		Dns64Nets:          []string{"64:ff9b::/96", "64:ff9b:1::/48"},
	}
}

// InstallNotifierHTTPGuard installs the client used by shoutrrr's generic HTTP
// notifier. Redirect targets are revalidated, and DNS is resolved at dial time so
// the connection is pinned to an address that passed the notifier network policy.
func InstallNotifierHTTPGuard(cfg *config.NotifierConf) {
	http.DefaultClient = NewNotifierHTTPClient(cfg)
}

// NewNotifierHTTPClient returns an outbound client protected against redirect
// policy bypasses and DNS rebinding. Proxying is disabled because a proxy would
// resolve the target independently after Homebox validates it.
func NewNotifierHTTPClient(cfg *config.NotifierConf) *http.Client {
	dialer := &net.Dialer{}
	return newPolicyHTTPClient(cfg, 0, defaultOutboundResolver, dialer.DialContext)
}

// NewRestrictedHTTPClient returns a proxy-disabled, DNS-pinned client that
// permits only public HTTP(S) destinations. It is used for endpoints supplied
// by non-administrator tenants, where localhost, private/bogon ranges, cloud
// metadata, DNS64 embeddings, redirects, and DNS rebinding must fail closed.
func NewRestrictedHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{}
	return newPolicyHTTPClient(strictOutboundNetworkPolicy(), timeout, defaultOutboundResolver, dialer.DialContext)
}

func newPolicyHTTPClient(
	cfg *config.NotifierConf,
	timeout time.Duration,
	resolve outboundResolver,
	dial outboundDialer,
) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = notifierDialContextWith(cfg, resolve, dial)

	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: notifierRedirectGuardWithResolver(cfg, resolve),
	}
}

func notifierDialContext(cfg *config.NotifierConf) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{}
	return notifierDialContextWith(cfg, defaultOutboundResolver, dialer.DialContext)
}

func notifierDialContextWith(
	cfg *config.NotifierConf,
	resolve outboundResolver,
	dial outboundDialer,
) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid notifier dial address: %w", err)
		}

		ips, err := resolve(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve notifier hostname: %w", err)
		}
		if err := validateNotifierIPs(ips, cfg); err != nil {
			return nil, fmt.Errorf("notifier destination blocked by network policy: %w", err)
		}

		var dialErrs []error
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErrs = append(dialErrs, err)
		}
		return nil, fmt.Errorf("failed to connect to notifier destination: %w", errors.Join(dialErrs...))
	}
}

// NotifierRedirectGuard rejects redirect targets that do not pass the same
// network policy as the original notifier URL.
func NotifierRedirectGuard(cfg *config.NotifierConf) func(req *http.Request, via []*http.Request) error {
	return notifierRedirectGuardWithResolver(cfg, defaultOutboundResolver)
}

func notifierRedirectGuardWithResolver(
	cfg *config.NotifierConf,
	resolve outboundResolver,
) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxNotifierRedirects {
			return fmt.Errorf("stopped after %d redirects", maxNotifierRedirects)
		}
		if cfg == nil {
			return fmt.Errorf("notifier configuration is nil")
		}
		if req.URL == nil {
			return fmt.Errorf("notifier redirect has no URL")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("notifier redirect scheme %q is not allowed", req.URL.Scheme)
		}
		ips, err := resolve(req.Context(), req.URL.Hostname())
		if err != nil {
			return fmt.Errorf("failed to resolve notifier redirect hostname: %w", err)
		}
		if err := validateNotifierIPs(ips, cfg); err != nil {
			return fmt.Errorf("redirect destination blocked by notifier network policy: %w", err)
		}
		return nil
	}
}
