// Package plan compiles declarations without environment lookup, network access,
// process execution, file creation, or worker startup.
package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/ding-labs/ding/internal/condition"
	"github.com/ding-labs/ding/internal/transform"
	"github.com/ding-labs/ding/internal/watch"
	"gopkg.in/yaml.v3"
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var fieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)

type Compiled struct {
	Definition  watch.Definition `json:"definition"`
	Revision    string           `json:"revision"`
	Fingerprint string           `json:"fingerprint"`
	Permissions []string         `json:"permissions"`
}
type CompiledDestination struct {
	Definition watch.Destination `json:"definition"`
	Revision   string            `json:"revision"`
}
type Bundle struct {
	Watches      []Compiled            `json:"watches"`
	Destinations []CompiledDestination `json:"destinations"`
}

func Parse(data []byte) (Bundle, error) {
	bundle := Bundle{Watches: []Compiled{}, Destinations: []CompiledDestination{}}
	if len(data) > 1<<20 {
		return bundle, fmt.Errorf("manifest exceeds 1 MiB")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	seen := map[string]bool{}
	for count := 0; ; count++ {
		var node yaml.Node
		if err := dec.Decode(&node); err == io.EOF {
			break
		} else if err != nil {
			return bundle, fmt.Errorf("invalid manifest YAML: %w", err)
		}
		if count >= 100 {
			return bundle, fmt.Errorf("manifest exceeds 100 resources")
		}
		if err := checkNode(&node); err != nil {
			return bundle, err
		}
		var header struct {
			Kind string `yaml:"kind"`
		}
		if err := node.Decode(&header); err != nil {
			return bundle, err
		}
		encoded, err := yaml.Marshal(&node)
		if err != nil {
			return bundle, err
		}
		strict := yaml.NewDecoder(bytes.NewReader(encoded))
		strict.KnownFields(true)
		var id string
		switch header.Kind {
		case "Watch":
			var d watch.Definition
			if err := strict.Decode(&d); err != nil {
				return bundle, err
			}
			conditionNode := lookupNode(lookupNode(node.Content[0], "spec"), "condition")
			if strings.Contains("|eq|ne|gt|gte|lt|lte|", "|"+d.Spec.Condition.Operator+"|") && d.Spec.Condition.Operator != "" && lookupNode(conditionNode, "value") == nil {
				return bundle, fmt.Errorf("comparison requires an explicit value (null is allowed)")
			}
			compiled, err := Compile(d)
			if err != nil {
				return bundle, fmt.Errorf("watch %q: %w", d.Metadata.ID, err)
			}
			id = d.Metadata.ID
			bundle.Watches = append(bundle.Watches, compiled)
		case "Destination":
			var d watch.Destination
			if err := strict.Decode(&d); err != nil {
				return bundle, err
			}
			compiled, err := CompileDestination(d)
			if err != nil {
				return bundle, fmt.Errorf("destination %q: %w", d.Metadata.ID, err)
			}
			id = d.Metadata.ID
			bundle.Destinations = append(bundle.Destinations, compiled)
		default:
			if lookupNode(node.Content[0], "rules") != nil || lookupNode(node.Content[0], "notifiers") != nil {
				return bundle, fmt.Errorf("legacy configuration: use ding migrate --config FILE --out NEW_DIRECTORY, or install legacy v0.14.0")
			}
			return bundle, fmt.Errorf("kind must be Watch or Destination")
		}
		key := header.Kind + ":" + id
		if seen[key] {
			return bundle, fmt.Errorf("duplicate resource %s", key)
		}
		seen[key] = true
	}
	if len(bundle.Watches)+len(bundle.Destinations) == 0 {
		return bundle, fmt.Errorf("empty manifest")
	}
	return bundle, nil
}
func checkNode(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode {
		return fmt.Errorf("YAML aliases are unsupported")
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
				return fmt.Errorf("mapping keys must be strings")
			}
			if seen[k.Value] {
				return fmt.Errorf("duplicate field %q", k.Value)
			}
			seen[k.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := checkNode(child); err != nil {
			return err
		}
	}
	return nil
}
func header(version, kind string, m watch.Metadata, want string) error {
	if version != watch.APIVersion || kind != want {
		return fmt.Errorf("expected %s %s", watch.APIVersion, want)
	}
	if !identifier.MatchString(m.ID) {
		return fmt.Errorf("metadata.id must be a stable 1..128 character identifier")
	}
	if len(m.Name) > 256 {
		return fmt.Errorf("metadata.name exceeds 256 bytes")
	}
	return nil
}
func duration(value string, min, max time.Duration) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d < min || d > max {
		return 0, fmt.Errorf("duration %q must be between %s and %s", value, min, max)
	}
	return d, nil
}
func validRef(ref *watch.SecretRef) bool { return ref != nil && envName.MatchString(ref.Env) }
func refs(refs map[string]watch.SecretRef) error {
	for key, ref := range refs {
		if key == "" || strings.ContainsAny(key, "\r\n") || !validRef(&ref) {
			return fmt.Errorf("invalid secret reference")
		}
	}
	return nil
}
func scalar(v any) bool {
	switch v.(type) {
	case nil, string, bool, int, int64, float64, json.Number:
		return true
	}
	return false
}
func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == "" && !strings.Contains(raw, "${")
}

func Compile(d watch.Definition) (Compiled, error) {
	if err := header(d.APIVersion, d.Kind, d.Metadata, "Watch"); err != nil {
		return Compiled{}, err
	}
	// Copy mutable input before applying defaults or sorting.
	raw, err := json.Marshal(d)
	if err != nil {
		return Compiled{}, err
	}
	var normalized watch.Definition
	if err = json.Unmarshal(raw, &normalized); err != nil {
		return Compiled{}, err
	}
	d = normalized
	s := &d.Spec.Source
	switch s.Type {
	case "http":
		if (s.URL == "") == (s.URLRef == nil) {
			return Compiled{}, fmt.Errorf("HTTP source requires exactly one of url or urlRef")
		}
		if s.URL != "" && !validURL(s.URL) {
			return Compiled{}, fmt.Errorf("invalid HTTP URL (use secret references for credentials)")
		}
		if s.URLRef != nil && !validRef(s.URLRef) {
			return Compiled{}, fmt.Errorf("invalid URL secret reference")
		}
		if len(s.Argv) > 0 || s.Directory != "" || len(s.Env) > 0 {
			return Compiled{}, fmt.Errorf("command fields are invalid for HTTP source")
		}
	case "command":
		if len(s.Argv) == 0 || len(s.Argv) > 100 || s.Argv[0] == "" || s.Directory == "" || !filepath.IsAbs(s.Directory) {
			return Compiled{}, fmt.Errorf("command requires explicit argv and absolute directory")
		}
		for _, arg := range s.Argv {
			if strings.ContainsRune(arg, 0) || len(arg) > 8192 {
				return Compiled{}, fmt.Errorf("invalid command argument")
			}
		}
		if s.URL != "" || s.URLRef != nil || len(s.Headers) > 0 {
			return Compiled{}, fmt.Errorf("HTTP fields are invalid for command source")
		}
		if err := refs(s.Env); err != nil {
			return Compiled{}, err
		}
		for name := range s.Env {
			if !envName.MatchString(name) {
				return Compiled{}, fmt.Errorf("invalid environment name")
			}
		}
	case "push":
		if s.URL != "" || s.URLRef != nil || len(s.Argv) > 0 || s.Directory != "" || s.Every != "" || s.Timeout != "" || len(s.Env) > 0 || len(s.Headers) > 0 {
			return Compiled{}, fmt.Errorf("push does not accept polling or command fields")
		}
	default:
		return Compiled{}, fmt.Errorf("unsupported source type %q", s.Type)
	}
	if s.Type != "push" {
		if s.Every == "" {
			s.Every = "5s"
		}
		if s.Timeout == "" {
			s.Timeout = "2s"
		}
		if _, err := duration(s.Every, time.Second, 24*time.Hour); err != nil {
			return Compiled{}, err
		}
		if _, err := duration(s.Timeout, time.Millisecond, 5*time.Minute); err != nil {
			return Compiled{}, err
		}
	}
	if err := refs(s.Headers); err != nil {
		return Compiled{}, err
	}
	if s.ObservedAtField != "" && !fieldName.MatchString(s.ObservedAtField) {
		return Compiled{}, fmt.Errorf("observedAtField must name a selected field")
	}
	if len(s.Fields) > 100 {
		return Compiled{}, fmt.Errorf("too many selected fields")
	}
	for name, path := range s.Fields {
		if !fieldName.MatchString(name) || !fieldName.MatchString(path) {
			return Compiled{}, fmt.Errorf("selected fields require simple dotted paths")
		}
	}
	if s.JQ != "" {
		if len(s.JQ) > 8192 {
			return Compiled{}, fmt.Errorf("jq exceeds 8192 bytes")
		}
		if _, err := transform.CompileJQ(s.JQ); err != nil {
			return Compiled{}, err
		}
	}
	c := &d.Spec.Condition
	if c.DedupFor != "" && c.Operator != "new-event" {
		return Compiled{}, fmt.Errorf("dedupFor belongs to new-event conditions")
	}
	if c.Numeric != "" {
		if c.Operator != "" || c.Value != nil || c.MissingFor != "" || c.DedupFor != "" {
			return Compiled{}, fmt.Errorf("numeric conditions cannot mix typed operators")
		}
		if c.Field == "" {
			c.Field = "value"
		}
		if strings.Contains(c.Numeric, "run") {
			return Compiled{}, fmt.Errorf("run-lifetime aggregation is unsupported")
		}
		if len(c.Numeric) > 4096 {
			return Compiled{}, fmt.Errorf("numeric expression too large")
		}
		if err := validateNumeric(c.Numeric); err != nil {
			return Compiled{}, err
		}
	} else if c.MissingFor != "" {
		if c.Field != "" || c.Operator != "" || c.Value != nil || c.DedupFor != "" {
			return Compiled{}, fmt.Errorf("missing-data condition cannot mix field conditions")
		}
		if _, err := duration(c.MissingFor, time.Second, 30*24*time.Hour); err != nil {
			return Compiled{}, err
		}
	} else {
		if !fieldName.MatchString(c.Field) {
			return Compiled{}, fmt.Errorf("condition.field is required")
		}
		switch c.Operator {
		case "eq", "ne":
			if !scalar(c.Value) {
				return Compiled{}, fmt.Errorf("comparison value must be a scalar")
			}
		case "gt", "gte", "lt", "lte":
			if _, ok := c.Value.(float64); !ok {
				return Compiled{}, fmt.Errorf("ordered comparison requires a numeric value")
			}
		case "changed":
			if c.Value != nil || c.DedupFor != "" {
				return Compiled{}, fmt.Errorf("changed takes no value or dedup horizon")
			}
		case "new-event":
			if c.Value != nil {
				return Compiled{}, fmt.Errorf("new-event takes no comparison value")
			}
			if _, err := duration(c.DedupFor, time.Second, 7*24*time.Hour); err != nil {
				return Compiled{}, err
			}
		default:
			return Compiled{}, fmt.Errorf("unsupported condition operator %q", c.Operator)
		}
	}
	if c.Field != "" && !fieldName.MatchString(c.Field) {
		return Compiled{}, fmt.Errorf("invalid condition field")
	}
	p := &d.Spec.Policy
	if p.Trigger == "" {
		p.Trigger = "transition"
	}
	if p.Consecutive == 0 {
		p.Consecutive = 1
	}
	if p.RecoverAfter == 0 {
		p.RecoverAfter = 1
	}
	if p.OnUnknown == "" {
		p.OnUnknown = "hold-incident"
	}
	if p.Trigger != "transition" && p.Trigger != "level" {
		return Compiled{}, fmt.Errorf("trigger must be transition or level")
	}
	if p.Consecutive < 1 || p.Consecutive > 10000 || p.RecoverAfter < 1 || p.RecoverAfter > 10000 || p.OnUnknown != "hold-incident" {
		return Compiled{}, fmt.Errorf("invalid count or unknown policy")
	}
	if p.Trigger == "level" {
		if _, err := duration(p.Interval, 0, 30*24*time.Hour); err != nil {
			return Compiled{}, fmt.Errorf("level policy requires interval: %w", err)
		}
	} else if p.Interval != "" {
		return Compiled{}, fmt.Errorf("interval belongs to level policy")
	}
	if p.MaxGap == "" && s.Every != "" {
		every, _ := time.ParseDuration(s.Every)
		p.MaxGap = (every * 2).String()
	}
	if p.MaxGap != "" {
		if _, err := duration(p.MaxGap, time.Second, 30*24*time.Hour); err != nil {
			return Compiled{}, err
		}
	}
	if c.MissingFor != "" && (p.Trigger != "transition" || p.Consecutive != 1 || p.RecoverAfter != 1) {
		return Compiled{}, fmt.Errorf("missingFor requires transition policy with consecutive=1 and recoverAfter=1")
	}
	if c.Operator == "changed" || c.Operator == "new-event" {
		if p.Trigger != "level" || p.Consecutive != 1 {
			return Compiled{}, fmt.Errorf("change/event conditions require level policy with consecutive=1")
		}
	}
	if len(d.Spec.GroupBy) > 16 {
		return Compiled{}, fmt.Errorf("too many grouping fields")
	}
	sort.Strings(d.Spec.GroupBy)
	for i, field := range d.Spec.GroupBy {
		if !fieldName.MatchString(field) || (i > 0 && d.Spec.GroupBy[i-1] == field) {
			return Compiled{}, fmt.Errorf("invalid/duplicate grouping field")
		}
	}
	for k, v := range d.Spec.Match {
		if !fieldName.MatchString(k) || !scalar(v) {
			return Compiled{}, fmt.Errorf("match must contain typed scalar fields")
		}
	}
	if len(d.Spec.Message) > 8192 {
		return Compiled{}, fmt.Errorf("message too long")
	}
	if _, err := template.New("message").Option("missingkey=error").Parse(d.Spec.Message); err != nil {
		return Compiled{}, fmt.Errorf("invalid message template: %w", err)
	}
	l := &d.Spec.Limits
	if l.MaxEntities == 0 {
		l.MaxEntities = 1000
	}
	if l.MaxSamples == 0 {
		l.MaxSamples = 10000
	}
	if l.MaxBytes == 0 {
		l.MaxBytes = 1 << 20
	}
	if l.MaxOutputs == 0 {
		l.MaxOutputs = 100
	}
	if l.IdleTTL == "" {
		l.IdleTTL = "24h"
	}
	if l.MaxEntities < 1 || l.MaxEntities > 10000 || l.MaxSamples < 1 || l.MaxSamples > 100000 || l.MaxBytes < 1 || l.MaxBytes > 1<<20 || l.MaxOutputs < 1 || l.MaxOutputs > 100 {
		return Compiled{}, fmt.Errorf("watch resource limits exceed supported bounds")
	}
	if _, err := duration(l.IdleTTL, time.Second, 30*24*time.Hour); err != nil {
		return Compiled{}, err
	}
	if len(d.Spec.Destinations) > 32 {
		return Compiled{}, fmt.Errorf("too many destinations")
	}
	if d.Spec.Destinations == nil {
		d.Spec.Destinations = []watch.Target{}
	}
	seen := map[string]bool{}
	for i := range d.Spec.Destinations {
		target := &d.Spec.Destinations[i]
		if !identifier.MatchString(target.Ref) || seen[target.Ref] {
			return Compiled{}, fmt.Errorf("invalid/duplicate destination reference")
		}
		seen[target.Ref] = true
		if len(target.Events) == 0 {
			target.Events = []string{"firing", "recovered", "changed", "new-event", "source_error", "source_recovered"}
		}
		eventTypes := map[string]bool{}
		for _, event := range target.Events {
			if !ValidEventType(event) || eventTypes[event] {
				return Compiled{}, fmt.Errorf("unsupported/duplicate event type %q", event)
			}
			eventTypes[event] = true
		}
		sort.Strings(target.Events)
	}
	sort.Slice(d.Spec.Destinations, func(i, j int) bool { return d.Spec.Destinations[i].Ref < d.Spec.Destinations[j].Ref })
	normalizeDurations(&s.Every, &s.Timeout, &c.MissingFor, &c.DedupFor, &p.Interval, &p.MaxGap, &l.IdleTTL)
	rev, err := watch.Revision(d)
	if err != nil {
		return Compiled{}, err
	}
	fingerprint, err := watch.Revision(struct {
		Source    watch.Source
		Condition watch.Condition
		Policy    watch.Policy
		GroupBy   []string
		Match     map[string]any
		Limits    watch.Limits
	}{*s, *c, *p, d.Spec.GroupBy, d.Spec.Match, *l})
	if err != nil {
		return Compiled{}, err
	}
	return Compiled{d, rev, fingerprint, ExecutionPermissions(d.Spec.Source.Type)}, nil
}
func ValidEventType(s string) bool {
	switch s {
	case "firing", "recovered", "changed", "new-event", "source_error", "source_recovered", "gap", "state_reset", "paused", "resumed", "deleted", "expired":
		return true
	}
	return false
}
func CompileDestination(d watch.Destination) (CompiledDestination, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return CompiledDestination{}, err
	}
	var normalized watch.Destination
	if err = json.Unmarshal(raw, &normalized); err != nil {
		return CompiledDestination{}, err
	}
	d = normalized

	if err := header(d.APIVersion, d.Kind, d.Metadata, "Destination"); err != nil {
		return CompiledDestination{}, err
	}
	switch d.Spec.Type {
	case "console":
		if d.Spec.URLRef != nil || len(d.Spec.Headers) > 0 {
			return CompiledDestination{}, fmt.Errorf("console takes no HTTP settings")
		}
	case "webhook", "slack", "discord":
		if !validRef(d.Spec.URLRef) {
			return CompiledDestination{}, fmt.Errorf("HTTP destinations require urlRef.env")
		}
	default:
		return CompiledDestination{}, fmt.Errorf("unsupported destination type %q", d.Spec.Type)
	}
	if err := refs(d.Spec.Headers); err != nil {
		return CompiledDestination{}, err
	}
	if d.Spec.MaxAttempts == 0 {
		d.Spec.MaxAttempts = 8
	}
	if d.Spec.MaxAge == "" {
		d.Spec.MaxAge = "24h"
	}
	if d.Spec.InitialBackoff == "" {
		d.Spec.InitialBackoff = "1s"
	}
	if d.Spec.MaxAttempts < 1 || d.Spec.MaxAttempts > 100 {
		return CompiledDestination{}, fmt.Errorf("invalid retry attempts")
	}
	if _, err := duration(d.Spec.MaxAge, time.Second, 7*24*time.Hour); err != nil {
		return CompiledDestination{}, err
	}
	if _, err := duration(d.Spec.InitialBackoff, time.Millisecond, time.Hour); err != nil {
		return CompiledDestination{}, err
	}
	normalizeDurations(&d.Spec.MaxAge, &d.Spec.InitialBackoff)
	rev, err := watch.Revision(d)
	return CompiledDestination{d, rev}, err
}

func lookupNode(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
func normalizeDurations(values ...*string) {
	for _, v := range values {
		if *v != "" {
			d, _ := time.ParseDuration(*v)
			*v = d.String()
		}
	}
}
func validateNumeric(expr string) error {
	if _, err := condition.ParseExpression(expr); err != nil {
		return err
	}
	for _, atom := range regexp.MustCompile(` AND | OR `).Split(expr, -1) {
		c, err := condition.ParseCondition(strings.TrimSpace(atom))
		if err != nil {
			return err
		}
		if math.IsInf(c.Literal, 0) || math.IsNaN(c.Literal) {
			return fmt.Errorf("numeric literal must be finite")
		}
		if c.RunBounded || (c.Windowed && (c.Window <= 0 || c.Window > 30*24*time.Hour)) {
			return fmt.Errorf("numeric window must be positive and at most 30 days")
		}
	}
	return nil
}

func ExecutionPermissions(sourceType string) []string {
	switch sourceType {
	case "http":
		return []string{"http-read"}
	case "command":
		return []string{"local-command (trusted configuration; not sandboxed)"}
	case "push":
		return []string{"authenticated-ingest"}
	}
	return []string{}
}
