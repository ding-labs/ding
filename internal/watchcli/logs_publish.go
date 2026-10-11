package watchcli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/watchrun"
)

func publishLogs(parent context.Context, client control.Client, id string, collector *logs.Collector, diagnostics io.Writer, session *watchrun.LogSession) func(*int) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	epoch := rand.Text()
	sequence := 0
	connected := true
	permanent := false
	path := "/v1/ingest/" + url.PathEscape(id)
	send := func(ctx context.Context, summary logs.Summary, ended bool, exitCode *int) error {
		sequence++
		attempt, stop := context.WithTimeout(ctx, 900*time.Millisecond)
		defer stop()
		if session != nil {
			input := watchrun.LogSnapshot{Session: session.ID, Sequence: int64(sequence), Summary: summary, Ended: ended, ExitCode: exitCode}
			return client.Produce(attempt, "/v1/logs/"+url.PathEscape(id)+"/summary", fmt.Sprintf("logs:%s:%d", session.ID, sequence), input)
		}
		data, _ := json.Marshal(summary)
		var fields map[string]any
		_ = json.Unmarshal(data, &fields)
		fields["epoch"], fields["summary_sequence"] = epoch, sequence
		if ended {
			fields["ready"], fields["warning_match_ratio"], fields["error_match_ratio"] = false, nil, nil
			fields["reason"] = "collection_ended"
		}
		return client.Produce(attempt, path, fmt.Sprintf("logs:%s:%d", epoch, sequence), fields)
	}
	go func() {
		defer close(done)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		var warned, previous time.Time
		var reconnectAt time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case at := <-tick.C:
				if session != nil && !connected {
					if permanent || at.Before(reconnectAt) {
						continue
					}
					reconnectAt = at.Add(5 * time.Second)
					attempt, stop := context.WithTimeout(ctx, time.Second)
					data, err := client.Call(attempt, http.MethodPost, "/v1/logs/"+url.PathEscape(id)+"/session", nil)
					stop()
					if err != nil {
						continue
					}
					var next watchrun.LogSession
					if json.Unmarshal(data, &next) != nil {
						continue
					}
					if !reflect.DeepEqual(session.Config, next.Config) {
						// Release the newly acquired lease; the application keeps
						// running, but a changed parser needs an explicit restart.
						session, sequence, permanent = &next, 0, true
						_ = send(ctx, logs.Summary{}, true, nil)
						fmt.Fprintln(diagnostics, "ding: log watch settings changed; restart capture to use the new configuration")
						continue
					}
					session, sequence, connected = &next, 0, true
					collector.Reset()
					previous = at
					fmt.Fprintln(diagnostics, "ding: log monitoring reconnected; rebuilding the live window")
				}
				if !previous.IsZero() && at.Sub(previous) > 2*time.Second {
					collector.Gap()
				}
				previous = at
				if err := send(ctx, collector.Snapshot(), false, nil); err != nil {
					collector.Gap()
					if session != nil {
						connected = false
						reconnectAt = at.Add(5 * time.Second)
					}
					if time.Since(warned) >= 5*time.Second {
						fmt.Fprintln(diagnostics, "ding: log monitoring interrupted; application output continues")
						warned = time.Now()
					}
				}
			}
		}
	}()
	return func(exitCode *int) {
		cancel()
		<-done
		if session != nil && (!connected || permanent) {
			return
		}
		if err := send(context.Background(), collector.Snapshot(), true, exitCode); err != nil {
			fmt.Fprintln(diagnostics, "ding: final log collection status could not be recorded")
		}
	}
}
