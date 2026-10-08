package server

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/config"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/ingester"
	"github.com/ding-labs/ding/internal/metrics"
	"github.com/ding-labs/ding/internal/notifier"
	"github.com/itchyny/gojq"
)

// Server holds the HTTP server state.
type Server struct {
	mu          sync.RWMutex
	lifecycleMu sync.Mutex
	closed      bool
	stopFlusher func()
	engine      *evaluator.Engine
	notifiers   map[string]notifier.Notifier
	cfg         *config.Config
	configPath  string
	mux         *http.ServeMux
	alertLogger *notifier.AlertLogger
	collector   *metrics.Collector // set once in New(), never swapped
	jqCode      *gojq.Code         // nil when no JQ configured
}

// New creates a Server. configPath is used by /reload.
// collector may be nil (e.g. in validate mode). alertLogger may be nil if not configured.
func New(eng *evaluator.Engine, notifiers map[string]notifier.Notifier, cfg *config.Config, configPath string, collector *metrics.Collector, alertLogger *notifier.AlertLogger, jqCode *gojq.Code) *Server {
	s := &Server{
		engine:      eng,
		notifiers:   notifiers,
		cfg:         cfg,
		configPath:  configPath,
		mux:         http.NewServeMux(),
		collector:   collector,
		alertLogger: alertLogger,
		jqCode:      jqCode,
	}
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ingest", s.handleIngest)
	s.mux.HandleFunc("/rules", s.handleRules)
	s.mux.HandleFunc("/reload", s.handleReload)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	return s
}

// Handler returns the HTTP handler for use in tests or net/http.
func (s *Server) Handler() http.Handler { return s.mux }

// BuildFromConfig loads a config file and builds an Engine + Notifiers + AlertLogger.
// Exported for use in main.go. Pass a nil collector to skip alert log construction
// (e.g. in validate mode).
func BuildFromConfig(path string, collector *metrics.Collector) (*evaluator.Engine, *config.Config, map[string]notifier.Notifier, *notifier.AlertLogger, *gojq.Code, error) {
	return buildFromConfig(path, collector)
}

// buildFromConfig is the internal implementation.
func CompileConfig(path string) (*evaluator.Engine, *config.Config, *gojq.Code, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading config: %w", err)
	}

	rules := make([]evaluator.EngineRule, len(cfg.Rules))
	for i, r := range cfg.Rules {
		alerts := make([]string, len(r.Alert))
		for j, a := range r.Alert {
			alerts[j] = a.Notifier
		}
		rules[i] = evaluator.EngineRule{
			Name:           r.Name,
			InputSignature: cfg.Server.Format + "\x00" + cfg.Server.JQ,
			Match:          r.Match,
			Condition:      r.Condition,
			Cooldown:       r.Cooldown,
			Message:        r.Message,
			Alerts:         alerts,
			Guard:          r.Guard,
			Mode:           r.Mode,
		}
	}
	eng, err := evaluator.NewEngineWithLimits(rules, cfg.Server.MaxBufferSize, evaluator.StateLimits{MaxLabelSets: cfg.Server.MaxLabelSets, IdleTTL: cfg.Server.StateIdleTTL.Duration})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("building engine: %w", err)
	}

	var jqCode *gojq.Code
	if cfg.Server.JQ != "" {
		jqCode, err = ingester.CompileJQ(cfg.Server.JQ)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return eng, cfg, jqCode, nil
}
func buildFromConfig(path string, collector *metrics.Collector) (*evaluator.Engine, *config.Config, map[string]notifier.Notifier, *notifier.AlertLogger, *gojq.Code, error) {
	eng, cfg, jqCode, err := CompileConfig(path)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	notifiers := map[string]notifier.Notifier{
		"stdout":         notifier.NewStdoutNotifier(nil),
		"github_actions": notifier.NewGitHubActionsNotifier(nil),
	}
	for name, nc := range cfg.Notifiers {
		switch nc.Type {
		case "webhook":
			notifiers[name] = notifier.NewWebhookNotifier(nc.URL, nc.MaxAttempts, nc.InitialBackoff.Duration, collector)
		case "slack":
			notifiers[name] = notifier.NewSlackNotifier(nc.URL, nc.MaxAttempts, nc.InitialBackoff.Duration, collector)
		case "discord":
			notifiers[name] = notifier.NewDiscordNotifier(nc.URL, nc.MaxAttempts, nc.InitialBackoff.Duration, collector)
		case "telegram":
			notifiers[name] = notifier.NewTelegramNotifier(nc.Token, nc.ChatID, nc.MaxAttempts, nc.InitialBackoff.Duration, collector)
		case "pagerduty":
			notifiers[name] = notifier.NewPagerDutyNotifier(nc.Token, nc.MaxAttempts, nc.InitialBackoff.Duration, collector)
		case "teams":
			notifiers[name] = notifier.NewTeamsNotifier(nc.URL, nc.MaxAttempts, nc.InitialBackoff.Duration, collector)
		case "github_actions":
			notifiers[name] = notifier.NewGitHubActionsNotifier(nil)
		case "kubernetes_event":
			n, err := notifier.NewKubernetesEventNotifier(nc.Namespace, nc.EventReason, nc.EventType, nc.MaxAttempts, nc.InitialBackoff.Duration, collector)
			if err != nil {
				closeResources(notifiers, nil, 0)
				return nil, nil, nil, nil, nil, fmt.Errorf("notifier %q: %w", name, err)
			}
			notifiers[name] = n
		case "gitlab_artifact":
			notifiers[name] = notifier.NewGitLabArtifactNotifier(nc.Path)
		case "buildkite_annotate":
			notifiers[name] = notifier.NewBuildkiteAnnotateNotifier(nc.Style)
		}
	}

	// Only open alert log when running in serve mode (collector non-nil).
	// Skipped during validate to avoid creating the file as a side effect.
	var alertLogger *notifier.AlertLogger
	if collector != nil && cfg.AlertLog.Path != "" {
		al, err := notifier.NewAlertLogger(cfg.AlertLog.Path)
		if err != nil {
			for _, n := range notifiers {
				if stopper, ok := n.(interface{ Stop() }); ok {
					stopper.Stop()
				}
			}
			return nil, nil, nil, nil, nil, fmt.Errorf("opening alert log: %w", err)
		}
		alertLogger = al
	}

	return eng, cfg, notifiers, alertLogger, jqCode, nil
}

// IngestLine processes a single raw event line from stdin.
func (s *Server) IngestLine(line []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return
	}
	cfg := s.cfg
	eng := s.engine
	notifiers := s.notifiers
	alertLogger := s.alertLogger
	jqCode := s.jqCode

	var events []ingester.Event
	var err error
	if jqCode != nil {
		events, err = ingester.RunJQ(jqCode, line)
	} else {
		format := ingester.DetectFormat(line, "", cfg.Server.Format)
		if format == "json" {
			events, err = ingester.ParseJSONLine(line)
		} else {
			events, err = ingester.ParsePrometheusText(line)
		}
	}
	if err != nil {
		log.Printf("ding: stdin parse error: %v", err)
		return
	}

	if _, _, err := s.processEvents(events, notifiers, eng, alertLogger); err != nil {
		log.Printf("ding: stdin rejected: %v", err)
	}
}

// Sweep prunes idle state even when no producer sends new observations.
func (s *Server) Sweep(now time.Time) {
	s.mu.RLock()
	eng := s.engine
	s.mu.RUnlock()
	eng.Sweep(now)
}
