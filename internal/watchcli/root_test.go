package watchcli

import (
	"bytes"
	"encoding/json"
	"github.com/ding-labs/ding/internal/watch"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineCLIContract(t *testing.T) {
	for _, name := range []string{"validate", "explain"} {
		root := Root("test")
		var out, errout bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errout)
		root.SetArgs([]string{name, "../../examples/watches/api-health.yaml", "--json"})
		if err := root.Execute(); err != nil {
			t.Fatal(err, errout.String())
		}
		var e watch.Envelope
		if err := json.Unmarshal(out.Bytes(), &e); err != nil || e.APIVersion != watch.APIVersion || e.Error != nil {
			t.Fatal(out.String(), err)
		}
	}
}
func TestJSONError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(p, []byte("kind: Unicorn"), 0600)
	root := Root("test")
	var out bytes.Buffer
	root.SetErr(&out)
	root.SetArgs([]string{"validate", p, "--json"})
	if err := root.Execute(); err == nil {
		t.Fatal("accepted")
	}
	var e watch.Envelope
	if err := json.Unmarshal(out.Bytes(), &e); err != nil || e.Error == nil || e.Error.Code != "invalid_manifest" {
		t.Fatal(out.String(), err)
	}
}

func TestHumanExplanation(t *testing.T) {
	root := Root("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "../../examples/watches/api-health.yaml"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Permissions: [http-read]")) {
		t.Fatal(out.String())
	}
}
func TestMissingManifest(t *testing.T) {
	root := Root("test")
	var out bytes.Buffer
	root.SetErr(&out)
	root.SetArgs([]string{"validate", filepath.Join(t.TempDir(), "absent")})
	if err := root.Execute(); err == nil {
		t.Fatal("accepted missing file")
	}
	if !bytes.Contains(out.Bytes(), []byte("cannot read manifest")) {
		t.Fatal(out.String())
	}
}

func TestArgumentFailuresHaveEnvelope(t *testing.T) {
	for _, args := range [][]string{{"validate", "--json"}, {"unknown", "--json"}, {"validate", "file", "--bogus", "--json"}} {
		var out, errout bytes.Buffer
		if err := Execute("test", args, &out, &errout); err == nil {
			t.Fatal("accepted", args)
		}
		var e watch.Envelope
		if err := json.Unmarshal(errout.Bytes(), &e); err != nil || e.Error == nil || e.Error.Code != "invalid_arguments" {
			t.Fatal(errout.String(), err)
		}
	}
}
func TestExecuteSuccessAndReportedError(t *testing.T) {
	var out bytes.Buffer
	if err := Execute("test", []string{"validate", "../../examples/watches/api-health.yaml", "--json"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Execute("test", []string{"validate", filepath.Join(t.TempDir(), "missing"), "--json"}, &out, &out); err == nil {
		t.Fatal("missing")
	}
	var e watch.Envelope
	if err := json.Unmarshal(out.Bytes(), &e); err != nil {
		t.Fatal("multiple error envelopes", out.String())
	}
}
