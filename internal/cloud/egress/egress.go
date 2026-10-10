// Package egress is the cloud-only network boundary. Local Ding deliberately
// retains access to user-owned private services.
package egress

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

func PublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, prefix := range blocked {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

// URL performs offline preflight. DNS is always checked again at connection time.
func URL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("cloud watches require a public HTTP(S) URL without embedded credentials or fragments")
	}
	if port := u.Port(); port != "" && !((u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443")) {
		return fmt.Errorf("cloud beta supports standard HTTP and HTTPS ports only")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if ip, err := netip.ParseAddr(host); err == nil {
		if PublicIP(ip) {
			return nil
		}
		return fmt.Errorf("private or special-purpose addresses cannot run in Ding Cloud")
	}
	if !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || strings.ContainsAny(host, "%\\\x00") {
		return fmt.Errorf("cloud endpoint must have a public DNS name")
	}
	labels := strings.Split(host, ".")
	if len(host) > 253 || strings.Trim(labels[len(labels)-1], "0123456789") == "" {
		return fmt.Errorf("ambiguous numeric host is denied")
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("invalid public hostname")
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
				return fmt.Errorf("use an ASCII or punycode public hostname")
			}
		}
	}
	return nil
}

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type Guard struct {
	Resolver Resolver
	Dial     func(context.Context, string, string) (net.Conn, error)
}

func (g Guard) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || (port != "80" && port != "443") {
		return nil, fmt.Errorf("egress port denied")
	}
	resolver := g.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ips, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 || len(ips) > 32 {
		return nil, fmt.Errorf("public endpoint resolution failed")
	}
	// Fail the entire mixed answer rather than falling through to an internal IP.
	for _, ip := range ips {
		if !PublicIP(ip) {
			return nil, fmt.Errorf("endpoint resolves to a private or special-purpose address")
		}
	}
	dial := g.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	for _, ip := range ips {
		conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("public endpoint connection failed")
}

type guarded struct{ transport *http.Transport }

func (g guarded) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := URL(r.URL.String()); err != nil {
		return nil, err
	}
	if r.Host != "" && r.Host != r.URL.Host {
		return nil, fmt.Errorf("custom Host is denied")
	}
	return g.transport.RoundTrip(r)
}

func (g Guard) Client() (*http.Client, func()) {
	t := &http.Transport{Proxy: nil, DialContext: g.dial, ForceAttemptHTTP2: false, MaxIdleConns: 64, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 32 << 10}
	return &http.Client{Transport: guarded{t}, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, t.CloseIdleConnections
}
