package watchcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestInspectionAuthoringCLIWorkflow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	creds, err := control.PrivateCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}
	app := watchrun.New(s)
	app.Lookup = func(string) (string, bool) { return "", false }
	server := httptest.NewServer(control.Handler(app, creds))
	defer server.Close()
	if err := control.SaveConnection(dir, server.URL); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []byte {
		t.Helper()
		var out, errs bytes.Buffer
		if err := Execute("test", append(args, "--state-dir", dir), &out, &errs); err != nil {
			t.Fatal(args, err, errs.String())
		}
		return out.Bytes()
	}
	manifest := "../../examples/watches/api-health.yaml"
	run("validate", manifest, "--json")
	run("explain", manifest, "--json")
	run("test", manifest, "--events", "../../testdata/watches/api-health.jsonl", "--json")
	run("apply", manifest, "--dry-run", "--json")
	records, err := app.List(ctx)
	if err != nil || len(records) != 0 {
		t.Fatal("dry run changed store", records, err)
	}
	run("apply", manifest, "--json")
	r, err := app.Record(ctx, "api-health")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for n := 0; n < 3; n++ {
		_, err := app.Accept(ctx, r, source.Batch{Observations: []watch.Observation{{Health: "ok", Fields: map[string]any{"http.status": 503}}}}, string(rune('a'+n)), now.Add(time.Duration(n)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	var envelope struct {
		Data store.EventPage `json:"data"`
	}
	if err := json.Unmarshal(run("events", "--watch", "api-health", "--limit", "1", "--json"), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Cursor == "" || len(envelope.Data.Events) != 1 {
		t.Fatal(envelope)
	}
	var next struct {
		Data store.EventPage `json:"data"`
	}
	json.Unmarshal(run("events", "--watch", "api-health", "--cursor", envelope.Data.Cursor, "--json"), &next)
	if len(next.Data.Events) != 1 || next.Data.Events[0].Type != "firing" {
		t.Fatal(next)
	}
	event := next.Data.Events[0]
	proof := run("events", "inspect", event.ID, "--json")
	path := filepath.Join(t.TempDir(), "proof.json")
	os.WriteFile(path, proof, 0600)
	run("replay", path, "--json")
	run("events", "observations", event.ID, "--json")
	run("doctor", "--json")
	run("watch", "inspect", "api-health", "--json")
	exported := run("export", "--watch", "api-health")
	bundle, err := plan.Parse(exported)
	if err != nil || bundle.Watches[0].Revision != r.Plan.Revision {
		t.Fatal(string(exported), err)
	}
	run("export", "--watch", "api-health", "--json")
	backup := filepath.Join(t.TempDir(), "verified.db")
	run("backup", "--out", backup, "--json")
	if _, err := os.Stat(backup); err != nil {
		t.Fatal(err)
	}
	app.Now = func() time.Time { return now.Add(10 * time.Second) }
	if _, err := app.DeliverOne(ctx); err != nil {
		t.Fatal(err)
	} // missing credentials => inspectable terminal failure
	run("delivery", "inspect", "1", "--json")
	run("delivery", "retry", "1", "--json")
	for _, args := range [][]string{{"events", "--cursor", "bad", "--json"}, {"backup", "--out", backup, "--json"}, {"replay", manifest, "--json"}, {"events", "inspect", "absent", "--json"}, {"delivery", "retry", "1", "--json"}} {
		var out, errs bytes.Buffer
		if err := Execute("test", append(args, "--state-dir", dir), &out, &errs); err == nil || !json.Valid(errs.Bytes()) || out.Len() != 0 {
			t.Fatal(args, err, out.String(), errs.String())
		}
	}
	followCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	root := Root("test")
	root.SetContext(followCtx)
	root.SetArgs([]string{"events", "--follow", "--json", "--state-dir", dir})
	var out bytes.Buffer
	root.SetOut(cancelWriter{Writer: &out, cancel: cancel})
	root.SetErr(&out)
	if err := root.Execute(); err != nil || !strings.Contains(out.String(), "cursor") {
		t.Fatal(err, out.String())
	}
}

type cancelWriter struct {
	Writer *bytes.Buffer
	cancel context.CancelFunc
}

func (w cancelWriter) Write(b []byte) (int, error) {
	n, err := w.Writer.Write(b)
	w.cancel()
	return n, err
}
