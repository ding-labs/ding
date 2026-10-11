package watchrun

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

var ErrLogSessionBusy = errors.New("a collector is already connected")

type LogSession struct {
	ID         string          `json:"id"`
	Revision   string          `json:"revision"`
	Generation int64           `json:"generation"`
	Config     watch.LogSource `json:"config"`
}

type LogSnapshot struct {
	Session  string             `json:"session"`
	Sequence int64              `json:"sequence"`
	Summary  logs.Summary       `json:"summary"`
	Ended    bool               `json:"ended,omitempty"`
	ExitCode *int               `json:"exitCode,omitempty"`
	Failure  string             `json:"failure,omitempty"`
	Position *logs.FilePosition `json:"position,omitempty"`
}

// The existing transactional source cursor stores this versioned checkpoint.
// It contains no raw log output and is committed with observations and alerts.
type logCheckpoint struct {
	Revision   string             `json:"revision"`
	Generation int64              `json:"generation"`
	Version    int                `json:"version"`
	ID         string             `json:"id"`
	Epoch      string             `json:"epoch"`
	Sequence   int64              `json:"sequence"`
	Started    time.Time          `json:"started"`
	Last       time.Time          `json:"last"`
	Status     string             `json:"status"`
	Position   *logs.FilePosition `json:"position,omitempty"`
}

func (c logCheckpoint) encode() string { b, _ := json.Marshal(c); return string(b) }

func (a *App) StartLogSession(ctx context.Context, id string) (LogSession, error) {
	return a.startLogSession(ctx, id, nil)
}

func (a *App) startLogSession(ctx context.Context, id string, owned *store.WatchRecord) (LogSession, error) {
	r, err := a.Record(ctx, id)
	if err != nil {
		return LogSession{}, err
	}
	if r.Status != "running" || r.Plan.Definition.Spec.Source.Type != "logs" {
		return LogSession{}, store.ErrStale
	}
	if owned == nil && r.Plan.Definition.Spec.Source.Logs.Input.Type != "external" || owned != nil && (r.Generation != owned.Generation || r.Plan.Revision != owned.Plan.Revision) {
		return LogSession{}, store.ErrStale
	}
	now := a.Now()
	var previous logCheckpoint
	_ = json.Unmarshal([]byte(r.Cursor), &previous)
	if previous.Epoch == a.logEpoch && previous.Revision == r.Plan.Revision && previous.Generation == r.Generation && previous.Status == "active" && now.Before(previous.Last.Add(5*time.Second)) {
		return LogSession{}, ErrLogSessionBusy
	}
	next := logCheckpoint{Version: 1, ID: rand.Text(), Epoch: a.logEpoch, Revision: r.Plan.Revision, Generation: r.Generation, Started: now, Last: now, Status: "active"}
	o := watch.Observation{Health: "waiting", Detail: "warming_up", Fields: map[string]any{"reason": "warming_up"}}
	if r.Cursor != "" {
		o.Health, o.Detail = "gap", "collector_reconnected"
	}
	_, err = a.accept(ctx, r, source.Batch{ExpectedCursor: &r.Cursor, Cursor: next.encode(), FreshUntil: now.Add(time.Second), Observations: []watch.Observation{o}}, "log-start:"+next.ID, a.Now)
	if err != nil {
		return LogSession{}, err
	}
	return LogSession{ID: next.ID, Revision: r.Plan.Revision, Generation: r.Generation, Config: *r.Plan.Definition.Spec.Source.Logs}, nil
}

func (a *App) AcceptLogSnapshot(ctx context.Context, id string, input LogSnapshot) (Receipt, error) {
	r, err := a.Record(ctx, id)
	if err != nil {
		return Receipt{}, err
	}
	if r.Status != "running" || r.Plan.Definition.Spec.Source.Type != "logs" {
		return Receipt{}, store.ErrStale
	}
	var checkpoint logCheckpoint
	if json.Unmarshal([]byte(r.Cursor), &checkpoint) != nil || checkpoint.Version != 1 || checkpoint.ID != input.Session || checkpoint.Epoch != a.logEpoch || checkpoint.Generation != r.Generation || checkpoint.Revision != r.Plan.Revision || input.Sequence < 1 || input.Sequence > 1<<40 {
		return Receipt{}, store.ErrStale
	}
	key := fmt.Sprintf("log:%s:%d", input.Session, input.Sequence)
	// A retry may follow later summaries. Only an existing durable receipt can
	// acknowledge an old sequence; an impossible cursor prevents a fresh write.
	if input.Sequence <= checkpoint.Sequence {
		impossible := "duplicate-only"
		return a.accept(ctx, r, source.Batch{ExpectedCursor: &impossible, Observations: []watch.Observation{{Health: "waiting"}}}, key, a.Now)
	}
	if checkpoint.Status != "active" || input.Sequence != checkpoint.Sequence+1 || !a.Now().Before(checkpoint.Last.Add(5*time.Second)) {
		return Receipt{}, store.ErrStale
	}
	now := a.Now()
	o := watch.Observation{Health: "waiting", Detail: "collection_ended", Fields: map[string]any{"reason": "collection_ended", "source_status": "ended"}}
	freshUntil := now.Add(time.Second)
	if input.Ended {
		checkpoint.Status = "ended"
		if input.Failure != "" {
			switch input.Failure {
			case "log_file_unavailable", "log_reader_ended", "log_reader_failed", "log_capture_failed":
			default:
				return Receipt{}, ErrInput
			}
			checkpoint.Status, o.Health, o.Detail = "failed", "unknown", input.Failure
			o.Fields["reason"], o.Fields["source_status"] = input.Failure, "failed"
		}
		if input.ExitCode != nil {
			if *input.ExitCode < 0 || *input.ExitCode > 255 {
				return Receipt{}, ErrInput
			}
			o.Fields["exit_code"] = *input.ExitCode
		}
	} else {
		config := r.Plan.Definition.Spec.Source.Logs
		window, _ := time.ParseDuration(config.Window)
		s, err := input.Summary.Assess(logs.WindowConfig{Window: window, MinEvents: config.MinEvents, MinCoverage: config.MinCoverage}, checkpoint.Started, now)
		if err != nil {
			return Receipt{}, fmt.Errorf("%w: %s", ErrInput, err)
		}
		o.Fields, err = s.Fields()
		if err != nil {
			return Receipt{}, ErrInput
		}
		o.Detail = s.Reason
		switch s.Reason {
		case "ready":
			o.Health = "ok"
		case "invalid_records", "collection_gap":
			o.Health = "unknown"
		}
		o.Fields["source_status"] = "active"
		freshUntil = s.WindowEnd.Add(3 * time.Second)
	}
	if input.Position != nil {
		p := input.Position
		if r.Plan.Definition.Spec.Source.Logs.Input.Type != "file" || len(p.Identity) > 128 || p.Offset < 0 || p.LagBytes < 0 || p.SkippedFrom < 0 || p.SkippedTo < 0 {
			return Receipt{}, ErrInput
		}
		checkpoint.Position = p
		o.Fields["lag_bytes"], o.Fields["file_offset"] = p.LagBytes, p.Offset
	}
	config := r.Plan.Definition.Spec.Source.Logs
	o.Fields["input_type"], o.Fields["parser_profile"], o.Fields["framing"] = config.Input.Type, config.TextProfile, config.Framing
	o.Fields["session"], o.Fields["summary_sequence"] = input.Session, input.Sequence
	checkpoint.Sequence, checkpoint.Last = input.Sequence, now
	return a.accept(ctx, r, source.Batch{ExpectedCursor: &r.Cursor, FreshUntil: freshUntil, Cursor: checkpoint.encode(), Observations: []watch.Observation{o}}, key, a.Now)
}

func (a *App) expireLogSessions(ctx context.Context, records []store.WatchRecord, now time.Time) {
	for _, r := range records {
		if r.Status != "running" || r.Plan.Definition.Spec.Source.Type != "logs" {
			continue
		}
		var c logCheckpoint
		if json.Unmarshal([]byte(r.Cursor), &c) != nil || c.Status != "active" || c.Epoch == a.logEpoch && now.Before(c.Last.Add(5*time.Second)) {
			continue
		}
		c.Status = "stalled"
		_, err := a.accept(ctx, r, source.Batch{ExpectedCursor: &r.Cursor, Cursor: c.encode(), Observations: []watch.Observation{{Health: "unknown", Detail: "collector_stalled", Fields: map[string]any{"source_status": "stalled", "reason": "collector_stalled"}}}}, "log-stalled:"+c.ID, a.Now)
		if err != nil && !errors.Is(err, store.ErrStale) {
			a.note(err)
		}
	}
}
