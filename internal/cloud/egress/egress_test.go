package egress

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

type resolver []netip.Addr

func (r resolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) { return r, nil }

func TestDeniesMetadataMappedPrivateAndSpecialNetworks(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "10.1.2.3", "172.16.0.1", "192.168.0.1", "100.100.100.200", "0.0.0.0", "240.0.0.1", "198.18.0.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "64:ff9b::a00:1", "2002:7f00:1::1", "2001:db8::1"} {
		if PublicIP(netip.MustParseAddr(ip)) {
			t.Fatal("allowed", ip)
		}
	}
	for _, ip := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !PublicIP(netip.MustParseAddr(ip)) {
			t.Fatal("denied", ip)
		}
	}
	for _, raw := range []string{"http://localhost", "http://api.local", "http://api.internal", "http://127.1", "http://2130706433", "http://user:pass@example.com", "file:///etc/passwd", "https://example.com:1234", "http://[::ffff:127.0.0.1]", "https://example.com/#secret"} {
		if URL(raw) == nil {
			t.Fatal("allowed", raw)
		}
	}
}

func TestDNSAnswerIsPinnedAndMixedPrivateAnswerFailsClosed(t *testing.T) {
	called := false
	g := Guard{Resolver: resolver{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("10.0.0.1")}, Dial: func(_ context.Context, _ string, address string) (net.Conn, error) {
		called = true
		if address != "1.1.1.1:443" {
			t.Fatal("unvalidated DNS lookup", address)
		}
		return nil, errors.New("synthetic refusal")
	}}
	if _, err := g.dial(context.Background(), "tcp", "example.com:443"); err == nil || called {
		t.Fatal("mixed DNS escaped guard")
	}
	g.Resolver = resolver{netip.MustParseAddr("1.1.1.1")}
	if _, err := g.dial(context.Background(), "tcp", "example.com:443"); err == nil || !called {
		t.Fatal("dial did not use pinned address")
	}
}
