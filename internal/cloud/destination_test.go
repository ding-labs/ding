package cloud

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDestinationTestReconcilesWithoutSendingTwice(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	tenant, err := s.Pool.Get(context.Background(), sessions[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	var sent atomic.Int64
	tenant.App.HTTP.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		sent.Add(1)
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "not an incident") {
			t.Error("test was not labeled")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}
	if err := s.Vault.Put(context.Background(), tenant.Account.ID, "WEBHOOK", "https://example.com/hook"); err != nil {
		t.Fatal(err)
	}
	body := `{"operationKey":"a-stable-test-operation","destination":"webhook","credential":"WEBHOOK"}`
	for i := 0; i < 2; i++ {
		w := cloudRequest(h, "POST", "/v1/cloud/destination-test", body, sessions[0], true)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "accepted") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if sent.Load() != 1 {
		t.Fatal("test notification duplicated")
	}
	if err := s.Vault.Put(context.Background(), tenant.Account.ID, "WEBHOOK", "https://example.com/changed"); err != nil {
		t.Fatal(err)
	}
	if w := cloudRequest(h, "POST", "/v1/cloud/destination-test", body, sessions[0], true); w.Code != 409 {
		t.Fatal("changed credential reused proof", w.Code)
	}
}
