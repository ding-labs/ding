package watchcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/replay"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/spf13/cobra"
)

func request(cmd *cobra.Command, dir string, structured bool, method, path string, body any) (json.RawMessage, error) {
	client, err := control.Connect(dir)
	if err != nil {
		return nil, Fail(cmd.ErrOrStderr(), structured, "daemon_unavailable", err.Error())
	}
	if path == "/v1/backup" {
		client.HTTP.Timeout = 2 * time.Minute
	}
	data, err := client.Call(cmd.Context(), method, path, body)
	if err != nil {
		if cmd.Context().Err() != nil {
			return nil, cmd.Context().Err()
		}
		var api *control.APIError
		if errors.As(err, &api) {
			return nil, Fail(cmd.ErrOrStderr(), structured, api.Code, api.Message)
		}
		return nil, Fail(cmd.ErrOrStderr(), structured, "daemon_unavailable", err.Error())
	}
	return data, nil
}
func present(cmd *cobra.Command, structured bool, data any) error {
	if structured {
		return Write(cmd.OutOrStdout(), data)
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}
func inspectionCommands(root *cobra.Command, dir *string) {
	var doctorJSON bool
	doctor := &cobra.Command{Use: "doctor", Short: "Inspect local store, source, quota, credential and delivery health", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd, *dir, doctorJSON, "GET", "/v1/doctor", nil)
	}}
	doctor.Flags().BoolVar(&doctorJSON, "json", false, "emit a versioned JSON response")
	root.AddCommand(doctor)
	var exportID string
	var exportJSON bool
	export := &cobra.Command{Use: "export --watch ID", Short: "Export the applied watch and current destinations as a reusable manifest", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		data, err := request(cmd, *dir, exportJSON, "GET", "/v1/watches/"+url.PathEscape(exportID)+"/export", nil)
		if err != nil {
			return err
		}
		if exportJSON {
			return Write(cmd.OutOrStdout(), data)
		}
		var out struct {
			Manifest string `json:"manifest"`
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return err
		}
		_, err = io.WriteString(cmd.OutOrStdout(), out.Manifest)
		return err
	}}
	export.Flags().StringVar(&exportID, "watch", "", "watch ID")
	_ = export.MarkFlagRequired("watch")
	export.Flags().BoolVar(&exportJSON, "json", false, "emit a versioned JSON response")
	root.AddCommand(export)
	var backupPath string
	var backupJSON bool
	backup := &cobra.Command{Use: "backup --out FILE", Short: "Create a verified SQLite backup without replacing an existing file", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		path, err := filepath.Abs(backupPath)
		if err != nil {
			return err
		}
		data, err := request(cmd, *dir, backupJSON, "POST", "/v1/backup", map[string]string{"path": path})
		if err != nil {
			return err
		}
		return present(cmd, backupJSON, data)
	}}
	backup.Flags().StringVar(&backupPath, "out", "", "new backup path on the daemon host")
	_ = backup.MarkFlagRequired("out")
	backup.Flags().BoolVar(&backupJSON, "json", false, "emit a versioned JSON response")
	root.AddCommand(backup)
	var eventID, cursor string
	var follow, eventJSON bool
	var limit int
	events := &cobra.Command{Use: "events", Short: "Read retained events with a resumable cursor", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		cmd.SetContext(ctx)
		for {
			q := url.Values{"watch": {eventID}, "cursor": {cursor}, "limit": {strconv.Itoa(limit)}}
			data, err := request(cmd, *dir, eventJSON, http.MethodGet, "/v1/events?"+q.Encode(), nil)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			var page store.EventPage
			if err := json.Unmarshal(data, &page); err != nil {
				return err
			}
			if err := present(cmd, eventJSON, page); err != nil {
				return err
			}
			cursor = page.Cursor
			if !follow {
				return nil
			}
			if page.More {
				continue
			}
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
	}}
	events.Flags().StringVar(&eventID, "watch", "", "filter by watch ID")
	events.Flags().StringVar(&cursor, "cursor", "", "opaque cursor from a previous response")
	events.Flags().BoolVar(&follow, "follow", false, "poll until interrupted; JSON mode emits one envelope per page")
	events.Flags().IntVar(&limit, "limit", 100, "maximum events per page (1..1000)")
	events.Flags().BoolVar(&eventJSON, "json", false, "emit a versioned JSON response")
	var inspectJSON bool
	inspect := &cobra.Command{Use: "inspect EVENT_ID", Short: "Export replay evidence and its verification status", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd, *dir, inspectJSON, "GET", "/v1/events/"+url.PathEscape(args[0]), nil)
	}}
	inspect.Flags().BoolVar(&inspectJSON, "json", false, "emit a versioned JSON response")
	events.AddCommand(inspect)
	var obsJSON bool
	var after int64
	observations := &cobra.Command{Use: "observations EVENT_ID", Short: "Read a page of selected observations referenced by an event", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd, *dir, obsJSON, "GET", "/v1/events/"+url.PathEscape(args[0])+"/observations?after="+strconv.FormatInt(after, 10), nil)
	}}
	observations.Flags().Int64Var(&after, "after", 0, "last observation sequence from the preceding page")
	observations.Flags().BoolVar(&obsJSON, "json", false, "emit a versioned JSON response")
	events.AddCommand(observations)
	root.AddCommand(events)
	deliveries := &cobra.Command{Use: "delivery", Short: "Inspect attempts and retry terminal deliveries"}
	for _, name := range []string{"inspect", "retry"} {
		var structured bool
		var before int64
		command := &cobra.Command{Use: name + " ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			method, path := "GET", "/v1/deliveries/"+url.PathEscape(args[0])+"?before="+strconv.FormatInt(before, 10)
			if cmd.Name() == "retry" {
				method, path = "POST", "/v1/deliveries/"+url.PathEscape(args[0])+"/retry"
			}
			return call(cmd, *dir, structured, method, path, nil)
		}}
		command.Flags().BoolVar(&structured, "json", false, "emit a versioned JSON response")
		if name == "inspect" {
			command.Flags().Int64Var(&before, "before", 0, "attempt cursor from the preceding page")
		}
		deliveries.AddCommand(command)
	}
	root.AddCommand(deliveries)
	root.AddCommand(replayCommand())
}
func replayCommand() *cobra.Command {
	var structured bool
	command := &cobra.Command{Use: "replay EVIDENCE.json", Short: "Verify exported event evidence offline without sources or deliveries", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		f, err := os.Open(args[0])
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "read_failed", "cannot read evidence")
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, 64<<20+1))
		if err != nil || len(raw) > 64<<20 {
			return Fail(cmd.ErrOrStderr(), structured, "invalid_evidence", "evidence exceeds size limit")
		}
		var envelope struct {
			APIVersion string          `json:"apiVersion"`
			Data       json.RawMessage `json:"data"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.APIVersion != "" {
			if envelope.APIVersion != watch.APIVersion {
				return Fail(cmd.ErrOrStderr(), structured, "invalid_evidence", "unsupported evidence version")
			}
			raw = envelope.Data
		}
		var proof replay.Evidence
		if err = json.Unmarshal(raw, &proof); err == nil {
			err = replay.Verify(proof)
		}
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "replay_failed", err.Error())
		}
		if structured {
			return Write(cmd.OutOrStdout(), map[string]any{"eventId": proof.Event.ID, "verified": true})
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Verified event %s\n", proof.Event.ID)
		return err
	}}
	command.Flags().BoolVar(&structured, "json", false, "emit a versioned JSON response")
	return command
}
