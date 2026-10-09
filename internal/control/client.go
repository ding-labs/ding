package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/watch"
)

type Client struct {
	URL, Token string
	HTTP       *http.Client
}
type APIError struct{ Code, Message string }

func (e *APIError) Error() string { return e.Message }
func Connect(dir string) (Client, error) {
	var connection Connection
	var credentials Credentials
	// Connecting is read-only: an absent daemon must not create credentials.
	data, err := os.ReadFile(filepath.Join(dir, "connection.json"))
	if err != nil || json.Unmarshal(data, &connection) != nil || source.URL(connection.URL) != nil {
		return Client{}, fmt.Errorf("daemon unavailable; start ding daemon with this state directory")
	}
	data, err = os.ReadFile(filepath.Join(dir, "tokens.json"))
	if err != nil || json.Unmarshal(data, &credentials) != nil || len(credentials.Admin) < 32 {
		return Client{}, fmt.Errorf("cannot read daemon credentials")
	}
	return Client{URL: connection.URL, Token: credentials.Admin, HTTP: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c Client) Call(ctx context.Context, method, path string, request any) (json.RawMessage, error) {
	var data []byte
	var err error
	if request != nil {
		data, err = json.Marshal(request)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid daemon endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach daemon")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<20+1))
	if err != nil || len(body) > 64<<20 {
		return nil, fmt.Errorf("invalid daemon response")
	}
	var envelope struct {
		APIVersion string          `json:"apiVersion"`
		Data       json.RawMessage `json:"data"`
		Error      *watch.Error    `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.APIVersion != watch.APIVersion {
		return nil, fmt.Errorf("invalid daemon response")
	}
	if envelope.Error != nil {
		return nil, &APIError{envelope.Error.Code, envelope.Error.Message}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("daemon request failed")
	}
	return envelope.Data, nil
}
