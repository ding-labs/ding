package plan

import (
	"fmt"
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
	if c.Input.Type != "external" {
		return fmt.Errorf("unsupported log input type")
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
	if c.EmitEvery == "" {
		c.EmitEvery = "1s"
	}
	if c.MinEvents == 0 {
		c.MinEvents = 100
	}
	if c.TextProfile != "common" || c.Framing != "line" || c.EmitEvery != "1s" {
		return fmt.Errorf("logs requires common text profile, line framing, and emitEvery: 1s")
	}
	if _, err := logs.NewParser(logs.ParserConfig{Format: c.Format, LevelField: c.LevelField, Patterns: c.Patterns}); err != nil {
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
