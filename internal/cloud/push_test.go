package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

const pushFixture = `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: ci}
spec:
  source: {type: push, fields: {repo: repo, failed: failed}}
  groupBy: [repo]
  condition: {field: failed, operator: eq, value: true}
  policy: {trigger: transition}
`

func pushRequest(h http.Handler, token, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "https://ding.example/v1/ingest/ci", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func receiptData(t *testing.T, w *httptest.ResponseRecorder) store.PushReceipt {
	t.Helper()
	var out struct {
		Data store.PushReceipt `json:"data"`
	}
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatalf("acceptance: %d %s", w.Code, w.Body.String())
	}
	return out.Data
}
func pushSetup(t *testing.T) (*Server, http.Handler, []state.Session, []state.Service) {
	t.Helper()
	s, _, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	s.Push = PushConfig{WorkspaceConcurrent: 16, WorkspaceRate: 1000, WorkspaceBurst: 1000, ProducerRate: 1000, ProducerBurst: 1000, BytesPerSecond: 100 << 20, ByteBurst: 100 << 20}
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	grants := []state.Service{}
	for _, session := range sessions {
		tenant, err := s.Pool.Get(context.Background(), session.Workspace)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: pushFixture}); err != nil {
			t.Fatal(err)
		}
		w := cloudRequest(h, "POST", "/v1/services", `{"name":"CI","scopes":["ingest"],"watches":["ci"],"days":30}`, session, true)
		var out struct {
			Data state.Service `json:"data"`
		}
		if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		grants = append(grants, out.Data)
	}
	return s, h, sessions, grants
}

func TestCloudPushIsolationRetryRotationLifecycle(t *testing.T) {
	s, h, sessions, grants := pushSetup(t)
	body := `{"repo":"owner/api","failed":true}`
	a := receiptData(t, pushRequest(h, grants[0].Token, "run-1", body))
	b := receiptData(t, pushRequest(h, grants[1].Token, "run-1", body))
	if a.AcceptanceID == b.AcceptanceID || a.Duplicate || b.Duplicate {
		t.Fatal("shared receipt namespace")
	}
	replay := receiptData(t, pushRequest(h, grants[0].Token, "run-1", body))
	if !replay.Duplicate || replay.AcceptanceID != a.AcceptanceID {
		t.Fatal("retry reevaluated")
	}
	if w := pushRequest(h, grants[0].Token, "run-1", `{"repo":"owner/api","failed":false}`); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("POST", "https://ding.example/v1/ingest/ci", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+grants[0].Token)
	r.Header.Set("X-Ding-Workspace", sessions[1].Workspace)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign workspace", w.Code)
	}
	p, err := s.DB.Principal(context.Background(), sessions[0].User, sessions[0].Workspace, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := s.DB.RotateService(context.Background(), p, grants[0].ID, 30, 60)
	if err != nil {
		t.Fatal(err)
	}
	if out := receiptData(t, pushRequest(h, rotated.Token, "run-1", body)); !out.Duplicate || out.AcceptanceID != a.AcceptanceID {
		t.Fatal("rotation lost receipt")
	}
	tenant, _ := s.Pool.Get(context.Background(), p.Workspace)
	if _, err := tenant.App.Lifecycle(context.Background(), "ci", watchrun.LifecycleRequest{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	if out := receiptData(t, pushRequest(h, rotated.Token, "run-1", body)); !out.Duplicate {
		t.Fatal("paused retry")
	}
	if w := pushRequest(h, rotated.Token, "new", body); w.Code != 409 {
		t.Fatal("paused accepted new input", w.Code)
	}
	if _, err := tenant.App.Lifecycle(context.Background(), "ci", watchrun.LifecycleRequest{Action: "delete"}); err != nil {
		t.Fatal(err)
	}
	// The public API retains deletion tombstones until history pruning.
	if err := tenant.App.Store.Update(context.Background(), func(tx *store.Tx) error {
		if err := tx.ResetState("ci"); err != nil {
			return err
		}
		return tx.Retain(time.Now().Add(31*24*time.Hour), time.Now().Add(31*24*time.Hour), nil, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: pushFixture}); err != nil {
		t.Fatal(err)
	}
	if w := pushRequest(h, rotated.Token, "new", body); w.Code != 403 {
		t.Fatal("grant inherited recreated watch", w.Code, w.Body.String())
	}
	if err := s.DB.RevokeService(context.Background(), p, grants[0].ID); err != nil {
		t.Fatal(err)
	}
	if w := pushRequest(h, rotated.Token, "run-1", body); w.Code != 401 {
		t.Fatal("revoked key", w.Code)
	}
}

func TestCloudPushConcurrentDedupAndUsage(t *testing.T) {
	s, h, sessions, grants := pushSetup(t)
	var wg sync.WaitGroup
	replies := make(chan *httptest.ResponseRecorder, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			replies <- pushRequest(h, grants[0].Token, "same", `{"repo":"a","failed":true}`)
		}()
	}
	wg.Wait()
	close(replies)
	unique := 0
	id := ""
	for w := range replies {
		out := receiptData(t, w)
		if !out.Duplicate {
			unique++
		}
		if id != "" && id != out.AcceptanceID {
			t.Fatal("concurrent receipts differ")
		}
		id = out.AcceptanceID
	}
	if unique != 1 {
		t.Fatal("unique accepts", unique)
	}
	tenant, _ := s.Pool.Get(context.Background(), sessions[0].Workspace)
	if err := tenant.App.Store.View(context.Background(), func(tx *store.Tx) error {
		u, e := tx.PushUsage()
		if u.Requests != 1 || u.Observations != 1 {
			t.Error("usage", u)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	// Closing admission preserves reconciliation and persisted definitions.
	s.Push.Disabled = true
	if !receiptData(t, pushRequest(h, grants[0].Token, "same", `{"repo":"a","failed":true}`)).Duplicate {
		t.Fatal("closed replay")
	}
	if w := pushRequest(h, grants[0].Token, "different", `{"repo":"a","failed":true}`); w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestCloudPushValidationAndBatchAtomicity(t *testing.T) {
	s, h, sessions, grants := pushSetup(t)
	cases := []struct {
		body   string
		status int
	}{
		{`{"repo":"a","repo":"b","failed":true}`, 400}, {`{"failed":true}`, 400}, {`{"repo":null,"failed":true}`, 400}, {`{"repo":"a","failed":9007199254740993}`, 400}, {`{"repo":"a"} {}`, 400}, {`[]`, 400}, {`{"repo":"a","ignored":` + strings.Repeat("[", 40) + `0` + strings.Repeat("]", 40) + `}`, 413},
	}
	for i, tc := range cases {
		w := pushRequest(h, grants[0].Token, fmt.Sprint(i), tc.body)
		if w.Code != tc.status {
			t.Errorf("case %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	tenant, _ := s.Pool.Get(context.Background(), sessions[0].Workspace)
	manifest := strings.Replace(pushFixture, "type: push, fields:", "type: push, itemsPath: events, fields:", 1)
	if _, err := tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if w := pushRequest(h, grants[0].Token, "batch-bad", `{"events":[{"repo":"a","failed":true},{"failed":false}]}`); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := tenant.App.Store.View(context.Background(), func(tx *store.Tx) error {
		u, e := tx.PushUsage()
		if u.Requests != 0 {
			t.Error("partial batch", u)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	out := receiptData(t, pushRequest(h, grants[0].Token, "batch", `{"events":[{"repo":"a","failed":true},{"repo":"b","failed":false}]}`))
	if out.Last-out.First != 1 {
		t.Fatal("batch bounds", out)
	}
}

type stalledPushBody struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *stalledPushBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return 0, io.EOF
}
func (b *stalledPushBody) Close() error { return nil }
func TestCloudPushSlowProducerDoesNotFenceRevocation(t *testing.T) {
	s, h, sessions, grants := pushSetup(t)
	body := &stalledPushBody{started: make(chan struct{}), release: make(chan struct{})}
	r := httptest.NewRequest("POST", "https://ding.example/v1/ingest/ci", nil)
	r.Body = body
	r.ContentLength = -1
	r.Header.Set("Authorization", "Bearer "+grants[0].Token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "slow")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { w := httptest.NewRecorder(); h.ServeHTTP(w, r); done <- w }()
	<-body.started
	p := access.Principal{User: sessions[0].User, Workspace: sessions[0].Workspace}
	revoked := make(chan error, 1)
	go func() { revoked <- s.DB.RevokeService(context.Background(), p, grants[0].ID) }()
	select {
	case err := <-revoked:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("slow body held revocation fence")
	}
	close(body.release)
	w := <-done
	if w.Code != 401 {
		t.Fatal("revoked in-flight request", w.Code, w.Body.String())
	}
}
