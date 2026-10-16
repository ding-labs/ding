package cloud

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/access"
	state "github.com/ding-labs/ding/internal/serverstate"
)

func TestCloudPushCanceledAuthorizationIsUnavailable(t *testing.T) {
	_, handler, _, grants := pushSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("POST", "https://ding.example/v1/ingest/ci", strings.NewReader(`{"repo":"owner/api","failed":true}`)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+grants[0].Token)
	r.Header.Set("Idempotency-Key", "canceled-auth")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"authorization_unavailable"`) {
		t.Fatalf("temporary authorization failure became revocation: %d %s", w.Code, w.Body.String())
	}
	receipt := receiptData(t, pushRequest(handler, grants[0].Token, "canceled-auth", `{"repo":"owner/api","failed":true}`))
	if receipt.Duplicate {
		t.Fatal("failed authorization committed input")
	}
}

func TestPushAuthorizationDistinguishesDenialFromStorageFailure(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{sql.ErrNoRows, 401}, {access.ErrDenied, 401}, {state.ErrServiceRevoked, 401},
		{context.Canceled, 503}, {context.DeadlineExceeded, 503}, {errors.New("private database failure"), 503},
	} {
		w := httptest.NewRecorder()
		pushAuthorizationError(w, tc.err)
		if w.Code != tc.status || strings.Contains(w.Body.String(), "private database failure") {
			t.Fatalf("authorization failure classification: %d %s", w.Code, w.Body.String())
		}
	}
}
