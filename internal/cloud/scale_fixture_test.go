package cloud

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type scaleConfig struct {
	Name                                                           string
	Rate, Seconds, Workers, Workspaces, Outputs, Groups, BodyBytes int
	Condition                                                      string // simple, window, or alerts
	RetainPadding, Defaults, Reads, Restart, Noop                  bool
	Preload                                                        int
	Noisy                                                          int    // percentage of producers assigned to the first workspace
	Receiver                                                       string // fast, slow, outage
}

func (c *scaleConfig) validate() error {
	if c.Workers == 0 {
		c.Workers = 512
	}
	if c.Workspaces == 0 {
		c.Workspaces = 1
	}
	if c.Outputs == 0 {
		c.Outputs = 1
	}
	if c.Groups == 0 {
		c.Groups = c.Outputs
	}
	if c.Condition == "" {
		c.Condition = "simple"
	}
	if c.Name == "" || c.Rate < 1 || c.Rate > 100000 || c.Seconds < 1 || c.Seconds > 3600 || c.Workers < 1 || c.Workers > 1024 || c.Workspaces < 1 || c.Workspaces > 100 || c.Outputs < 1 || c.Outputs > 100 || c.Groups < c.Outputs || c.Groups > 10000 || c.BodyBytes < 0 || c.BodyBytes > 1<<20 || c.Preload < 0 || c.Preload > 500000 || c.Noisy < 0 || c.Noisy > 99 {
		return fmt.Errorf("invalid scale configuration")
	}
	if c.Condition != "simple" && c.Condition != "window" && c.Condition != "alerts" {
		return fmt.Errorf("invalid condition")
	}
	if c.Receiver != "" && c.Receiver != "fast" && c.Receiver != "slow" && c.Receiver != "outage" {
		return fmt.Errorf("invalid receiver")
	}
	return nil
}

func (c scaleConfig) manifest() string {
	fields := "{repo: repo, value: value}"
	if c.RetainPadding {
		fields = "{repo: repo, value: value, padding: padding}"
	}
	condition := "{field: value, operator: gt, value: 95}"
	policy := "{trigger: transition}"
	if c.Condition == "window" {
		condition = `{field: value, numeric: "avg(value) over 1m > 95"}`
	}
	if c.Condition == "alerts" {
		policy = "{trigger: level, interval: 0s}"
	}
	m := fmt.Sprintf("apiVersion: ding.ing/v1alpha1\nkind: Watch\nmetadata: {id: ci}\nspec:\n  source: {type: push, itemsPath: events, fields: %s}\n  groupBy: [repo]\n  condition: %s\n  policy: %s\n  limits: {maxBytes: 1048576, maxEntities: %d, maxOutputs: 100, maxSamples: 10000}\n", fields, condition, policy, c.Groups)
	if c.Receiver != "" {
		m += "  destinations: [{ref: hook}]\n---\napiVersion: ding.ing/v1alpha1\nkind: Destination\nmetadata: {id: hook}\nspec: {type: webhook, urlRef: {env: SCALE_WEBHOOK}}\n"
	}
	return m
}

// Variants rotate through the whole group set. Each body stays immutable in a
// producer fixture, so retries use exactly the originally offered bytes.
func (c scaleConfig) bodies() ([]json.RawMessage, error) {
	n := (c.Groups + c.Outputs - 1) / c.Outputs
	out := make([]json.RawMessage, n)
	for variant := range n {
		events := []map[string]any{}
		for i := range c.Outputs {
			events = append(events, map[string]any{"repo": fmt.Sprintf("group-%d", (variant*c.Outputs+i)%c.Groups), "value": 50})
		}
		if c.Condition == "alerts" {
			for _, e := range events {
				e["value"] = 100
			}
		}
		body := map[string]any{"events": events}
		if c.RetainPadding {
			events[0]["padding"] = ""
		} else {
			body["padding"] = ""
		}
		raw, _ := json.Marshal(body)
		if c.BodyBytes > len(raw) {
			pad := strings.Repeat("x", c.BodyBytes-len(raw))
			if c.RetainPadding {
				events[0]["padding"] = pad
			} else {
				body["padding"] = pad
			}
		}
		out[variant], _ = json.Marshal(body)
	}
	return out, nil
}

func TestScaleFixtureSizesAndGroups(t *testing.T) {
	for _, retained := range []bool{false, true} {
		c := scaleConfig{Name: "fixture", Rate: 10, Seconds: 1, Outputs: 18, Groups: 100, BodyBytes: 600000, RetainPadding: retained}
		if err := c.validate(); err != nil {
			t.Fatal(err)
		}
		bodies, err := c.bodies()
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, b := range bodies {
			if len(b) != 600000 {
				t.Fatal(len(b))
			}
			var d struct{ Events []struct{ Repo string } }
			if err := json.Unmarshal(b, &d); err != nil {
				t.Fatal(err)
			}
			if len(d.Events) != 18 {
				t.Fatal(len(d.Events))
			}
			for _, e := range d.Events {
				seen[e.Repo] = true
			}
		}
		if len(seen) != 100 {
			t.Fatal(len(seen))
		}
	}
}
