// Package notificationstyle gives human-facing notifications consistent status.
// It changes presentation only; event evidence and routing stay authoritative.
package notificationstyle

import (
	"strings"

	"github.com/ding-labs/ding/internal/watch"
)

type Status string

const (
	Warning   Status = "warning"
	Error     Status = "error"
	Recovered Status = "recovered"
)

func ForEvent(event watch.Event) Status {
	// Lifecycle evidence takes priority over potentially stale source fields.
	switch event.Type {
	case "recovered", "source_recovered":
		return Recovered
	case "source_error":
		return Error
	case "gap", "state_reset":
		return Warning
	}
	for _, key := range []string{"severity", "level", "log.level"} {
		value, _ := event.Fields[key].(string)
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "warn", "warning":
			return Warning
		case "error", "err", "fatal", "critical", "crit", "panic":
			return Error
		}
	}
	if event.Type == "firing" {
		return Error
	}
	// Changes, test messages, and administrative events do not imply health.
	return ""
}

func (s Status) Label() string {
	switch s {
	case Warning:
		return "Warning"
	case Error:
		return "Error"
	case Recovered:
		return "Recovered"
	}
	return ""
}

func (s Status) IconColor() string {
	switch s {
	case Warning:
		return "yellow"
	case Error:
		return "red"
	case Recovered:
		return "green"
	}
	return ""
}

func (s Status) IconURL() string {
	if color := s.IconColor(); color != "" {
		return "https://ding.ing/assets/notifications/" + color + ".png"
	}
	return ""
}

func (s Status) Color() int {
	switch s {
	case Warning:
		return 0xe7e37e
	case Error:
		return 0xe77e7e
	case Recovered:
		return 0x7ee787
	}
	return 0
}
