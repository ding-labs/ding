package cloud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// WorkOS signs timestamp + "." + the untouched body with the literal endpoint
// secret. Its timestamp is in milliseconds, unlike Stripe's seconds.
func validWorkOSSignature(header http.Header, raw []byte, secret string, now time.Time) bool {
	if len(header.Values("WorkOS-Signature")) != 1 {
		return false
	}
	var timestamp, signature string
	for _, part := range strings.Split(header.Get("WorkOS-Signature"), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return false
		}
		switch key {
		case "t":
			if timestamp != "" || value == "" {
				return false
			}
			timestamp = value
		case "v1":
			if signature != "" || value == "" {
				return false
			}
			signature = value
		}
	}
	millis, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || millis <= 0 {
		return false
	}
	issued := time.UnixMilli(millis)
	if issued.Before(now.Add(-5*time.Minute)) || issued.After(now.Add(5*time.Minute)) {
		return false
	}
	want, err := hex.DecodeString(signature)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(raw)
	return hmac.Equal(want, mac.Sum(nil))
}

func workOSSignupInput(raw []byte) ([]byte, string, bool, error) {
	var event struct {
		ID        string `json:"id"`
		Event     string `json:"event"`
		CreatedAt string `json:"created_at"`
		Data      struct {
			ID     string `json:"id"`
			Object string `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &event) != nil || event.ID == "" || len(event.ID) > 256 || event.Event == "" {
		return nil, "", false, fmt.Errorf("WorkOS event identity required")
	}
	if event.Event != "user.created" {
		return nil, "", true, nil
	}
	if event.Data.ID == "" || event.Data.Object != "user" {
		return nil, "", false, fmt.Errorf("WorkOS user.created snapshot required")
	}
	if _, err := time.Parse(time.RFC3339Nano, event.CreatedAt); err != nil {
		return nil, "", false, fmt.Errorf("WorkOS event creation time required")
	}
	// Only the provider event ID, type and timestamp enter retained observations
	// and notifications. User IDs, names, emails and metadata stay out of them.
	body, err := json.Marshal(map[string]string{"eventId": event.ID, "type": event.Event, "createdAt": event.CreatedAt})
	return body, event.ID, false, err
}
