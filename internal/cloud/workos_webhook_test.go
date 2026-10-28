package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

const workOSSignupFixture = `{"id":"event_signup_1","event":"user.created","created_at":"2026-10-27T19:00:00.123Z","data":{"object":"user","id":"user_private","email":"private@example.com","first_name":"Private","metadata":{"private":"value"}}}`

func workOSHeader(raw, secret string, at time.Time) string {
	timestamp := fmt.Sprint(at.UnixMilli())
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + raw))
	return "t=" + timestamp + ", v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWorkOSSignature(t *testing.T) {
	now := time.Now()
	secret := "whsec_synthetic_workos_signing_secret"
	valid := workOSHeader(workOSSignupFixture, secret, now)
	for _, tc := range []struct {
		name, signature, body string
		valid                 bool
	}{
		{"valid", valid, workOSSignupFixture, true},
		{"changed body", valid, workOSSignupFixture + " ", false},
		{"wrong secret", workOSHeader(workOSSignupFixture, "wrong", now), workOSSignupFixture, false},
		{"old", workOSHeader(workOSSignupFixture, secret, now.Add(-6*time.Minute)), workOSSignupFixture, false},
		{"future", workOSHeader(workOSSignupFixture, secret, now.Add(6*time.Minute)), workOSSignupFixture, false},
		{"seconds", "t=" + fmt.Sprint(now.Unix()) + ",v1=00", workOSSignupFixture, false},
		{"duplicate timestamp", valid + ",t=1", workOSSignupFixture, false},
		{"duplicate signature", valid + ",v1=00", workOSSignupFixture, false},
		{"malformed", "t=invalid,v1=zz", workOSSignupFixture, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{"Workos-Signature": []string{tc.signature}}
			if got := validWorkOSSignature(h, []byte(tc.body), secret, now); got != tc.valid {
				t.Fatalf("signature accepted=%v, want %v", got, tc.valid)
			}
		})
	}
	h := http.Header{}
	h.Add("WorkOS-Signature", valid)
	h.Add("WorkOS-Signature", valid)
	if validSignature("workos", h, []byte(workOSSignupFixture), secret) {
		t.Fatal("multiple signature headers accepted")
	}
}

func TestWorkOSSignupProjectionExcludesPersonalData(t *testing.T) {
	body, key, ignored, err := workOSSignupInput([]byte(workOSSignupFixture))
	if err != nil || ignored || key != "event_signup_1" {
		t.Fatal(key, ignored, err)
	}
	var fields map[string]string
	if err := json.Unmarshal(body, &fields); err != nil || len(fields) != 3 || fields["type"] != "user.created" {
		t.Fatal("unexpected retained fields", string(body), err)
	}
	for _, text := range []string{"user_private", "private@example.com", "Private", "metadata"} {
		if strings.Contains(string(body), text) {
			t.Fatal("personal data entered signup evidence")
		}
	}
	if _, _, ignored, err := workOSSignupInput([]byte(strings.Replace(workOSSignupFixture, "user.created", "user.updated", 1))); err != nil || !ignored {
		t.Fatal("unselected event was not ignored", err)
	}
	for _, raw := range []string{`{}`, strings.Replace(workOSSignupFixture, `"object":"user"`, `"object":"organization"`, 1), strings.Replace(workOSSignupFixture, "2026-10-27T19:00:00.123Z", "invalid", 1)} {
		if _, _, _, err := workOSSignupInput([]byte(raw)); err == nil {
			t.Fatal("malformed signup accepted")
		}
	}
}

func TestWorkOSSignupDurablyQueuesOneNotificationOnRedelivery(t *testing.T) {
	s, h, sessions, _ := pushSetup(t)
	s.Push.NativeWebhooks = true
	ctx := context.Background()
	tenant, err := s.Pool.Get(ctx, sessions[0].Workspace)
	if err != nil {
		t.Fatal(err)
	}
	manifest := `apiVersion: ding.ing/v1alpha1
kind: Destination
metadata: {id: signup-slack}
spec: {type: slack, urlRef: {env: SIGNUP_TEST_SLACK}}
---
apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: signups}
spec:
  source: {type: push, fields: {eventId: eventId, type: type, createdAt: createdAt}}
  condition: {field: eventId, operator: new-event, dedupFor: 168h}
  policy: {trigger: level, interval: 0s}
  destinations: [{ref: signup-slack, events: [new-event]}]
`
	if _, err = tenant.App.Apply(ctx, watchrun.ApplyRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	secret := "whsec_synthetic_workos_signing_secret"
	for _, events := range []string{`[]`, `["user.updated"]`, `["user.created","user.updated"]`} {
		w := cloudRequest(h, "POST", "/v1/webhooks", `{"name":"Invalid WorkOS events","provider":"workos","watch":"signups","events":`+events+`,"secret":"`+secret+`"}`, sessions[0], true)
		if w.Code != 400 {
			t.Fatal("unsupported WorkOS subscription accepted", w.Code)
		}
	}
	w := cloudRequest(h, "POST", "/v1/webhooks", `{"name":"WorkOS signups","provider":"workos","watch":"signups","events":["user.created"],"secret":"`+secret+`"}`, sessions[0], true)
	var connection struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &connection) != nil || connection.Data.URL == "" {
		t.Fatalf("create connection: %d %s", w.Code, w.Body.String())
	}
	send := func(body, secret string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", connection.Data.URL, strings.NewReader(body))
		r.Header.Set("WorkOS-Signature", workOSHeader(body, secret, time.Now()))
		r.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		h.ServeHTTP(out, r)
		return out
	}
	first := receiptData(t, send(workOSSignupFixture, secret))
	duplicate := receiptData(t, send(workOSSignupFixture, secret))
	if first.Duplicate || !duplicate.Duplicate || first.AcceptanceID != duplicate.AcceptanceID {
		t.Fatal("redelivery changed durable acceptance")
	}
	if w := send(workOSSignupFixture, "wrong"); w.Code != 401 {
		t.Fatal("invalid signature accepted", w.Code)
	}
	if w := send(strings.Replace(workOSSignupFixture, "user.created", "user.updated", 1), secret); w.Code != 200 {
		t.Fatal("unselected event not acknowledged", w.Code)
	}
	if err := tenant.App.Store.View(ctx, func(tx *store.Tx) error {
		intents, err := tx.Outbox("signups")
		if err != nil {
			return err
		}
		if len(intents) != 1 {
			t.Fatalf("wanted one notification intent, got %d", len(intents))
		}
		for _, private := range []string{"user_private", "private@example.com", "Private", "metadata"} {
			if strings.Contains(string(intents[0].Payload), private) {
				t.Fatal("personal data entered Slack payload")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	other, err := s.Pool.Get(ctx, sessions[1].Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.App.Store.View(ctx, func(tx *store.Tx) error {
		intents, err := tx.Outbox("")
		if len(intents) != 0 {
			t.Fatal("signup notification crossed workspaces")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
