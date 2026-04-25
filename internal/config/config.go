package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

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
	Port          int      `yaml:"port"`
	Format        string   `yaml:"format"`
	MaxBufferSize int      `yaml:"max_buffer_size"`
	ReadTimeout   Duration `yaml:"read_timeout"`
	WriteTimeout  Duration `yaml:"write_timeout"`
	IdleTimeout   Duration `yaml:"idle_timeout"`
	MaxBodyBytes  int64    `yaml:"max_body_bytes"`
	JQ            string   `yaml:"jq"`
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
	MaxAttempts    int      `yaml:"max_attempts"`
	InitialBackoff Duration `yaml:"initial_backoff"`
}

type AlertTarget struct {
	Notifier string `yaml:"notifier"`
}

// GuardConfig defines a pre-fire HTTP guard check for a rule.
// Before firing an alert the engine makes a GET request to URL and only
// fires if the response status matches ExpectStatus. Results are cached
// for TTL (default 5s) to avoid hammering the guard endpoint.
type GuardConfig struct {
	URL          string   `yaml:"url"`
	ExpectStatus int      `yaml:"expect_status"`
	TTLRaw       Duration `yaml:"ttl"`
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
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
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
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Format == "" {
		cfg.Server.Format = "auto"
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

	if cfg.Persistence.StateFile != "" && cfg.Persistence.FlushInterval.Duration == 0 {
		cfg.Persistence.FlushInterval.Duration = 30 * time.Second
	}

	validFormats := map[string]bool{"json": true, "prometheus": true, "auto": true}
	if !validFormats[cfg.Server.Format] {
		return fmt.Errorf("invalid server.format %q: must be json, prometheus, or auto", cfg.Server.Format)
	}

	for i, rule := range cfg.Rules {
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
			if g.URL == "" {
				return fmt.Errorf("rule %q: guard.url is required", rule.Name)
			}
			if g.ExpectStatus == 0 {
				return fmt.Errorf("rule %q: guard.expect_status is required", rule.Name)
			}
			if g.TTL == 0 {
				cfg.Rules[i].Guard.TTL = 5 * time.Second
			}
		}
	}

	for name, nc := range cfg.Notifiers {
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
		case "github_actions":
			// no required fields; auto-detects GITHUB_STEP_SUMMARY at runtime
		case "":
			return fmt.Errorf("notifier %q: type is required", name)
		default:
			return fmt.Errorf("notifier %q: unknown type %q", name, nc.Type)
		}
	}

	return nil
}
