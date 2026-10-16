package cloud

import (
	"context"
	"database/sql"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

type heldSharedBody struct {
	reader           io.Reader
	started, release chan struct{}
	once             sync.Once
}

func (b *heldSharedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return b.reader.Read(p)
}
func (b *heldSharedBody) Close() error { return nil }

func TestSharedIngestExpiresBeforeDurableAcceptance(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			t.Setenv("DING_TEST_BACKEND", backend)
			s, h, sessions := testCloudServer(t)
			s.Shared = true
			s.TenantHandler = s.serveTenant
			ctx := context.Background()
			tenant, err := s.Pool.Get(ctx, sessions[0].Workspace)
			if err != nil {
				t.Fatal(err)
			}
			c := scaleConfig{Name: "expiry", Rate: 1, Seconds: 1}
			if err = c.validate(); err != nil {
				t.Fatal(err)
			}
			if _, err = tenant.App.Apply(ctx, watchrun.ApplyRequest{Manifest: c.manifest()}); err != nil {
				t.Fatal(err)
			}
			record, err := tenant.App.Record(ctx, "ci")
			if err != nil {
				t.Fatal(err)
			}
			principal, err := s.DB.Principal(ctx, sessions[0].User, sessions[0].Workspace, "browser", nil)
			if err != nil {
				t.Fatal(err)
			}
			grant, err := s.DB.CreateBoundService(ctx, principal, "expiry", []string{"ingest"}, []string{"ci"}, 1, map[string]string{"ci": record.Lifetime})
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(2 * time.Second).Truncate(time.Second)
			if pg, ok := s.Pool.storage.(postgresWorkspaces); ok {
				_, err = pg.pool.ExecContext(ctx, "UPDATE ding_control.service_keys SET expires_at=$1 WHERE service=$2", deadline.Unix(), grant.ID)
			} else {
				raw, e := sql.Open("sqlite", filepath.Join(s.Pool.root, "control.db"))
				if e != nil {
					t.Fatal(e)
				}
				_, err = raw.ExecContext(ctx, "UPDATE service_keys SET expires_at=? WHERE service=?", deadline.Unix(), grant.ID)
				raw.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			var before int64
			if err = tenant.App.Store.View(ctx, func(tx *store.Tx) error { var e error; before, e = tx.ObservationWatermark(); return e }); err != nil {
				t.Fatal(err)
			}
			bodies, _ := c.bodies()
			body := &heldSharedBody{reader: strings.NewReader(string(bodies[0])), started: make(chan struct{}), release: make(chan struct{})}
			r := httptest.NewRequest("POST", "https://ding.example/v1/ingest/ci", nil)
			r.Body = body
			r.ContentLength = -1
			r.Header.Set("Authorization", "Bearer "+grant.Token)
			r.Header.Set("Content-Type", "application/json")
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { w := httptest.NewRecorder(); h.ServeHTTP(w, r); done <- w }()
			select {
			case <-body.started:
			case response := <-done:
				t.Fatalf("request rejected before expiry fixture: %d", response.Code)
			case <-time.After(time.Second):
				t.Fatal("body did not start")
			}
			time.Sleep(max(0, time.Until(deadline)))
			close(body.release)
			response := <-done
			if response.Code != 401 {
				t.Fatal("expired producer accepted", response.Code, response.Body.String())
			}
			if err = tenant.App.Store.View(ctx, func(tx *store.Tx) error {
				after, e := tx.ObservationWatermark()
				if after != before {
					t.Error("expired producer appended observations")
				}
				return e
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
