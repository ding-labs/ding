package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ding-labs/ding/internal/watch"
)

// Produce retries only transient failures with the same immutable request and
// receipt key. Its caller supplies a short freshness deadline for live summaries.
func (c Client) Produce(ctx context.Context, path, key string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	backoff := 50 * time.Millisecond
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+path, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("invalid producer endpoint")
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		response, err := client.Do(req)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
			response.Body.Close()
			if response.StatusCode == http.StatusAccepted && readErr == nil {
				var receipt watch.Envelope
				if json.Unmarshal(data, &receipt) == nil && receipt.APIVersion == watch.APIVersion && receipt.Error == nil {
					return nil
				}
			}
			if response.StatusCode < 500 && response.StatusCode != 429 && response.StatusCode != http.StatusAccepted {
				return fmt.Errorf("producer request rejected (HTTP %d)", response.StatusCode)
			}
			if delay, err := time.ParseDuration(response.Header.Get("Retry-After") + "s"); err == nil && delay > backoff {
				backoff = delay
			}
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("live summary could not be committed before its deadline")
		case <-timer.C:
		}
		if backoff < 250*time.Millisecond {
			backoff *= 2
		}
	}
}
