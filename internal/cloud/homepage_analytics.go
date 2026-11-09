package cloud

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const homepageAnalyticsOrigin = "https://ding.ing"

var homepageHandoff = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")

type homepageAnalytics struct {
	key    string
	origin string
	client *http.Client
}

// Only the public-site Worker receives this dedicated credential. Redirects
// must never carry credentials or a pseudonymous account to another origin.
func newHomepageAnalytics(key string) *homepageAnalytics {
	return &homepageAnalytics{key: key, origin: homepageAnalyticsOrigin, client: &http.Client{
		Timeout:       2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (a *homepageAnalytics) post(ctx context.Context, action, subject, handoff string) (*http.Response, error) {
	account := sha256.Sum256([]byte(subject))
	payload, err := json.Marshal(map[string]string{"action": action, "account": hex.EncodeToString(account[:]), "handoff": handoff})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", a.origin+"/_funnel/"+action, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(a.key))
	io.WriteString(mac, ts+".")
	mac.Write(payload)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Ding-Signature", "t="+ts+", v1="+hex.EncodeToString(mac.Sum(nil)))
	return a.client.Do(req)
}

func (a *homepageAnalytics) confirm(ctx context.Context, subject, handoff string) error {
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := a.post(ctx, "verified", subject, handoff)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNoContent {
				return nil
			}
			if resp.StatusCode < 500 {
				return fmt.Errorf("receipt returned %d", resp.StatusCode)
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return errors.New("homepage measurement unavailable")
}
