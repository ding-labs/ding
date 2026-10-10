// Package mcpclient is a bounded client of Ding's scoped integration API.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

const MaxResponse = 16 << 20

type Error struct{ Code, Message string }

func (e *Error) Error() string        { return e.Code + ": " + e.Message }
func Fail(code, message string) error { return &Error{code, message} }
func Unknown() error {
	return Fail("outcome_unknown", "connection interrupted; look up the SAME operation key before retrying; never generate a new key for this attempt")
}

func Segment(value string) (string, error) {
	if value == "" || utf8.RuneCountInString(value) > 256 || value == "." || value == ".." || strings.ContainsAny(value, "/\\\x00") {
		return "", Fail("invalid_id", "use an ID returned by Ding")
	}
	return url.PathEscape(value), nil
}

func HTTPClient() *http.Client {
	return &http.Client{Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{
		Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second,
		MaxConnsPerHost: 4, MaxIdleConns: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 35 * time.Second,
	}}
}

type Client struct {
	Connection mcpconfig.Connection
	HTTP       *http.Client
}

func New(connection mcpconfig.Connection) *Client {
	return &Client{Connection: connection, HTTP: HTTPClient()}
}
func (c *Client) Close() { c.HTTP.CloseIdleConnections() }

func (c *Client) Call(ctx context.Context, method, path string, query map[string]any, body any) (any, error) {
	connection := c.Connection
	if err := connection.Validate(); err != nil {
		return nil, mcpconfig.ErrConfig
	}
	if !allowed(method, path) {
		return nil, Fail("invalid_route", "unsupported integration operation")
	}
	u := connection.DaemonURL + "/v1/integrations" + path
	if len(query) > 0 {
		v := url.Values{}
		for k, value := range query {
			b, _ := json.Marshal(value)
			if s, ok := value.(string); ok {
				v.Set(k, s)
			} else {
				v.Set(k, string(b))
			}
		}
		u += "?" + v.Encode()
	}
	var content []byte
	var err error
	if body != nil {
		content, err = json.Marshal(body)
		if err != nil {
			return nil, Fail("invalid_arguments", "cannot encode request")
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(content))
	if err != nil {
		return nil, Fail("invalid_route", "unsupported integration operation")
	}
	req.Header.Set("Authorization", "Bearer "+c.Connection.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	mutation := method == http.MethodPost && path != "/preview"
	failure := func(code, message string) (any, error) {
		if mutation {
			return nil, Unknown()
		}
		return nil, Fail(code, message)
	}
	response, err := c.HTTP.Do(req)
	if err != nil {
		return failure("daemon_unavailable", "start Ding or check the configured connection")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxResponse+1))
	if err != nil {
		return failure("daemon_unavailable", "Ding response was interrupted")
	}
	if len(raw) > MaxResponse {
		return failure("response_too_large", "narrow the query or inspect in Ding Console")
	}
	var envelope struct {
		APIVersion string          `json:"apiVersion"`
		Data       json.RawMessage `json:"data"`
		Error      *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return failure("invalid_response", "Ding returned an invalid API response")
	}
	if envelope.APIVersion != "ding.ing/v1alpha1" {
		return failure("incompatible_daemon", "upgrade Ding to an integration-enabled build")
	}
	if envelope.Error != nil {
		return nil, Fail(bounded(envelope.Error.Code, 100), bounded(envelope.Error.Message, 2000))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || len(envelope.Data) == 0 {
		return failure("daemon_error", "Ding could not complete the request")
	}
	var data any
	if json.Unmarshal(envelope.Data, &data) != nil {
		return failure("invalid_response", "Ding returned an invalid API response")
	}
	return data, nil
}

func bounded(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}

func allowed(method, path string) bool {
	if strings.ContainsAny(path, "?\\\x00") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if !strings.HasPrefix(path, "/") {
		return false
	}
	for _, p := range parts {
		decoded, err := url.PathUnescape(p)
		if err != nil {
			return false
		}
		if _, err := Segment(decoded); err != nil {
			return false
		}
	}
	if method == http.MethodGet {
		if len(parts) == 1 {
			switch parts[0] {
			case "capabilities", "watches", "events", "deliveries", "destinations":
				return true
			}
		}
		if len(parts) == 2 {
			switch parts[0] {
			case "watches", "events", "deliveries", "operations":
				return true
			}
		}
	}
	if method == http.MethodPost {
		return path == "/preview" || path == "/apply" || len(parts) == 3 && (parts[0] == "watches" && parts[2] == "lifecycle" || parts[0] == "deliveries" && parts[2] == "retry")
	}
	return false
}

// SafeMessage distinguishes already-sanitized boundary errors from arbitrary
// network, filesystem, parser, or SDK exceptions.
func SafeMessage(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Error()
	}
	if errors.Is(err, os.ErrExist) {
		return "configuration already exists; use it or explicitly revoke its grant before re-pairing"
	}
	return "could not complete the command; verify private configuration, daemon availability, and required options"
}
