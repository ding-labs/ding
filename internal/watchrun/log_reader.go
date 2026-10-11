package watchrun

import (
	"context"
	"io"
	"runtime"
	"time"

	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
)

// runLogReader owns a single live-only observer attempt. Failure ends the
// session; the scheduler starts a fresh window after the reconnect interval.
func (a *App) runLogReader(ctx context.Context, r store.WatchRecord) {
	id := r.Plan.Definition.Metadata.ID
	session, err := a.startLogSession(ctx, id, &r)
	if err != nil {
		return
	}
	config := session.Config
	window, _ := time.ParseDuration(config.Window)
	c, err := logs.NewCollector(logs.ParserConfig{Format: config.Format, LevelField: config.LevelField, Patterns: config.Patterns}, logs.WindowConfig{Window: window, MinEvents: config.MinEvents, MinCoverage: config.MinCoverage}, LogFraming(config))
	if err != nil {
		a.note(err)
		return
	}
	defer c.Close()
	readerCtx, stop := context.WithCancel(ctx)
	defer stop()
	sequence := int64(0)
	var file *logs.FileFollower
	publish := func(failure string) error {
		sequence++
		s := LogSnapshot{Session: session.ID, Sequence: sequence, Summary: c.Snapshot(), Ended: failure != "", Failure: failure}
		if file != nil {
			pos := file.Position()
			s.Position = &pos
		}
		attempt, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
		defer cancel()
		_, err := a.AcceptLogSnapshot(attempt, id, s)
		return err
	}
	if runtime.GOOS == "windows" {
		_ = publish("log_capture_failed")
		return
	}
	completed := make(chan string, 1)
	if config.Input.Type == "file" {
		file = &logs.FileFollower{Path: config.Input.Path, Collector: c}
		go func() { _ = file.Run(readerCtx); completed <- "log_file_unavailable" }()
	} else {
		env := []string{}
		for name, ref := range config.Input.Env {
			value, err := source.Resolve(&ref, a.Lookup)
			if err != nil {
				_ = publish("log_reader_failed")
				return
			}
			env = append(env, name+"="+value)
		}
		go func() {
			result := c.Exec(readerCtx, logs.ExecOptions{Argv: config.Input.Argv, Directory: config.Input.Directory, Env: env, Stdout: io.Discard, Stderr: io.Discard})
			failure := "log_reader_ended"
			if result.Err != nil || result.ExitCode != 0 {
				failure = "log_reader_failed"
			}
			completed <- failure
		}()
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var previous time.Time
	for {
		select {
		case <-ctx.Done():
			stop()
			<-completed
			return
		case failure := <-completed:
			if ctx.Err() == nil {
				_ = publish(failure)
			}
			return
		case at := <-tick.C:
			if !previous.IsZero() && at.Sub(previous) > 2*time.Second {
				c.Gap()
			}
			previous = at
			if err := publish(""); err != nil {
				// Do not advance the reader under an uncertain/stale checkpoint.
				// Stalled-session detection reports failed commits separately.
				stop()
				<-completed
				return
			}
		}
	}
}
