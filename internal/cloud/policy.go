// Package cloud hosts the same deterministic engine behind an explicit tenant
// and resource boundary. It is not imported by the local CLI or MCP adapter.
package cloud

import (
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/cloud/egress"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watchrun"
)

const MaxManifestBytes = 64 << 10

func Limits() watchrun.Limits {
	return watchrun.Limits{MaxWatches: 3, MaxPending: 100, MaxBytes: 64 << 20, Retention: 7 * 24 * time.Hour}
}

func Policy(bundle plan.Bundle) error {
	if len(bundle.Watches) > 3 || len(bundle.Destinations) > 3 {
		return fmt.Errorf("cloud beta allows at most three watches and three destinations per manifest")
	}
	for _, compiled := range bundle.Watches {
		w := compiled.Definition.Spec
		if w.Source.Type != "http" || w.Source.URLRef != nil {
			return fmt.Errorf("cloud beta supports public HTTP sources with explicit URLs only")
		}
		if err := egress.URL(w.Source.URL); err != nil {
			return err
		}
		every, err := time.ParseDuration(w.Source.Every)
		if err != nil || every < 5*time.Minute {
			return fmt.Errorf("cloud beta requires an interval of at least five minutes; review a cadence change explicitly")
		}
		timeout, err := time.ParseDuration(w.Source.Timeout)
		if err != nil || timeout > 10*time.Second {
			return fmt.Errorf("cloud source timeout must be at most ten seconds")
		}
		if w.Source.JQ != "" {
			return fmt.Errorf("cloud beta uses bounded field projection; custom jq programs remain available locally")
		}
		if w.Limits.MaxBytes > 64<<10 || w.Limits.MaxEntities > 10 || w.Limits.MaxSamples > 100 || w.Limits.MaxOutputs > 10 {
			return fmt.Errorf("cloud watch limits: 64 KiB responses, 10 entities, 100 samples, 10 projected outputs; set these explicitly")
		}
		if len(w.Destinations) > 3 || len(w.Source.Headers) > 10 || len(w.Source.Fields) > 10 {
			return fmt.Errorf("cloud watch exceeds destination or projection limits")
		}
	}
	for _, compiled := range bundle.Destinations {
		d := compiled.Definition.Spec
		if d.Type != "webhook" && d.Type != "slack" && d.Type != "discord" {
			return fmt.Errorf("cloud delivery requires webhook, Slack, or Discord; desktop delivery cannot reach a closed laptop")
		}
		age, _ := time.ParseDuration(d.MaxAge)
		if d.MaxAttempts > 8 || age > 24*time.Hour || len(d.Headers) > 10 {
			return fmt.Errorf("cloud destinations allow at most eight attempts, 24 hours, and ten headers")
		}
	}
	return nil
}
