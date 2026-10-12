package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	state "github.com/ding-labs/ding/internal/serverstate"
	stripehook "github.com/stripe/stripe-go/v86/webhook"
)

func githubHeader(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
func TestGitHubSignedWorkflowUsesScopedDurablePush(t *testing.T) {
	s, h, sessions, _ := pushSetup(t)
	s.Push.NativeWebhooks = true
	p, err := s.DB.Principal(context.Background(), sessions[0].User, sessions[0].Workspace, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	life, err := s.producerLifetimes(context.Background(), p.Workspace, []string{"ci"})
	if err != nil {
		t.Fatal(err)
	}
	secret := "a-test-secret-at-least-32-characters"
	c, err := s.Vault.CreateWebhook(context.Background(), p, state.WebhookConnection{Name: "GitHub", Provider: "github", Watch: "ci", Lifetime: life["ci"], Repositories: []string{"123"}}, secret)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"action":"completed","repository":{"id":123,"full_name":"owner/api"},"workflow_run":{"id":456,"run_attempt":1,"name":"test","status":"completed","conclusion":"failure"}}`
	send := func(body, key, secret string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://ding.example/v1/hooks/github/"+c.ID, strings.NewReader(body))
		r.Header.Set("X-Hub-Signature-256", githubHeader(body, secret))
		r.Header.Set("X-GitHub-Event", "workflow_run")
		r.Header.Set("X-GitHub-Delivery", key)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	original := receiptData(t, send(body, "delivery", secret))
	if original.Duplicate {
		t.Fatal("first input duplicate")
	}
	if !receiptData(t, send(body, "delivery", secret)).Duplicate {
		t.Fatal("provider redelivery duplicated")
	}
	if w := send(body, "bad", "wrong"); w.Code != 401 {
		t.Fatal("invalid signature", w.Code)
	}
	if w := send(strings.Replace(body, `"id":123`, `"id":124`, 1), "foreign", secret); w.Code != 400 {
		t.Fatal("wrong repository", w.Code)
	}
	if w := send(strings.Replace(body, `"failure"`, `"cancelled"`, 1), "canceled", secret); w.Code != 200 {
		t.Fatal("canceled event must be ignored", w.Code)
	}
	newSecret := "another-strong-secret-at-least-32-characters"
	if err := s.Vault.RotateWebhook(context.Background(), p, c.ID, newSecret, 60); err != nil {
		t.Fatal(err)
	}
	if !receiptData(t, send(body, "delivery", newSecret)).Duplicate {
		t.Fatal("signature rotation changed retry namespace")
	}
	if err := s.DB.DeleteWebhook(context.Background(), p, c.ID); err != nil {
		t.Fatal(err)
	}
	if w := send(body, "delivery", newSecret); w.Code != 404 {
		t.Fatal("deleted connection accepted", w.Code)
	}
}
func TestStripeSignatureAndSnapshotContract(t *testing.T) {
	raw := []byte(`{"id":"evt_123","object":"event","type":"invoice.payment_failed","account":"acct_123","livemode":false,"data":{"object":{"id":"in_123","object":"invoice","amount_due":500}}}`)
	secret := "whsec_this_is_a_synthetic_test_secret"
	signed := stripehook.GenerateTestSignedPayload(&stripehook.UnsignedPayload{Payload: raw, Secret: secret, Timestamp: time.Now()})
	header := http.Header{}
	header.Set("Stripe-Signature", signed.Header)
	if !validSignature("stripe", header, raw, secret) || validSignature("stripe", header, append(raw, ' '), secret) {
		t.Fatal("raw signature contract")
	}
	old := stripehook.GenerateTestSignedPayload(&stripehook.UnsignedPayload{Payload: raw, Secret: secret, Timestamp: time.Now().Add(-10 * time.Minute)})
	header.Set("Stripe-Signature", old.Header)
	if validSignature("stripe", header, raw, secret) {
		t.Fatal("stale signature")
	}
	c := state.WebhookConnection{Provider: "stripe", Account: "acct_123", Events: []string{"invoice.payment_failed"}}
	body, key, ignored, err := webhookInput(c, header, raw)
	if err != nil || ignored || key != "evt_123" || !json.Valid(body) {
		t.Fatal(key, ignored, err)
	}
	c.Live = true
	if _, _, _, err := webhookInput(c, header, raw); err == nil {
		t.Fatal("mode bypass")
	}
	c.Live = false
	c.Account = "acct_other"
	if _, _, _, err := webhookInput(c, header, raw); err == nil {
		t.Fatal("account bypass")
	}
	if _, _, _, err := webhookInput(c, header, []byte(`{"id":"evt_thin","object":"event","type":"invoice.payment_failed","livemode":false}`)); err == nil {
		t.Fatal("thin event accepted")
	}
}
