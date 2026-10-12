package mcpremote

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/mcpclient"
)

func TestSecureTransportRejectsUnsafeOAuthEndpointsBeforeSending(t *testing.T) {
	called := false
	transport := secureTransport{tripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	})}
	for _, endpoint := range []string{"http://identity.example/token", "https://user:password@identity.example/token", "https://identity.example/token#fragment"} {
		r, err := http.NewRequest("POST", endpoint, strings.NewReader("refresh_token=secret"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := transport.RoundTrip(r); err == nil || called {
			t.Fatal("sent credentials to an unsafe endpoint")
		}
	}
}

type transportBody struct {
	io.Reader
	closed bool
}

func (b *transportBody) Close() error { b.closed = true; return nil }

func TestSecureTransportBoundsKnownAndUnknownResponseLengths(t *testing.T) {
	for _, length := range []int64{-1, mcpclient.MaxResponse + 2} {
		body := &transportBody{Reader: strings.NewReader(strings.Repeat(" ", mcpclient.MaxResponse+2))}
		transport := secureTransport{tripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{Body: body, ContentLength: length}, nil
		})}
		r, _ := http.NewRequest("GET", "https://identity.example/metadata", nil)
		response, err := transport.RoundTrip(r)
		if length > mcpclient.MaxResponse {
			if err == nil || !body.closed {
				t.Fatal("accepted oversized response or leaked body")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		n, err := io.Copy(io.Discard, response.Body)
		if err != nil || n != mcpclient.MaxResponse+1 {
			t.Fatal("unbounded response", n, err)
		}
		response.Body.Close()
		if !body.closed {
			t.Fatal("leaked response body")
		}
	}
}
