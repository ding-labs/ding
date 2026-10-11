package plan

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/watch"
)

func compileLogs(s *watch.Source) error {
	if s.Logs == nil || s.URL != "" || s.URLRef != nil || len(s.Argv) > 0 || s.Directory != "" || len(s.Env) > 0 || len(s.Headers) > 0 || s.JQ != "" || len(s.Fields) > 0 || s.Every != "" || s.Timeout != "" || s.ObservedAtField != "" {
		return fmt.Errorf("logs requires source.logs and does not accept polling or projection fields")
	}
	c := s.Logs
	if c.Input.Type == "" {
		c.Input.Type = "external"
	}
	if err := compileLogInput(&c.Input); err != nil {
		return err
	}
	if c.Format == "" {
		c.Format = "auto"
	}
	if c.LevelField == "" {
		c.LevelField = "level"
	}
	if c.TextProfile == "" {
		c.TextProfile = "common"
	}
	if c.Framing == "" {
		c.Framing = "line"
	}
	if c.Window == "" {
		c.Window = "60s"
	}
	if c.Retention == "" {
		c.Retention = "1h"
	}
	retention, err := time.ParseDuration(c.Retention)
	if err != nil || retention < time.Minute || retention > 7*24*time.Hour {
		return fmt.Errorf("log summary retention must be between 1m and 168h")
	}
	c.Retention = retention.String()
	if c.EmitEvery == "" {
		c.EmitEvery = "1s"
	}
	if c.MinEvents == 0 {
		c.MinEvents = 100
	}
	if c.TextProfile != "common" || c.EmitEvery != "1s" {
		return fmt.Errorf("logs requires common text profile and emitEvery: 1s")
	}
	framing := logs.FrameConfig{Mode: c.Framing}
	if c.Multiline != nil {
		m := c.Multiline
		if m.MaxLines == 0 {
			m.MaxLines = 100
		}
		if m.FlushAfter == "" {
			m.FlushAfter = "1s"
		}
		framing.Start, framing.Continue, framing.MaxLines, framing.FlushAfter = m.Start, m.Continue, m.MaxLines, m.FlushAfter
	}
	if c.Format == "jsonl" && c.Framing == "multiline" {
		return fmt.Errorf("JSONL uses one object per line")
	}
	if _, err := logs.NewConfiguredFramer(framing, func([]byte, bool) {}); err != nil {
		return err
	}
	if _, err := logs.NewParser(logs.ParserConfig{Format: c.Format, LevelField: c.LevelField, Patterns: c.Patterns, LevelMap: c.LevelMap}); err != nil {
		return err
	}
	window, err := time.ParseDuration(c.Window)
	if err != nil {
		return fmt.Errorf("invalid log window")
	}
	if _, err := logs.NewWindow(logs.WindowConfig{Window: window, MinEvents: c.MinEvents, MinCoverage: c.MinCoverage}, time.Time{}); err != nil {
		return err
	}
	c.Window = window.String()
	return nil
}

func compileLogInput(i *watch.LogInput) error {
	switch i.Type {
	case "external":
		if i.Path != "" || i.Start != "" || len(i.Argv) > 0 || i.Directory != "" || len(i.Env) > 0 || i.ReconnectEvery != "" {
			return fmt.Errorf("external logs do not accept file or command settings")
		}
	case "file":
		if !filepath.IsAbs(i.Path) || strings.ContainsRune(i.Path, 0) || len(i.Argv) > 0 || i.Directory != "" || len(i.Env) > 0 || i.ReconnectEvery != "" {
			return fmt.Errorf("file logs require an absolute path without command settings")
		}
		if i.Start == "" {
			i.Start = "end"
		}
		if i.Start != "end" {
			return fmt.Errorf("file logs support only live collection from end")
		}
	case "command":
		if i.Path != "" || i.Start != "" || len(i.Argv) == 0 || len(i.Argv) > 100 || i.Argv[0] == "" || !filepath.IsAbs(i.Directory) {
			return fmt.Errorf("log reader requires explicit argv and absolute directory without file settings")
		}
		for _, arg := range i.Argv {
			if len(arg) > 8192 || strings.ContainsRune(arg, 0) {
				return fmt.Errorf("invalid log reader argument")
			}
		}
		if err := refs(i.Env); err != nil {
			return err
		}
		for name := range i.Env {
			if !envName.MatchString(name) {
				return fmt.Errorf("invalid log reader environment name")
			}
		}
		if i.ReconnectEvery == "" {
			i.ReconnectEvery = "5s"
		}
		d, err := time.ParseDuration(i.ReconnectEvery)
		if err != nil || d < 5*time.Second || d > 5*time.Minute {
			return fmt.Errorf("log reader reconnectEvery must be between 5s and 5m")
		}
		i.ReconnectEvery = d.String()
	default:
		return fmt.Errorf("unsupported log input type")
	}
	return nil
}
