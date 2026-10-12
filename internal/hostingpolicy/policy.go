// Package hostingpolicy validates cloud eligibility offline, before sign-in or upload.
package hostingpolicy

import (
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/cloud/egress"
	"github.com/ding-labs/ding/internal/integrationpolicy"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watchrun"
)

const MaxManifestBytes = integrationpolicy.CloudManifestBytes

func Limits() watchrun.Limits         { return (Capacity{}).Limits() }
func Policy(bundle plan.Bundle) error { return (Capacity{}).Policy(bundle) }
func (c Capacity) Policy(bundle plan.Bundle) error {
	c = c.Effective()
	if err := c.Validate(); err != nil {
		return err
	}
	if len(bundle.Watches) > c.MaxWatches || len(bundle.Destinations) > c.MaxDestinations {
		return fmt.Errorf("manifest exceeds configured workspace capacity")
	}
	for _, compiled := range bundle.Watches {
		w := compiled.Definition.Spec
		switch w.Source.Type {
		case "push":
		case "http":
			if w.Limits.MaxBytes > 64<<10 {
				return fmt.Errorf("cloud HTTP acquisition has a separate 64 KiB response limit")
			}
			if w.Source.URLRef != nil {
				return fmt.Errorf("cloud HTTP sources require explicit public URLs")
			}
			if err := egress.URL(w.Source.URL); err != nil {
				return err
			}
			every, err := time.ParseDuration(w.Source.Every)
			if err != nil || every < time.Duration(c.MinPollSeconds)*time.Second {
				return fmt.Errorf("poll interval is below configured capacity")
			}
			timeout, err := time.ParseDuration(w.Source.Timeout)
			if err != nil || timeout > 10*time.Second {
				return fmt.Errorf("cloud source timeout must be at most ten seconds")
			}
		default:
			return fmt.Errorf("cloud supports public HTTP and authenticated push sources")
		}
		if w.Source.JQ != "" {
			return fmt.Errorf("cloud uses bounded field projection; custom jq programs remain available locally")
		}
		if w.Limits.MaxBytes > c.MaxBodyBytes || w.Limits.MaxEntities > c.MaxEntities || w.Limits.MaxSamples > c.MaxSamples || w.Limits.MaxOutputs > c.MaxOutputs {
			return fmt.Errorf("watch exceeds configured cloud body, entity, sample, or output capacity")
		}
		if len(w.Destinations) > 32 || len(w.Source.Headers) > 10 || len(w.Source.Fields) > 100 {
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

// TransferPolicy is intentionally narrower than creation eligibility: push
// producers must be reconfigured and verified at their new destination.
func TransferPolicy(bundle plan.Bundle) error {
	if err := Policy(bundle); err != nil {
		return err
	}
	for _, w := range bundle.Watches {
		if w.Definition.Spec.Source.Type != "http" {
			return fmt.Errorf("automatic transfer requires HTTP; export/create the push watch and reconfigure its producer")
		}
	}
	return nil
}
