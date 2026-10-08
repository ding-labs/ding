package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/ding-labs/ding/internal/evaluator"
	"github.com/ding-labs/ding/internal/ingester"
	"github.com/ding-labs/ding/internal/notifier"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Acquire cfg FIRST so MaxBodyBytes is available for MaxBytesReader.
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		jsonError(w, "server closed", http.StatusServiceUnavailable)
		return
	}
	cfg := s.cfg
	eng := s.engine
	notifiers := s.notifiers
	alertLogger := s.alertLogger
	jqCode := s.jqCode

	r.Body = http.MaxBytesReader(w, r.Body, cfg.Server.MaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			jsonError(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		jsonError(w, "reading body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var events []ingester.Event
	var parseErr error
	if jqCode != nil {
		events, parseErr = ingester.RunJQ(jqCode, body)
	} else {
		format := ingester.DetectFormat(body, r.Header.Get("Content-Type"), cfg.Server.Format)
		if format == "json" {
			events, parseErr = ingester.ParseJSONLine(body)
		} else {
			events, parseErr = ingester.ParsePrometheusText(body)
		}
	}
	if parseErr != nil {
		jsonError(w, parseErr.Error(), http.StatusBadRequest)
		return
	}

	totalAlerts, accepted, evalErr := s.processEvents(events, notifiers, eng, alertLogger)
	if evalErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": evalErr.Error(), "events": accepted, "alerts_fired": totalAlerts})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"events":%d,"alerts_fired":%d}`, len(events), totalAlerts)
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	eng := s.engine
	s.mu.RUnlock()

	statuses := eng.RulesStatus(time.Now())
	type ruleResp struct {
		Name        string            `json:"name"`
		Condition   string            `json:"condition"`
		Cooldown    string            `json:"cooldown"`
		CoolingDown map[string]string `json:"cooling_down"`
	}
	resp := make([]ruleResp, len(statuses))
	for i, st := range statuses {
		resp[i] = ruleResp{
			Name:        st.Name,
			Condition:   st.Condition,
			Cooldown:    st.Cooldown,
			CoolingDown: st.CoolingDown,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("ding: encoding rules response: %v", err)
	}
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.configPath == "" {
		jsonError(w, "no config path set", http.StatusInternalServerError)
		return
	}

	if err := s.Reload(); err != nil {
		jsonError(w, "reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("ding: config reloaded from %s", s.configPath)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"reloaded"}`))
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.collector == nil {
		http.Error(w, "metrics not available", http.StatusServiceUnavailable)
		return
	}

	s.mu.RLock()
	notifiers := s.notifiers
	eng := s.engine
	s.mu.RUnlock()

	queueDepth := 0
	for _, n := range notifiers {
		if wn, ok := n.(interface{ QueueDepth() int }); ok {
			queueDepth += wn.QueueDepth()
		}
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.collector.WritePrometheus(w, queueDepth)
	stats := eng.StateStats()
	fmt.Fprintf(w, "ding_state_label_sets %d\nding_state_buffers %d\nding_state_rejections_total %d\n", stats.LabelSets, stats.Buffers, stats.Rejected)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	data, _ := json.Marshal(map[string]string{"error": msg})
	w.Write(data)
}

func (s *Server) processEvents(events []ingester.Event, notifiers map[string]notifier.Notifier, eng *evaluator.Engine, alertLogger *notifier.AlertLogger) (int, int, error) {
	now := time.Now()
	totalAlerts := 0
	if s.collector != nil {
		s.collector.IncrEvents(int64(len(events)))
	}
	for i, event := range events {
		alerts, err := eng.ProcessChecked(event, now)
		if err != nil {
			return totalAlerts, i, err
		}
		totalAlerts += len(alerts)
		for _, alert := range alerts {
			if alertLogger != nil {
				if err := alertLogger.Log(alert); err != nil {
					log.Printf("ding: alert log write error: %v", err)
				}
			}
			if s.collector != nil {
				s.collector.IncrAlerts(alert.Rule)
			}
			for _, name := range alert.Notifiers {
				n, ok := notifiers[name]
				if !ok {
					log.Printf("ding: unknown notifier %q for rule %q", name, alert.Rule)
					continue
				}
				if err := n.Send(alert); err != nil {
					log.Printf("ding: notifier %q error: %v", name, err)
				}
			}
		}
	}
	return totalAlerts, len(events), nil
}
