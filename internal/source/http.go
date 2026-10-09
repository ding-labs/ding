// Package source acquires bounded data. It does not advance durable checkpoints.
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/transform"
	"github.com/ding-labs/ding/internal/watch"
)

type Lookup func(string) (string, bool)
type Batch struct {
	Entity       string
	Deadline     time.Time
	Observations []watch.Observation
	Cursor       string
	RetryAt      time.Time
}
type HTTP struct {
	Client *http.Client
	Lookup Lookup
}
type validators struct {
	ETag     string `json:"etag,omitempty"`
	Modified string `json:"modified,omitempty"`
}

func Resolve(ref *watch.SecretRef, lookup Lookup) (string, error) {
	if ref == nil || lookup == nil {
		return "", fmt.Errorf("missing credential reference")
	}
	value, ok := lookup(ref.Env)
	if !ok || value == "" {
		return "", fmt.Errorf("missing credential")
	}
	return value, nil
}
func URL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return fmt.Errorf("invalid HTTP endpoint")
	}
	return nil
}
func (h HTTP) Fetch(ctx context.Context, p plan.Compiled, cursor string, now time.Time) Batch {
	s := p.Definition.Spec.Source
	b := Batch{Cursor: cursor}
	unknown := func(reason string) Batch {
		b.Observations = []watch.Observation{{Health: "unknown", Detail: reason}}
		return b
	}
	endpoint := s.URL
	if s.URLRef != nil {
		var err error
		endpoint, err = Resolve(s.URLRef, h.Lookup)
		if err != nil {
			return unknown("missing_credentials")
		}
	}
	if URL(endpoint) != nil {
		return unknown("invalid_endpoint")
	}
	timeout, _ := time.ParseDuration(s.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return unknown("invalid_request")
	}
	for name, ref := range s.Headers {
		value, err := Resolve(&ref, h.Lookup)
		if err != nil {
			return unknown("missing_credentials")
		}
		req.Header.Set(name, value)
	}
	var cached validators
	if cursor != "" && json.Unmarshal([]byte(cursor), &cached) != nil {
		return unknown("invalid_checkpoint")
	}
	if cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}
	if cached.Modified != "" {
		req.Header.Set("If-Modified-Since", cached.Modified)
	}
	client := http.DefaultClient
	if h.Client != nil {
		client = h.Client
	}
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := c.Do(req)
	if err != nil {
		return unknown("source_transport_failed")
	}
	defer response.Body.Close()
	b.RetryAt = delivery.RetryAfter(response.Header.Get("Retry-After"), now)
	if response.StatusCode == http.StatusNotModified {
		if cached.ETag == "" && cached.Modified == "" {
			return unknown("source_unexpected_304")
		}
		b.Observations = []watch.Observation{{Health: "unchanged"}}
		return b
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return unknown("source_redirect_rejected")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(p.Definition.Spec.Limits.MaxBytes)+1))
	if err != nil {
		return unknown("source_read_failed")
	}
	if len(body) > p.Definition.Spec.Limits.MaxBytes {
		return unknown("source_response_too_large")
	}
	outputs := []map[string]any{{}}
	if len(s.Fields) > 0 || s.JQ != "" {
		var err error
		outputs, err = transform.Project(ctx, body, s.JQ, s.Fields, p.Definition.Spec.Limits.MaxOutputs, p.Definition.Spec.Limits.MaxBytes)
		if err != nil {
			return unknown("source_invalid_projection")
		}
	}

	for _, fields := range outputs {
		fields["http.status"] = response.StatusCode
	}
	b.Observations, err = Observations(outputs, s.ObservedAtField)
	if err != nil {
		return unknown("source_invalid_observed_time")
	}

	data, _ := json.Marshal(validators{response.Header.Get("ETag"), response.Header.Get("Last-Modified")})
	b.Cursor = string(data)
	return b
}
