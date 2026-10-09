// Package migrate converts a supported legacy rule as a whole or reports why it
// cannot. Conversion is pure: credentials are never resolved and watches never run.
package migrate

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
	"gopkg.in/yaml.v3"
)

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Binding struct {
	Notifier    string `json:"notifier"`
	Environment string `json:"environment"`
	From        string `json:"from"`
}
type RuleReport struct {
	Name     string  `json:"name"`
	ID       string  `json:"id"`
	Status   string  `json:"status"`
	File     string  `json:"file,omitempty"`
	Issues   []Issue `json:"issues"`
	Warnings []Issue `json:"warnings"`
}
type Report struct {
	Converted      int          `json:"converted"`
	Unsupported    int          `json:"unsupported"`
	ReviewRequired bool         `json:"reviewRequired"`
	State          string       `json:"state"`
	Warnings       []Issue      `json:"warnings"`
	Bindings       []Binding    `json:"bindings"`
	Rules          []RuleReport `json:"rules"`
}
type File struct {
	Name    string
	Content []byte
}
type Result struct {
	Report Report
	Files  []File
}

var envRef = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
var simpleAction = regexp.MustCompile(`{{\s*\.([A-Za-z_][A-Za-z0-9_]*)\s*}}`)
var nonID = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func identifier(name string) string {
	stem := strings.Trim(nonID.ReplaceAllString(name, "-"), "-")
	if len(stem) > 48 {
		stem = stem[:48]
	}
	if stem == "" {
		stem = "rule"
	}
	hash := sha256.Sum256([]byte(name))
	return fmt.Sprintf("legacy-%s-%x", stem, hash[:])
}
func issue(code, message string) Issue { return Issue{Code: code, Message: message} }
func Convert(raw []byte) (Result, error) {
	result := Result{Report: Report{ReviewRequired: true, State: "New watch state: legacy baselines, cooldowns and snapshots are not imported.", Rules: []RuleReport{}, Bindings: []Binding{}, Warnings: []Issue{
		issue("producer_rewire", "Each converted rule is an authenticated push watch. Send the original JSON object separately to each watch's /v1/ingest/{id}; old /ingest, stdin and Prometheus text are not supported."),
		issue("accepted_time", "Conditions use accepted input time; legacy provider timestamps no longer control windows. Replay with recorded accepted times before applying."),
		issue("delivery_contract", "Webhook/console payloads use versioned events; provider formatting changes. Delivery is durable and at least once; verify downstream receivers."),
		issue("runtime_configuration", "Server/auth settings, snapshot persistence and alert-log paths are replaced by daemon flags, private credentials, SQLite and event history."),
	}}, Files: []File{}}
	if len(raw) > 1<<20 {
		return result, fmt.Errorf("legacy config exceeds 1 MiB")
	}
	var node yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	if dec.Decode(&node) != nil {
		return result, fmt.Errorf("invalid legacy YAML")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return result, fmt.Errorf("expected one legacy document")
	}
	if err := checkNode(&node); err != nil {
		return result, err
	}
	encoded, err := yaml.Marshal(&node)
	if err != nil {
		return result, err
	}
	var cfg legacyConfig
	dec = yaml.NewDecoder(bytes.NewReader(encoded))
	dec.KnownFields(true)
	if dec.Decode(&cfg) != nil {
		return result, fmt.Errorf("invalid legacy configuration shape or unknown fields")
	}
	if len(cfg.Rules) == 0 || len(cfg.Rules) > 1000 {
		return result, fmt.Errorf("legacy config must contain 1..1000 rules")
	}
	names := map[string]bool{}
	bindingSeen := map[string]bool{}
	for _, r := range cfg.Rules {
		if r.Name == "" || names[r.Name] {
			return result, fmt.Errorf("legacy rule names must be nonempty and unique")
		}
		names[r.Name] = true
		rr := RuleReport{Name: r.Name, ID: identifier(r.Name), Status: "unsupported", Issues: []Issue{}, Warnings: []Issue{}}
		add := func(code, msg string) { rr.Issues = append(rr.Issues, issue(code, msg)) }
		if r.Guard != nil {
			add("http_guard", "Live HTTP guards require redesign; no guard has been removed silently.")
		}
		if r.Mode != "" && r.Mode != "during-run" {
			add("run_lifecycle", "End-of-run or unknown execution modes are unsupported.")
		}
		if r.Match["metric"] == "run.exit" || r.Match["metric"] == "run.summary" {
			add("synthetic_run_event", "The watch daemon does not synthesize command-wrapper run events.")
		}
		if cfg.Server.Format == "prometheus" || (cfg.Server.Format != "" && cfg.Server.Format != "auto" && cfg.Server.Format != "json") {
			add("input_format", "Convert this input to JSON upstream; the declared format cannot be preserved.")
		}
		ruleJSON, _ := json.Marshal(r)
		if strings.Contains(string(ruleJSON), "${") || strings.Contains(cfg.Server.JQ, "${") {
			add("environment_expansion", "Only whole notifier URL environment references can be converted without resolving values. Make rule expressions explicit.")
		}
		expr, e := condition.ParseExpression(r.Condition)
		if e != nil {
			add("numeric_condition", "Condition is not supported numeric grammar.")
		} else {
			for _, w := range expr.Windows() {
				if w.RunBounded {
					add("run_window", "Run-lifetime aggregation has no equivalent in a persistent watch.")
					break
				}
			}
			if len(expr.Windows()) > 0 {
				rr.Warnings = append(rr.Warnings, issue("window_boundary", "Windows are (now-window, now]; legacy included the exact lower boundary. A full sample budget is unknown instead of truncating old samples."))
			}
		}
		cooldown := time.Duration(0)
		if r.Cooldown != "" {
			cooldown, e = time.ParseDuration(r.Cooldown)
			if e != nil || cooldown < 0 {
				add("cooldown", "Cooldown is not a nonnegative duration.")
			}
		}
		jq, message, match, e := projection(cfg.Server.JQ, r)
		if e != nil {
			add("message_template", e.Error())
		}
		destinations := []plan.CompiledDestination{}
		targets := []watch.Target{}
		bindings := []Binding{}
		destSeen := map[string]bool{}
		for _, target := range r.Alert {
			if destSeen[target.Notifier] {
				continue
			}
			destSeen[target.Notifier] = true
			d, b, err := destination(target.Notifier, cfg.Notifiers)
			if err != nil {
				add("destination", err.Error())
				continue
			}
			destinations = append(destinations, d)
			targets = append(targets, watch.Target{Ref: d.Definition.Metadata.ID, Events: []string{"firing"}})
			if b != nil {
				bindings = append(bindings, *b)
			}
		}
		maxEntities := cfg.Server.MaxLabelSets
		if maxEntities == 0 {
			maxEntities = 10000
		}
		definition := watch.Definition{APIVersion: watch.APIVersion, Kind: "Watch", Metadata: watch.Metadata{ID: rr.ID, Name: r.Name}, Spec: watch.Spec{
			Source: watch.Source{Type: "push", JQ: jq}, Condition: watch.Condition{Field: "value", Numeric: r.Condition}, Policy: watch.Policy{Trigger: "level", Interval: cooldown.String(), Consecutive: 1, RecoverAfter: 1, OnUnknown: "hold-incident"}, GroupBy: []string{"legacy_group"}, Match: match, Message: message, Destinations: targets,
			Limits: watch.Limits{MaxEntities: maxEntities, MaxSamples: cfg.Server.MaxBufferSize, MaxBytes: cfg.Server.MaxBodyBytes, IdleTTL: cfg.Server.StateIdleTTL},
		}}
		p, e := plan.Compile(definition)
		if e != nil {
			add("watch_capability", "The rule, transform or resource limits cannot compile as a watch; inspect the legacy limits and unsupported template/condition report.")
		}
		if len(rr.Issues) == 0 {
			var output bytes.Buffer
			enc := yaml.NewEncoder(&output)
			enc.SetIndent(2)
			if err := enc.Encode(p.Definition); err != nil {
				return result, err
			}
			for _, d := range destinations {
				if err := enc.Encode(d.Definition); err != nil {
					return result, err
				}
			}
			if err := enc.Close(); err != nil {
				return result, err
			}
			if _, err := plan.Parse(output.Bytes()); err != nil {
				add("manifest_limits", "Converted rule exceeds manifest resource or size limits.")
			} else {
				rr.Status = "converted"
				rr.File = rr.ID + ".yaml"
				result.Files = append(result.Files, File{rr.File, output.Bytes()})
				result.Report.Converted++
				for _, b := range bindings {
					if !bindingSeen[b.Notifier] {
						result.Report.Bindings = append(result.Report.Bindings, b)
						bindingSeen[b.Notifier] = true
					}
				}
			}
		}
		if rr.Status == "unsupported" {
			result.Report.Unsupported++
		}
		result.Report.Rules = append(result.Report.Rules, rr)
	}
	return result, nil
}
func checkNode(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode {
		return fmt.Errorf("legacy YAML aliases are unsupported")
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag != "!!str" || seen[k.Value] {
				return fmt.Errorf("legacy mapping keys must be unique strings")
			}
			seen[k.Value] = true
		}
	}
	for _, c := range n.Content {
		if err := checkNode(c); err != nil {
			return err
		}
	}
	return nil
}
func destination(name string, notifiers map[string]legacyNotifier) (plan.CompiledDestination, *Binding, error) {
	n, ok := notifiers[name]
	if !ok {
		if name == "stdout" {
			n.Type = "console"
		} else {
			return plan.CompiledDestination{}, nil, fmt.Errorf("Notifier %q requires a supported configured destination; CI outputs are unsupported.", name)
		}
	}
	switch n.Type {
	case "console", "webhook", "slack", "discord":
	default:
		return plan.CompiledDestination{}, nil, fmt.Errorf("Notifier %q uses unsupported type %q (including CI/Kubernetes outputs).", name, n.Type)
	}
	d := watch.Destination{APIVersion: watch.APIVersion, Kind: "Destination", Metadata: watch.Metadata{ID: identifier("destination-" + name)}, Spec: watch.DestinationSpec{Type: n.Type, MaxAttempts: n.MaxAttempts, InitialBackoff: n.InitialBackoff}}
	if d.Spec.MaxAttempts == 0 {
		d.Spec.MaxAttempts = 3
	}
	var binding *Binding
	if n.Type != "console" {
		if n.URL == "" {
			return plan.CompiledDestination{}, nil, fmt.Errorf("Notifier %q has no URL.", name)
		}
		env := "DING_MIGRATED_" + strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256([]byte(name)))) + "_URL"
		if m := envRef.FindStringSubmatch(n.URL); m != nil {
			env = m[1]
		}
		d.Spec.URLRef = &watch.SecretRef{Env: env}
		binding = &Binding{Notifier: name, Environment: env, From: "notifiers." + name + ".url (set the entire URL; literal values are not copied)"}
	}
	compiled, err := plan.CompileDestination(d)
	if err != nil {
		return compiled, nil, fmt.Errorf("Notifier %q has unsupported retry settings.", name)
	}
	return compiled, binding, nil
}
func projection(prefix string, r legacyRule) (string, string, map[string]any, error) {
	// Canonical all-string-label identity preserves legacy grouping, including
	// labels not mentioned in a selector. Numeric extra fields do not group.
	jq := `if type != "object" or (.metric|type) != "string" or .metric == "" or (.value|type) != "number" then error("invalid legacy metric") else . end | . as $e | (to_entries | map(select(.key != "metric" and .key != "value" and .key != "timestamp" and (.value|type)=="string")) | sort_by(.key) | from_entries) as $labels | {value:$e.value,metric:$e.metric,legacy_group:($labels|tojson),rule_name:` + strconv.Quote(r.Name)
	match := map[string]any{}
	keys := make([]string, 0, len(r.Match))
	for key := range r.Match {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for i, key := range keys {
		alias := fmt.Sprintf("match_%d", i)
		match[alias] = r.Match[key]
		value := "$labels[" + strconv.Quote(key) + "] // \"\""
		if key == "metric" {
			value = "$e.metric"
		}
		jq += "," + alias + ":(" + value + ")"
	}
	message := r.Message
	if message == "" {
		message = `rule {{ printf "%q" .Fields.rule_name }} fired (metric={{ .Fields.metric }} value={{ .Fields.value }})`
	} else {
		rest := simpleAction.ReplaceAllString(message, "")
		if strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
			return "", "", nil, fmt.Errorf("Only literal text and simple {{ .field }} placeholders are converted; rewrite functions, formatting and control flow explicitly.")
		}
		unsupported := ""
		index := 0
		message = simpleAction.ReplaceAllStringFunc(message, func(action string) string {
			field := simpleAction.FindStringSubmatch(action)[1]
			alias := fmt.Sprintf("message_%d", index)
			index++
			value := "$e[" + strconv.Quote(field) + "]"
			switch field {
			case "avg", "max", "min", "sum", "count", "fired_at":
				unsupported = field
			case "rule":
				value = "($labels.rule // " + strconv.Quote(r.Name) + ")"
			case "timestamp":
				value = "null"
			case "metric", "value":
			default:
				value = "(if (" + value + "|type)==\"string\" or (" + value + "|type)==\"number\" then " + value + " else null end)"
			}
			jq += "," + alias + ":(" + value + ")"
			return "{{ .Fields." + alias + " }}"
		})
		if unsupported != "" {
			return "", "", nil, fmt.Errorf("Aggregate and fired_at template fields require explicit message redesign.")
		}
	}
	jq += "}"
	if prefix != "" {
		jq = "(" + prefix + ") | (if type == \"array\" then .[] else . end) | (" + jq + ")"
	}
	return jq, message, match, nil
}
