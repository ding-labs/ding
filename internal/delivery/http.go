// Package delivery implements one bounded delivery attempt. Retry scheduling and
// persistence belong to callers, so the same transport can serve an outbox.
package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Outcome string

const (
	Delivered Outcome = "delivered"
	Retryable Outcome = "retryable"
	Permanent Outcome = "permanent"
	Exhausted Outcome = "exhausted"
	Canceled  Outcome = "canceled"
)

type Result struct {
	Outcome Outcome
	RetryAt time.Time
	Detail  string
}
type Request struct {
	URL      string
	Body     []byte
	Header   http.Header
	Provider string
}

// HTTP never includes destination URLs, response bodies, or tokens in errors.
// Redirects are rejected: credentials and alert contents must not be forwarded.
func HTTP(ctx context.Context, client *http.Client, request Request, now time.Time) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, request.URL, bytes.NewReader(request.Body))
	if err != nil {
		return Result{Outcome: Permanent, Detail: "invalid delivery request"}
	}
	req.Header = request.Header.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Content-Type", "application/json")
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		return Result{Outcome: Retryable, Detail: "delivery transport failed"}
	}
	defer resp.Body.Close()
	result := Result{Outcome: Permanent, Detail: fmt.Sprintf("HTTP %d", resp.StatusCode), RetryAt: RetryAfter(resp.Header.Get("Retry-After"), now)}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
		result.Outcome = Retryable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if request.Provider == "telegram" && err == nil && len(body) <= 64*1024 {
			var envelope struct {
				Parameters struct {
					Retry int `json:"retry_after"`
				} `json:"parameters"`
			}
			if json.Unmarshal(body, &envelope) == nil && envelope.Parameters.Retry > 0 {
				at := RetryAfter(strconv.Itoa(envelope.Parameters.Retry), now)
				if at.After(result.RetryAt) {
					result.RetryAt = at
				}
			}
		}
		return result
	}
	if err != nil || len(body) > 64*1024 {
		return Result{Outcome: Retryable, Detail: "invalid delivery response"}
	}
	switch request.Provider {
	case "slack":
		if strings.TrimSpace(string(body)) != "ok" {
			return Result{Outcome: Permanent, Detail: "Slack rejected delivery"}
		}
	case "telegram":
		var envelope struct {
			OK         bool `json:"ok"`
			Code       int  `json:"error_code"`
			Parameters struct {
				Retry int `json:"retry_after"`
			} `json:"parameters"`
		}
		if json.Unmarshal(body, &envelope) != nil {
			return Result{Outcome: Retryable, Detail: "invalid Telegram response"}
		}
		if !envelope.OK {
			result.Detail = "Telegram rejected delivery"
			if envelope.Code == 429 || envelope.Code == 408 || envelope.Code >= 500 {
				result.Outcome = Retryable
			}
			if envelope.Parameters.Retry > 0 {
				result.RetryAt = RetryAfter(strconv.Itoa(envelope.Parameters.Retry), now)
			}
			return result
		}
	case "pagerduty":
		var envelope struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(body, &envelope) != nil {
			return Result{Outcome: Retryable, Detail: "invalid PagerDuty response"}
		}
		if envelope.Status != "success" {
			return Result{Outcome: Permanent, Detail: "PagerDuty rejected delivery"}
		}
	}
	return Result{Outcome: Delivered}
}

func RetryAfter(value string, now time.Time) time.Time {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && seconds >= 0 {
		// Bound the timestamp arithmetic; callers may expire jobs before this time.
		if seconds > 365*24*3600 {
			seconds = 365 * 24 * 3600
		}
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date
	}
	return time.Time{}
}

func Backoff(initial time.Duration, attempt int) time.Duration {
	if initial <= 0 {
		initial = time.Second
	}
	for i := 1; i < attempt && initial < time.Hour; i++ {
		initial *= 2
	}
	if initial > time.Hour || initial <= 0 {
		return time.Hour
	}
	return initial
}
