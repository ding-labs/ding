package server

import (
	"fmt"
	"log"
	"time"

	"github.com/ding-labs/ding/internal/config"
	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/notifier"
	"github.com/itchyny/gojq"
)

// StartPersistence must run before accepting input. Failed restore leaves the
// original snapshot untouched; the caller must close the constructed server.
func (s *Server) StartPersistence() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("server closed")
	}
	if s.stopFlusher != nil {
		return nil
	}
	if err := RestoreStateFile(s.engine, s.cfg.Persistence.StateFile, time.Now()); err != nil {
		return err
	}
	s.startFlusher()
	return nil
}
func RestoreStateFile(eng *evaluator.Engine, path string, now time.Time) error {
	if path == "" {
		return nil
	}
	snap, err := evaluator.LoadSnapshot(path)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if snap == nil {
		return nil
	}
	report, err := evaluator.RestoreEngine(eng, *snap, now)
	if err != nil {
		return fmt.Errorf("restoring state: %w", err)
	}
	if len(report.ResetRules) > 0 {
		log.Printf("ding: reset incompatible/retired state: %v", report.ResetRules)
	}
	return nil
}
func (s *Server) startFlusher() {
	s.stopFlusher = func() {}
	if s.cfg.Persistence.StateFile != "" {
		interval := s.cfg.Persistence.FlushInterval.Duration
		if interval <= 0 {
			interval = time.Minute
		}
		s.stopFlusher = s.engine.StartFlusher(s.cfg.Persistence.StateFile, interval)
	}
}

func (s *Server) Reload() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return fmt.Errorf("server closed")
	}
	eng, cfg, ns, logger, jq, err := buildFromConfig(s.configPath, s.collector)
	if err != nil {
		return err
	}
	return s.swap(eng, cfg, ns, logger, jq)
}

// SwapEngine waits for evaluation and dispatch before copying state and retiring
// resources. The live in-memory snapshot has no disk-flush-to-swap race.
func (s *Server) SwapEngine(eng *evaluator.Engine, cfg *config.Config, ns map[string]notifier.Notifier, logger *notifier.AlertLogger, jq *gojq.Code) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.swap(eng, cfg, ns, logger, jq)
}
func (s *Server) swap(eng *evaluator.Engine, cfg *config.Config, ns map[string]notifier.Notifier, logger *notifier.AlertLogger, jq *gojq.Code) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		closeResources(ns, logger, 0)
		return fmt.Errorf("server closed")
	}
	if cfg.Server.Listen != s.cfg.Server.Listen || cfg.Server.Port != s.cfg.Server.Port || cfg.Server.AdminToken != s.cfg.Server.AdminToken || cfg.Server.IngestToken != s.cfg.Server.IngestToken || cfg.Server.TokenFile != s.cfg.Server.TokenFile {
		s.mu.Unlock()
		closeResources(ns, logger, 0)
		return fmt.Errorf("listen/authentication changes require a restart")
	}
	report, err := evaluator.RestoreEngine(eng, evaluator.SnapshotEngine(s.engine), time.Now())
	if err != nil {
		s.mu.Unlock()
		closeResources(ns, logger, 0)
		return fmt.Errorf("transferring live state: %w", err)
	}
	if len(report.ResetRules) > 0 {
		log.Printf("ding: reset incompatible/retired state: %v", report.ResetRules)
	}
	oldNS, oldLogger := s.notifiers, s.alertLogger
	timeout := s.cfg.Server.DrainTimeout.Duration
	if s.stopFlusher != nil {
		s.stopFlusher()
	}
	s.engine, s.cfg, s.notifiers, s.alertLogger, s.jqCode = eng, cfg, ns, logger, jq
	s.startFlusher()
	s.mu.Unlock()
	closeResources(oldNS, oldLogger, timeout)
	return nil
}

// Close stops admissions, waits for active producers, persists the final state,
// then drains delivery. It is serialized with both HTTP and signal reloads.
func (s *Server) Close(timeout time.Duration) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if s.stopFlusher != nil {
		s.stopFlusher()
	}
	ns, logger := s.notifiers, s.alertLogger
	s.mu.Unlock()
	closeResources(ns, logger, timeout)
}
func closeResources(ns map[string]notifier.Notifier, logger *notifier.AlertLogger, timeout time.Duration) {
	notifier.DrainAll(ns, timeout)
	if logger != nil {
		if err := logger.Close(); err != nil {
			log.Printf("ding: closing alert log: %v", err)
		}
	}
}
