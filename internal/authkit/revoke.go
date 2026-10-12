// Package authkit implements the server-side cleanup of WorkOS Connect consent.
package authkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

type Revoker struct {
	key  string
	base string
	http *http.Client
}

// NewRevoker pins the API origin; identity discovery cannot redirect this key.
func NewRevoker(key string) *Revoker {
	return &Revoker{key: key, base: "https://api.workos.com", http: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_]{1,128}$`)

// Revoke locates the application's API identifier by its exact OAuth client ID.
// Dashboard IDs and authorized-application IDs are not interchangeable.
func (c *Revoker) Revoke(ctx context.Context, subject, client string) error {
	if c.key == "" || !identifier.MatchString(subject) || client == "" {
		return fmt.Errorf("WorkOS revocation is not configured for this identity")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	path := "/user_management/users/" + subject + "/authorized_applications"
	after := ""
	for page := 0; page < 10; page++ {
		q := url.Values{"limit": {"100"}}
		if after != "" {
			q.Set("after", after)
		}
		body, err := c.request(ctx, "GET", path+"?"+q.Encode())
		if err != nil {
			return err
		}
		var result struct {
			Object string `json:"object"`
			Data   []struct {
				Application struct {
					ID       string `json:"id"`
					ClientID string `json:"client_id"`
				} `json:"application"`
			} `json:"data"`
			Metadata struct {
				After string `json:"after"`
			} `json:"list_metadata"`
		}
		if json.Unmarshal(body, &result) != nil || result.Object != "list" || result.Data == nil {
			return fmt.Errorf("invalid WorkOS applications response")
		}
		for _, app := range result.Data {
			if app.Application.ClientID == client {
				if !identifier.MatchString(app.Application.ID) {
					return fmt.Errorf("invalid WorkOS application identifier")
				}
				_, err = c.request(ctx, "DELETE", path+"/"+app.Application.ID)
				return err
			}
		}
		if result.Metadata.After == "" {
			return nil
		} // Already disconnected.
		if result.Metadata.After == after {
			return fmt.Errorf("invalid WorkOS pagination")
		}
		after = result.Metadata.After
	}
	return fmt.Errorf("WorkOS application lookup exceeded its limit")
}

func (c *Revoker) request(ctx context.Context, method, path string) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, method, c.base+path, nil)
	if err != nil {
		return nil, fmt.Errorf("could not prepare WorkOS revocation")
	}
	r.Header.Set("Authorization", "Bearer "+c.key)
	r.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(r)
	if err != nil {
		return nil, fmt.Errorf("WorkOS revocation unavailable")
	}
	defer resp.Body.Close()
	if method == "DELETE" && resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("WorkOS revocation returned HTTP %d", resp.StatusCode)
	}
	if method == "DELETE" {
		return nil, nil
	}
	const maxBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return nil, fmt.Errorf("could not read WorkOS applications")
	}
	return body, nil
}
