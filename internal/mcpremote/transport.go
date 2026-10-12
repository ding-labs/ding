package mcpremote

import (
	"io"
	"net/http"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

type secureTransport struct{ base http.RoundTripper }

func (t secureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.User != nil || r.URL.Fragment != "" {
		return nil, mcpconfig.ErrConfig
	}
	next := t.base
	if next == nil {
		next = http.DefaultTransport
	}
	response, err := next.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	if response.ContentLength > mcpclient.MaxResponse {
		response.Body.Close()
		return nil, mcpclient.Fail("response_too_large", "cloud response exceeds the bridge limit")
	}
	response.Body = boundedBody{Reader: io.LimitReader(response.Body, mcpclient.MaxResponse+1), Closer: response.Body}
	return response, nil
}

type boundedBody struct {
	io.Reader
	io.Closer
}
