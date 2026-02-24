package pisag

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"strings"

	"example.com/pisag_go/ports"
)

var (
	ErrRedirectNotAllowed = errors.New("pisag: redirect not allowed")
	ErrIPNotAllowed       = errors.New("pisag: ip not allowed")
)

// NewClient returns an http.Client hardened by PISAG policy.
// - DefaultTransport forbidden (we construct our own)
// - ProxyFromEnvironment forbidden (Proxy=nil)
// - DialContext required, with DNS->IP validation
// - TLS >= 1.2
// - Redirects limited, host-stable by default
func NewClient(policy ports.Policy) (*http.Client, error) {
	if policy.MaxRedirects <= 0 {
		policy.MaxRedirects = 3
	}
	if policy.Timeout <= 0 {
		policy.Timeout = 15 * time.Second
	}

	tr, err := NewTransport(policy)
	if err != nil {
		return nil, err
	}

	c := &http.Client{
		Transport: tr,
		Timeout:   policy.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= policy.MaxRedirects {
				return ErrRedirectNotAllowed
			}
			// Enforce https (again)
			if req.URL == nil || req.URL.Scheme != "https" {
				return ErrRedirectNotAllowed
			}
			// Reject host change redirects (strict for v4)
			if len(via) > 0 {
				prev := via[len(via)-1].URL
				if prev != nil && !sameHostPort(prev, req.URL) {
					return ErrRedirectNotAllowed
				}
			}
			return nil
		},
	}
	return c, nil
}

func NewTransport(policy ports.Policy) (*http.Transport, error) {
	// We intentionally do NOT use http.DefaultTransport.
	dialer := &net.Dialer{
		Timeout:   8 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Custom resolver + IP validation before connect.
	// We validate resolved IPs per connection attempt.
	dialContext := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}

		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}

		var lastErr error
		for _, ip := range ips {
			if !isAllowedIPWithPolicy(ip, policy) {
				continue
			}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}

		// If nothing matched policy, return ErrIPNotAllowed (not last dial error).
		// This is useful for tests and security debugging.
		if lastErr != nil {
			return nil, ErrIPNotAllowed
		}
		return nil, ErrIPNotAllowed
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if policy.TLSRootCAs != nil {
		tlsCfg.RootCAs = policy.TLSRootCAs
	}

	tr := &http.Transport{
		Proxy: nil, // ProxyFromEnvironment forbidden
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialContext(ctx, network, address)
		},
		ForceAttemptHTTP2:       true,
		MaxIdleConns:            64,
		IdleConnTimeout:         90 * time.Second,
		TLSHandshakeTimeout:     8 * time.Second,
		ExpectContinueTimeout:   1 * time.Second,
		TLSClientConfig:         tlsCfg,
	}
	return tr, nil
}

// RequestGuard normalizes + enforces allowlist for a URL string.
func RequestGuard(raw string, policy ports.Policy) (*url.URL, error) {
	u, err := NormalizeURL(raw)
	if err != nil {
		return nil, err
	}
	if err := IsAllowed(u, policy); err != nil {
		return nil, err
	}
	return u, nil
}

func sameHostPort(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	ah, ap := a.Hostname(), a.Port()
	bh, bp := b.Hostname(), b.Port()
	if ap == "" {
		ap = "443"
	}
	if bp == "" {
		bp = "443"
	}
	return ah == bh && ap == bp
}

// isAllowedIP blocks:
// - loopback, link-local, multicast, unspecified
// - private RFC1918 (10/8, 172.16/12, 192.168/16)
// - unique local (fc00::/7) and link-local (fe80::/10)
//
// v4 note: This is strict. If you *need* to allow specific CIDRs (like compose network),
// add an explicit allow list in v5. For v4, we assume allowlist host resolves to safe IPs
// within your controlled environment and not private metadata endpoints.
func isAllowedIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// Normalize IPv4-in-IPv6
	if v4 := ip.To4(); v4 != nil {
		ip = v4
		// RFC1918
		if v4[0] == 10 {
			return false
		}
		if v4[0] == 172 && v4[1]&0xF0 == 16 {
			return false
		}
		if v4[0] == 192 && v4[1] == 168 {
			return false
		}
		// loopback 127/8
		if v4[0] == 127 {
			return false
		}
		// link-local 169.254/16
		if v4[0] == 169 && v4[1] == 254 {
			return false
		}
		// multicast 224/4
		if v4[0] >= 224 && v4[0] <= 239 {
			return false
		}
		// unspecified 0.0.0.0
		if v4[0] == 0 && v4[1] == 0 && v4[2] == 0 && v4[3] == 0 {
			return false
		}
		return true
	}

	// IPv6 checks
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	// link-local fe80::/10
	if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return false
	}
	// unique local fc00::/7
	if (ip[0] & 0xfe) == 0xfc {
		return false
	}
	return true
}

func isAllowedIPWithPolicy(ip net.IP, policy ports.Policy) bool {
	if ip == nil {
		return false
	}

	// If explicit CIDR allowlist exists, allow ONLY if it matches,
	// but keep absolute-deny for metadata + unspecified + multicast.
	if len(policy.AllowCIDRs) > 0 {
		if isAbsoluteDenyIP(ip) {
			return false
		}
		return ipInCIDRs(ip, policy.AllowCIDRs)
	}

	// No CIDR allowlist: strict defaults (prod)
	if v4 := ip.To4(); v4 != nil {
		// absolute denies
		if isAbsoluteDenyIP(ip) {
			return false
		}
		// loopback 127/8 deny
		if v4[0] == 127 {
			return false
		}
		// RFC1918 deny
		if v4[0] == 10 {
			return false
		}
		if v4[0] == 172 && v4[1]&0xF0 == 16 {
			return false
		}
		if v4[0] == 192 && v4[1] == 168 {
			return false
		}
		return true
	}

	// IPv6 strict defaults (prod)
	if isAbsoluteDenyIP(ip) {
		return false
	}
	// loopback ::1 deny
	if ip.IsLoopback() {
		return false
	}
	// link-local fe80::/10 deny
	if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return false
	}
	// unique local fc00::/7 deny
	if (ip[0] & 0xfe) == 0xfc {
		return false
	}
	return true
}

func isAbsoluteDenyIP(ip net.IP) bool {
	if ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// metadata/link-local 169.254/16 is absolute deny
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
		// multicast 224/4
		if v4[0] >= 224 && v4[0] <= 239 {
			return true
		}
		// 0.0.0.0
		if v4[0] == 0 && v4[1] == 0 && v4[2] == 0 && v4[3] == 0 {
			return true
		}
	}
	return false
}

func ipInCIDRs(ip net.IP, cidrs []string) bool {
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil || n == nil {
			continue
		}
		if n.Contains(ip) {
			return true
		}
	}
	return false
}


// RequestGuardWithAllowlistKey normalizes + enforces allowlist for a URL string,
// and also enforces allowlist_key (fail-closed).
//
// v4 fixed rule:
// - allowlistKey must be non-empty (NULL/empty => deny)
var ErrAllowlistKeyRequired = errors.New("pisag: allowlist_key required")

func RequestGuardWithAllowlistKey(raw string, policy ports.Policy, allowlistKey string) (*url.URL, error) {
	if strings.TrimSpace(allowlistKey) == "" {
		return nil, ErrAllowlistKeyRequired
	}
	u, err := NormalizeURL(raw)
	if err != nil {
		return nil, err
	}
	if err := IsAllowed(u, policy); err != nil {
		return nil, err
	}
	return u, nil
}