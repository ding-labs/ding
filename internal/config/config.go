package config

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var envVarRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnvVars replaces every ${VAR} reference in raw with the value of
// the corresponding environment variable, looked up via os.LookupEnv. If any
// referenced variable is not set in the process environment, returns an error
// naming all unset vars (sorted, deduplicated). An env var set to "" is
// considered set and substitutes to "".
func expandEnvVars(raw []byte) ([]byte, error) {
	var unset []string
	seen := map[string]struct{}{}
	out := envVarRef.ReplaceAllFunc(raw, func(match []byte) []byte {
		name := string(envVarRef.FindSubmatch(match)[1])
		if val, ok := os.LookupEnv(name); ok {
			return []byte(val)
		}
		if _, dup := seen[name]; !dup {
			seen[name] = struct{}{}
			unset = append(unset, name)
		}
		return match
	})
	if len(unset) > 0 {
		sort.Strings(unset)
		return nil, fmt.Errorf("unset env vars referenced in config: %s", strings.Join(unset, ", "))
	}
	return out, nil
}

// Duration wraps time.Duration for YAML unmarshaling of strings like "5m".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.Duration = dur
	return nil
}

type ServerConfig struct {
	Listen        string   `yaml:"listen"`
	AdminToken    string   `yaml:"admin_token"`
	IngestToken   string   `yaml:"ingest_token"`
	TokenFile     string   `yaml:"token_file"`
	Port          int      `yaml:"port"`
	Format        string   `yaml:"format"`
	MaxBufferSize int      `yaml:"max_buffer_size"`
	MaxLabelSets  int      `yaml:"max_label_sets"`
	StateIdleTTL  Duration `yaml:"state_idle_ttl"`
	ReadTimeout   Duration `yaml:"read_timeout"`
	WriteTimeout  Duration `yaml:"write_timeout"`
	IdleTimeout   Duration `yaml:"idle_timeout"`
	MaxBodyBytes  int64    `yaml:"max_body_bytes"`
	JQ            string   `yaml:"jq"`
	DrainTimeout  Duration `yaml:"drain_timeout"`
}

type PersistenceConfig struct {
	StateFile     string   `yaml:"state_file"`
	FlushInterval Duration `yaml:"flush_interval"`
}

type AlertLogConfig struct {
	Path string `yaml:"path"`
}

type NotifierConfig struct {
	Type           string   `yaml:"type"`
	URL            string   `yaml:"url"`
	Token          string   `yaml:"token,omitempty"`
	ChatID         string   `yaml:"chat_id,omitempty"`
	MaxAttempts    int      `yaml:"max_attempts"`
	InitialBackoff Duration `yaml:"initial_backoff"`
	// Fields below are specific to type: kubernetes_event. All optional.
	Namespace   string `yaml:"namespace,omitempty"`    // override POD_NAMESPACE downward API
	EventReason string `yaml:"event_reason,omitempty"` // K8s Event reason (default "DingAlertFired")
	EventType   string `yaml:"event_type,omitempty"`   // K8s Event type, "Normal" or "Warning" (default "Warning")
	// Fields below are specific to type: gitlab_artifact. All optional.
	Path string `yaml:"path,omitempty"` // artifact file path (default "ding-alerts.md")
	// Fields below are specific to type: buildkite_annotate. All optional.
	Style string `yaml:"style,omitempty"` // annotation style: success | info | warning | error (default "error")
}

type AlertTarget struct {
	Notifier string `yaml:"notifier"`
}

// GuardConfig defines a pre-fire HTTP guard check for a rule.
// Before firing an alert the engine makes a GET request to URL and only
// fires if the response status matches ExpectStatus. Results are cached
// for TTL (default 5s) to avoid hammering the guard endpoint.
type GuardConfig struct {
	URL          string        `yaml:"url"`
	ExpectStatus int           `yaml:"expect_status"`
	TTLRaw       Duration      `yaml:"ttl"`
	TTL          time.Duration `yaml:"-"`
}

type Rule struct {
	Name        string            `yaml:"name"`
	Match       map[string]string `yaml:"match"`
	Condition   string            `yaml:"condition"`
	Cooldown    time.Duration     `yaml:"-"`
	CooldownRaw Duration          `yaml:"cooldown"`
	Message     string            `yaml:"message"`
	Alert       []AlertTarget     `yaml:"alert"`
	Guard       *GuardConfig      `yaml:"guard"`
	// Mode controls when the rule fires. Empty or "during-run" fires alerts
	// in real-time as events flow through. "end-of-run" populates windowed
	// aggregates during the run but only fires on run exit (ding run mode).
	// In ding serve mode, end-of-run rules are inert.
	Mode string `yaml:"mode"`
}

type Config struct {
	Server      ServerConfig              `yaml:"server"`
	Notifiers   map[string]NotifierConfig `yaml:"notifiers"`
	Rules       []Rule                    `yaml:"rules"`
	Persistence PersistenceConfig         `yaml:"persistence"`
	AlertLog    AlertLogConfig            `yaml:"alert_log"`
}

// Load reads and parses a ding.yaml file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	return Parse(data)
}

// Parse decodes YAML before expanding scalar values, so secret contents cannot
// inject configuration structure. Unknown fields and extra documents fail.
func Parse(data []byte) (*Config, error) {
	var node yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&node); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("config must contain exactly one YAML document")
	}
	if err := expandNode(&node); err != nil {
		return nil, err
	}
	encoded, err := yaml.Marshal(&node)
	if err != nil {
		return nil, err
	}
	var cfg Config
	strict := yaml.NewDecoder(bytes.NewReader(encoded))
	strict.KnownFields(true)
	if err := strict.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	// Copy parsed duration values
	for i := range cfg.Rules {
		cfg.Rules[i].Cooldown = cfg.Rules[i].CooldownRaw.Duration
		if g := cfg.Rules[i].Guard; g != nil {
			g.TTL = g.TTLRaw.Duration
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// isBuiltinNotifier reports whether name refers to a notifier that is always
// available without an explicit `notifiers:` block. These are registered by
// the server at engine-build time.
func isBuiltinNotifier(name string) bool {
	switch name {
	case "stdout", "github_actions":
		return true
	}
	return false
}

// Validate sets defaults and checks for semantic errors.
func (cfg *Config) Validate() error {
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = "127.0.0.1"
	}
	ip := net.ParseIP(cfg.Server.Listen)
	if ip == nil {
		return fmt.Errorf("server.listen must be an explicit IP address")
	}
	if !ip.IsLoopback() && (len(cfg.Server.AdminToken) < 16 || len(cfg.Server.IngestToken) < 16) {
		return fmt.Errorf("remote binding requires separate admin_token and ingest_token (at least 16 characters)")
	}
	if cfg.Server.AdminToken != "" && cfg.Server.AdminToken == cfg.Server.IngestToken {
		return fmt.Errorf("admin and ingest tokens must differ")
	}
	if cfg.Server.Port < 0 || cfg.Server.Port > 65535 || cfg.Server.MaxBufferSize < 0 || cfg.Server.MaxBodyBytes < 0 || cfg.Server.ReadTimeout.Duration < 0 || cfg.Server.WriteTimeout.Duration < 0 || cfg.Server.IdleTimeout.Duration < 0 || cfg.Server.DrainTimeout.Duration < 0 || cfg.Persistence.FlushInterval.Duration < 0 {
		return fmt.Errorf("server bounds and durations must be positive; port must be 1..65535")
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Format == "" {
		cfg.Server.Format = "auto"
	}
	if cfg.Server.MaxLabelSets == 0 {
		cfg.Server.MaxLabelSets = 10000
	}
	if cfg.Server.StateIdleTTL.Duration == 0 {
		cfg.Server.StateIdleTTL.Duration = time.Hour
	}
	if cfg.Server.MaxLabelSets < 0 || cfg.Server.StateIdleTTL.Duration < 0 {
		return fmt.Errorf("state limits must be positive")
	}
	if cfg.Server.MaxBufferSize == 0 {
		cfg.Server.MaxBufferSize = 10000
	}
	if cfg.Server.ReadTimeout.Duration == 0 {
		cfg.Server.ReadTimeout.Duration = 5 * time.Second
	}
	if cfg.Server.WriteTimeout.Duration == 0 {
		cfg.Server.WriteTimeout.Duration = 10 * time.Second
	}
	if cfg.Server.IdleTimeout.Duration == 0 {
		cfg.Server.IdleTimeout.Duration = 60 * time.Second
	}
	if cfg.Server.MaxBodyBytes == 0 {
		cfg.Server.MaxBodyBytes = 1 << 20
	}
	if cfg.Server.DrainTimeout.Duration == 0 {
		cfg.Server.DrainTimeout.Duration = 5 * time.Second
	}

	if cfg.Persistence.StateFile != "" && cfg.Persistence.FlushInterval.Duration == 0 {
		cfg.Persistence.FlushInterval.Duration = 30 * time.Second
	}

	validFormats := map[string]bool{"json": true, "prometheus": true, "auto": true}
	if !validFormats[cfg.Server.Format] {
		return fmt.Errorf("invalid server.format %q: must be json, prometheus, or auto", cfg.Server.Format)
	}

	names := map[string]bool{}
	for i, rule := range cfg.Rules {
		if names[rule.Name] {
			return fmt.Errorf("duplicate rule name %q", rule.Name)
		}
		names[rule.Name] = true
		if rule.Cooldown < 0 {
			return fmt.Errorf("rule %q: cooldown must not be negative", rule.Name)
		}
		if rule.Name == "" {
			return fmt.Errorf("rule[%d]: name is required", i)
		}
		if rule.Condition == "" {
			return fmt.Errorf("rule %q: condition is required", rule.Name)
		}
		switch rule.Mode {
		case "", "during-run", "end-of-run":
			// valid
		default:
			return fmt.Errorf("rule %q: invalid mode %q (must be empty, \"during-run\", or \"end-of-run\")", rule.Name, rule.Mode)
		}
		for _, target := range rule.Alert {
			if isBuiltinNotifier(target.Notifier) {
				continue
			}
			if _, ok := cfg.Notifiers[target.Notifier]; !ok {
				return fmt.Errorf("rule %q: alert references unknown notifier %q", rule.Name, target.Notifier)
			}
		}
		if g := rule.Guard; g != nil {
			if !validHTTPURL(g.URL) {
				return fmt.Errorf("rule %q: guard.url is required", rule.Name)
			}
			if g.ExpectStatus < 100 || g.ExpectStatus > 599 || g.TTL < 0 {
				return fmt.Errorf("rule %q: guard.expect_status is required", rule.Name)
			}
			if g.TTL == 0 {
				cfg.Rules[i].Guard.TTL = 5 * time.Second
			}
		}
	}

	for name, nc := range cfg.Notifiers {
		if nc.MaxAttempts < 0 || nc.MaxAttempts > 100 || nc.InitialBackoff.Duration < 0 {
			return fmt.Errorf("notifier %q: invalid retry bounds", name)
		}
		if nc.URL != "" && !validHTTPURL(nc.URL) {
			return fmt.Errorf("notifier %q: invalid HTTP URL", name)
		}

		switch nc.Type {
		case "webhook":
			if nc.URL == "" {
				return fmt.Errorf("notifier %q: webhook type requires a url", name)
			}
			if nc.MaxAttempts == 0 {
				nc.MaxAttempts = 3
			}
			if nc.InitialBackoff.Duration == 0 {
				nc.InitialBackoff.Duration = 1 * time.Second
			}
			cfg.Notifiers[name] = nc
		case "slack":
			if nc.URL == "" {
				return fmt.Errorf("notifier %q: slack type requires a url", name)
			}
			if nc.MaxAttempts == 0 {
				nc.MaxAttempts = 3
			}
			if nc.InitialBackoff.Duration == 0 {
				nc.InitialBackoff.Duration = 1 * time.Second
			}
			cfg.Notifiers[name] = nc
		case "discord":
			if nc.URL == "" {
				return fmt.Errorf("notifier %q: discord type requires a url", name)
			}
			if nc.MaxAttempts == 0 {
				nc.MaxAttempts = 3
			}
			if nc.InitialBackoff.Duration == 0 {
				nc.InitialBackoff.Duration = 1 * time.Second
			}
			cfg.Notifiers[name] = nc
		case "telegram":
			if nc.Token == "" {
				return fmt.Errorf("notifier %q: telegram type requires a token", name)
			}
			if nc.ChatID == "" {
				return fmt.Errorf("notifier %q: telegram type requires a chat_id", name)
			}
			if nc.MaxAttempts == 0 {
				nc.MaxAttempts = 3
			}
			if nc.InitialBackoff.Duration == 0 {
				nc.InitialBackoff.Duration = 1 * time.Second
			}
			cfg.Notifiers[name] = nc
		case "pagerduty":
			if nc.Token == "" {
				return fmt.Errorf("notifier %q: pagerduty type requires a token (routing key)", name)
			}
			if nc.MaxAttempts == 0 {
				nc.MaxAttempts = 3
			}
			if nc.InitialBackoff.Duration == 0 {
				nc.InitialBackoff.Duration = 1 * time.Second
			}
			cfg.Notifiers[name] = nc
		case "teams":
			if nc.URL == "" {
				return fmt.Errorf("notifier %q: teams type requires a url", name)
			}
			if nc.MaxAttempts == 0 {
				nc.MaxAttempts = 3
			}
			if nc.InitialBackoff.Duration == 0 {
				nc.InitialBackoff.Duration = 1 * time.Second
			}
			cfg.Notifiers[name] = nc
		case "github_actions":
			// no required fields; auto-detects GITHUB_STEP_SUMMARY at runtime
		case "kubernetes_event":
			if nc.EventType != "" && nc.EventType != "Normal" && nc.EventType != "Warning" {
				return fmt.Errorf("notifier %q: kubernetes_event type requires event_type to be \"Normal\" or \"Warning\"", name)
			}
			if nc.MaxAttempts == 0 {
				nc.MaxAttempts = 3
			}
			if nc.InitialBackoff.Duration == 0 {
				nc.InitialBackoff.Duration = 1 * time.Second
			}
			cfg.Notifiers[name] = nc
		case "gitlab_artifact":
			// No required fields; path defaults to "ding-alerts.md" if empty.
		case "buildkite_annotate":
			if nc.Style != "" {
				switch nc.Style {
				case "success", "info", "warning", "error":
				default:
					return fmt.Errorf("notifier %q: buildkite_annotate type requires style to be one of \"success\", \"info\", \"warning\", \"error\"", name)
				}
			}
		case "":
			return fmt.Errorf("notifier %q: type is required", name)
		default:
			return fmt.Errorf("notifier %q: unknown type %q", name, nc.Type)
		}
	}

	return nil
}

func expandNode(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("YAML aliases are not supported")
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		expanded, err := expandEnvVars([]byte(node.Value))
		if err != nil {
			return err
		}
		node.Value = string(expanded)
	}
	for i, child := range node.Content {
		if node.Kind == yaml.MappingNode && i%2 == 0 {
			if envVarRef.MatchString(child.Value) {
				return fmt.Errorf("environment references in keys are not supported")
			}
			continue
		}
		if err := expandNode(child); err != nil {
			return err
		}
	}
	return nil
}
func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
