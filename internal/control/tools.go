package control

import (
	"context"
	"encoding/json"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/replay"
	"github.com/ding-labs/ding/internal/watchrun"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Line     int    `json:"line,omitempty"`
}
type CompileResult struct {
	Valid        bool                        `json:"valid"`
	Bundle       plan.Bundle                 `json:"bundle"`
	Descriptions []plan.Description          `json:"descriptions"`
	Diagnostics  []Diagnostic                `json:"diagnostics"`
	Credentials  []watchrun.CredentialHealth `json:"credentials"`
}
type ToolRequest struct {
	Manifest string          `json:"manifest"`
	Fixture  string          `json:"fixture,omitempty"`
	Watch    string          `json:"watch,omitempty"`
	Evidence json.RawMessage `json:"evidence,omitempty"`
}
type Verification struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

var yamlLine = regexp.MustCompile(`line (\d+):`)

func compileTool(app *watchrun.App, manifest string) CompileResult {
	out := CompileResult{Descriptions: []plan.Description{}, Diagnostics: []Diagnostic{}, Credentials: []watchrun.CredentialHealth{}}
	b, err := plan.Parse([]byte(manifest))
	if err != nil {
		d := Diagnostic{Code: "invalid_manifest", Severity: "error", Path: "", Message: err.Error()}
		if m := yamlLine.FindStringSubmatch(err.Error()); len(m) == 2 {
			d.Line, _ = strconv.Atoi(m[1])
		}
		out.Diagnostics = append(out.Diagnostics, d)
		return out
	}
	out.Valid = true
	out.Bundle = b
	out.Credentials = app.BundleCredentials(b, nil)
	for _, w := range b.Watches {
		out.Descriptions = append(out.Descriptions, plan.Describe(w))
	}
	return out
}
func decodeBounded(w http.ResponseWriter, r *http.Request, out any, max int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func toolRoutes(mux *http.ServeMux, app *watchrun.App) {
	slots := make(chan struct{}, 2)
	for _, kind := range []string{"compile", "test", "replay"} {
		mux.HandleFunc("POST /v1/tools/"+kind, func(w http.ResponseWriter, r *http.Request) {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				fail(w, 429, "tool_busy", "two console tools are already running; try again shortly")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			var req ToolRequest
			bound := int64(12 << 20)
			if kind == "compile" {
				bound = 2 << 20
			}
			if kind == "replay" {
				bound = 64 << 20
			}
			if decodeBounded(w, r, &req, bound) != nil {
				fail(w, 400, "invalid_request", "invalid or oversized tool request")
				return
			}
			if len(req.Manifest) > 1<<20 || len(req.Fixture) > 8<<20 {
				fail(w, 413, "input_too_large", "manifest limit is 1 MiB; fixture limit is 8 MiB")
				return
			}
			switch kind {
			case "compile":
				write(w, 200, compileTool(app, req.Manifest), nil)
			case "test":
				b, err := plan.Parse([]byte(req.Manifest))
				if err != nil {
					fail(w, 400, "invalid_manifest", err.Error())
					return
				}
				var selected *plan.Compiled
				for i := range b.Watches {
					if b.Watches[i].Definition.Metadata.ID == req.Watch || (req.Watch == "" && len(b.Watches) == 1) {
						selected = &b.Watches[i]
					}
				}
				if selected == nil {
					fail(w, 400, "invalid_watch", "select one watch from this bundle")
					return
				}
				report, err := replay.RunContext(ctx, *selected, strings.NewReader(req.Fixture))
				if err != nil {
					fail(w, 400, "invalid_fixture", err.Error())
					return
				}
				write(w, 200, report, nil)
			case "replay":
				raw := req.Evidence
				var envelope struct {
					APIVersion string          `json:"apiVersion"`
					Data       json.RawMessage `json:"data"`
				}
				if json.Unmarshal(raw, &envelope) == nil && len(envelope.Data) > 0 {
					raw = envelope.Data
				}
				var e replay.Evidence
				if json.Unmarshal(raw, &e) != nil || e.Event.ID == "" || e.Definition.Metadata.ID == "" {
					write(w, 200, Verification{"malformed", "Provide a recorded evidence object or API envelope."}, nil)
					return
				}
				if e.Checkpoint == nil {
					write(w, 200, Verification{"unavailable", "This event has no replay checkpoint."}, nil)
					return
				}
				if err := replay.Verify(e); err != nil {
					write(w, 200, Verification{"mismatch", err.Error()}, nil)
					return
				}
				write(w, 200, Verification{"verified", "The checkpoint reproduced the recorded event exactly."}, nil)
			}
		})
	}
}
