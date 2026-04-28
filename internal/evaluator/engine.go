package evaluator

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/zuchka/ding/internal/config"
	"github.com/zuchka/ding/internal/ingester"
)

// EngineRule is the fully-resolved rule fed to the Engine.
type EngineRule struct {
	Name      string
	Match     map[string]string
	Condition string
	Cooldown  time.Duration
	Message   string
	Alerts    []string         // notifier names ("stdout" or named webhook)
	Guard     *config.GuardConfig // optional HTTP guard; nil means no guard
	// Mode is "" / "during-run" (default) or "end-of-run". End-of-run rules
	// populate buffers during Process() but only fire when ProcessEndOfRun()
	// is invoked at run exit (ding run mode).
	Mode string
}

// Alert is a fired alert ready to dispatch.
type Alert struct {
	Rule      string
	Message   string
	Metric    string
	Value     float64
	Labels    map[string]string
	Floats    map[string]float64 // extra numeric fields from the event
	FiredAt   time.Time
	Notifiers []string
	// Aggregate values for windowed rules (zero if event-per-event)
	Avg   float64
	Max   float64
	Min   float64
	Count float64
	Sum   float64
}

// guardEntry caches the result of a guard HTTP check.
type guardEntry struct {
	allowed   bool
	expiresAt time.Time
}

// Engine evaluates events against rules and produces alerts.
// Thread-safe. Supports atomic hot-swap via Swap().
//
// Locking strategy:
//   - mu (RWMutex): protects rules/maxBuf during hot-reload. Process() holds RLock;
//     SwapEngine() holds Lock.
//   - bufMu (Mutex): protects buffers and seenLabelKeys independently of mu, so
//     buffer creation never races with concurrent Process() calls.
//   - guardMu (Mutex): protects guardCache independently of mu.
type Engine struct {
	mu            sync.RWMutex
	bufMu         sync.Mutex
	guardMu       sync.Mutex
	rules         []parsedRule
	buffers       map[string]*RingBuffer // keyed by "ruleName:labelSetKey"
	seenLabelKeys map[string][]string    // ruleName -> seen label-set keys (for /rules endpoint)
	cooldown      *CooldownTracker
	maxBuf        int
	guardCache    map[string]guardEntry
	guardHTTP     *http.Client
}

type parsedRule struct {
	EngineRule
	expr ConditionExpr
}

// IsEndOfRun reports whether this rule fires only on run exit.
func (r parsedRule) IsEndOfRun() bool { return r.Mode == "end-of-run" }

// NewEngine creates an Engine from a slice of EngineRules.
func NewEngine(rules []EngineRule, maxBufferSize int) (*Engine, error) {
	parsed := make([]parsedRule, len(rules))
	for i, r := range rules {
		expr, err := ParseConditionExpr(r.Condition)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", r.Name, err)
		}
		parsed[i] = parsedRule{EngineRule: r, expr: expr}
	}
	return &Engine{
		rules:         parsed,
		buffers:       make(map[string]*RingBuffer),
		seenLabelKeys: make(map[string][]string),
		cooldown:      NewCooldownTracker(),
		maxBuf:        maxBufferSize,
		guardCache:    make(map[string]guardEntry),
		guardHTTP:     &http.Client{Timeout: 3 * time.Second},
	}, nil
}

// Process evaluates an event against all rules. Returns fired alerts.
func (e *Engine) Process(event ingester.Event, now time.Time) []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var alerts []Alert
	for _, rule := range e.rules {
		mr := MatchRule{Match: rule.Match}
		if !Match(event, mr) {
			continue
		}

		labelKey := LabelSetKey(event.Labels)

		// Track seen label keys for /rules endpoint (before condition check)
		e.trackLabelKey(rule.Name, labelKey)

		// Unconditional pre-pass: populate all windowed ring buffers before evaluation.
		// Buffers must receive every event regardless of short-circuit outcome so that
		// aggregates remain accurate when conditions later become true.
		leaves := rule.expr.collectWindowedLeaves()
		ctx := evalContext{
			Value:      event.Value,
			Aggregates: make(map[int]float64, len(leaves)),
			Available:  make(map[int]bool, len(leaves)),
		}
		for _, leaf := range leaves {
			leafBufKey := rule.Name + ":" + strconv.Itoa(leaf.ID) + ":" + labelKey
			buf := e.getOrCreateBuffer(leafBufKey, leaf.Window)
			buf.Add(event.Value, event.At)
			if buf.HasEntries(now) {
				ctx.Available[leaf.ID] = true
				switch leaf.Func {
				case "avg":
					ctx.Aggregates[leaf.ID] = buf.Avg(now)
				case "max":
					ctx.Aggregates[leaf.ID] = buf.Max(now)
				case "min":
					ctx.Aggregates[leaf.ID] = buf.Min(now)
				case "sum":
					ctx.Aggregates[leaf.ID] = buf.Sum(now)
				case "count":
					ctx.Aggregates[leaf.ID] = buf.Count(now)
				}
			}
		}

		// End-of-run rules populate buffers but never fire during Process().
		// They only fire via ProcessEndOfRun() when the wrapped command exits.
		if rule.Mode == "end-of-run" {
			continue
		}

		if rule.Guard != nil && !e.checkGuard(rule.Name, rule.Guard, now) {
			continue
		}

		if !rule.expr.eval(ctx) {
			continue
		}

		if e.cooldown.IsActive(rule.Name, labelKey) {
			continue
		}
		if rule.Cooldown > 0 {
			e.cooldown.Set(rule.Name, labelKey, rule.Cooldown)
		}

		alert := Alert{
			Rule:      rule.Name,
			Metric:    event.Metric,
			Value:     event.Value,
			Labels:    event.Labels,
			Floats:    event.Floats,
			FiredAt:   now,
			Notifiers: rule.Alerts,
		}
		// Populate flat aggregate fields for backward compat.
		// Only for single-windowed-leaf rules; compound multi-leaf rules leave these zero.
		if len(leaves) == 1 {
			leaf := leaves[0]
			leafBufKey := rule.Name + ":" + strconv.Itoa(leaf.ID) + ":" + labelKey
			buf := e.getOrCreateBuffer(leafBufKey, leaf.Window)
			alert.Avg = buf.Avg(now)
			alert.Max = buf.Max(now)
			alert.Min = buf.Min(now)
			alert.Count = buf.Count(now)
			alert.Sum = buf.Sum(now)
		}
		alert.Message = renderMessage(rule.Message, alert)
		alerts = append(alerts, alert)
	}
	return alerts
}

// ProcessEndOfRun evaluates all rules with mode "end-of-run" using whatever
// state was accumulated during the run, and returns alerts to dispatch.
// Called once when a wrapped subprocess exits (ding run mode). Cooldowns
// are not consulted — end-of-run rules fire at most once per run anyway.
func (e *Engine) ProcessEndOfRun(now time.Time) []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var alerts []Alert
	for _, rule := range e.rules {
		if !rule.IsEndOfRun() {
			continue
		}

		// Snapshot label keys seen for this rule. If none were seen, evaluate
		// once with the empty key so rules that never observed a matching event
		// can still fail (e.g., "count(value) over 1h > 0" should fire when
		// no events were seen — though in practice, these rules need at least
		// one buffer entry to evaluate).
		e.bufMu.Lock()
		labelKeys := append([]string{}, e.seenLabelKeys[rule.Name]...)
		e.bufMu.Unlock()
		if len(labelKeys) == 0 {
			labelKeys = []string{""}
		}

		leaves := rule.expr.collectWindowedLeaves()

		for _, labelKey := range labelKeys {
			ctx := evalContext{
				Aggregates: make(map[int]float64, len(leaves)),
				Available:  make(map[int]bool, len(leaves)),
			}
			for _, leaf := range leaves {
				leafBufKey := rule.Name + ":" + strconv.Itoa(leaf.ID) + ":" + labelKey
				buf := e.lookupBuffer(leafBufKey)
				if buf == nil || !buf.HasEntries(now) {
					continue
				}
				ctx.Available[leaf.ID] = true
				switch leaf.Func {
				case "avg":
					ctx.Aggregates[leaf.ID] = buf.Avg(now)
				case "max":
					ctx.Aggregates[leaf.ID] = buf.Max(now)
				case "min":
					ctx.Aggregates[leaf.ID] = buf.Min(now)
				case "sum":
					ctx.Aggregates[leaf.ID] = buf.Sum(now)
				case "count":
					ctx.Aggregates[leaf.ID] = buf.Count(now)
				}
			}

			if rule.Guard != nil && !e.checkGuard(rule.Name, rule.Guard, now) {
				continue
			}
			if !rule.expr.eval(ctx) {
				continue
			}

			alert := Alert{
				Rule:      rule.Name,
				Metric:    "run.summary",
				Labels:    parseLabelKey(labelKey),
				FiredAt:   now,
				Notifiers: rule.Alerts,
			}
			if len(leaves) == 1 {
				leaf := leaves[0]
				leafBufKey := rule.Name + ":" + strconv.Itoa(leaf.ID) + ":" + labelKey
				if buf := e.lookupBuffer(leafBufKey); buf != nil {
					alert.Avg = buf.Avg(now)
					alert.Max = buf.Max(now)
					alert.Min = buf.Min(now)
					alert.Count = buf.Count(now)
					alert.Sum = buf.Sum(now)
				}
			}
			alert.Message = renderMessage(rule.Message, alert)
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// lookupBuffer returns the buffer for key, or nil if it doesn't exist.
// Unlike getOrCreateBuffer, never creates.
func (e *Engine) lookupBuffer(key string) *RingBuffer {
	e.bufMu.Lock()
	defer e.bufMu.Unlock()
	return e.buffers[key]
}

// parseLabelKey reverses LabelSetKey, returning the label map for an
// already-canonicalized "k1=v1,k2=v2" string. Returns nil for empty input.
func parseLabelKey(key string) map[string]string {
	if key == "" {
		return nil
	}
	out := map[string]string{}
	for _, p := range strings.Split(key, ",") {
		if eq := strings.IndexByte(p, '='); eq > 0 {
			out[p[:eq]] = p[eq+1:]
		}
	}
	return out
}

// RulesStatus returns rule names with their cooldown states.
func (e *Engine) RulesStatus() []RuleStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Snapshot seenLabelKeys under bufMu
	e.bufMu.Lock()
	seenCopy := make(map[string][]string, len(e.seenLabelKeys))
	for k, v := range e.seenLabelKeys {
		cp := make([]string, len(v))
		copy(cp, v)
		seenCopy[k] = cp
	}
	e.bufMu.Unlock()

	out := make([]RuleStatus, len(e.rules))
	for i, r := range e.rules {
		cooling := make(map[string]string)
		for _, labelKey := range seenCopy[r.Name] {
			cooling[labelKey] = e.cooldown.RemainingString(r.Name, labelKey)
		}
		out[i] = RuleStatus{
			Name:        r.Name,
			Condition:   r.Condition,
			Cooldown:    r.Cooldown.String(),
			CoolingDown: cooling,
		}
	}
	return out
}

// RuleStatus is used by the /rules HTTP endpoint.
type RuleStatus struct {
	Name        string
	Condition   string
	Cooldown    string
	CoolingDown map[string]string
}

// trackLabelKey records that a label-set key was seen for a rule.
func (e *Engine) trackLabelKey(ruleName, labelKey string) {
	e.bufMu.Lock()
	defer e.bufMu.Unlock()
	for _, k := range e.seenLabelKeys[ruleName] {
		if k == labelKey {
			return
		}
	}
	e.seenLabelKeys[ruleName] = append(e.seenLabelKeys[ruleName], labelKey)
}

// getOrCreateBuffer returns the ring buffer for a buffer key, creating it if needed.
// Uses bufMu independently of the RWMutex so it is safe to call from Process() under RLock.
func (e *Engine) getOrCreateBuffer(key string, window time.Duration) *RingBuffer {
	e.bufMu.Lock()
	defer e.bufMu.Unlock()
	if buf, ok := e.buffers[key]; ok {
		return buf
	}
	buf := NewRingBuffer(window, e.maxBuf)
	e.buffers[key] = buf
	return buf
}

// LabelSetKey serializes a label map to a canonical sorted string.
func LabelSetKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + labels[k]
	}
	return strings.Join(parts, ",")
}

// StartFlusher starts a background goroutine that periodically saves engine state to path.
// Returns a stop function that triggers a final flush and blocks until it completes.
// The caller should initialize stopFlusher to a no-op before calling this, so shutdown
// can call it unconditionally: var stopFlusher func() = func() {}
func (e *Engine) StartFlusher(path string, interval time.Duration) func() {
	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				snap := SnapshotEngine(e)
				if err := SaveSnapshot(path, snap); err != nil {
					log.Printf("ding: state flush failed: %v", err)
				}
			case <-stop:
				snap := SnapshotEngine(e)
				if err := SaveSnapshot(path, snap); err != nil {
					log.Printf("ding: final state flush failed: %v", err)
				}
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(stop) })
		<-done
	}
}

// checkGuard performs (or returns a cached result of) an HTTP guard check.
// Returns true if the alert should be allowed to fire.
func (e *Engine) checkGuard(ruleName string, g *config.GuardConfig, now time.Time) bool {
	// Check cache first.
	e.guardMu.Lock()
	entry, ok := e.guardCache[ruleName]
	if ok && now.Before(entry.expiresAt) {
		e.guardMu.Unlock()
		return entry.allowed
	}
	e.guardMu.Unlock()

	// Cache miss or expired — make the HTTP request.
	resp, err := e.guardHTTP.Get(g.URL) //nolint:noctx
	allowed := err == nil && resp != nil && resp.StatusCode == g.ExpectStatus
	if resp != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	e.guardMu.Lock()
	e.guardCache[ruleName] = guardEntry{allowed: allowed, expiresAt: now.Add(g.TTL)}
	e.guardMu.Unlock()
	return allowed
}

func renderMessage(tmpl string, alert Alert) string {
	if tmpl == "" {
		return fmt.Sprintf("rule %q fired (metric=%s value=%v)", alert.Rule, alert.Metric, alert.Value)
	}
	t, err := template.New("msg").Parse(tmpl)
	if err != nil {
		return tmpl // return raw if template is invalid
	}
	data := map[string]interface{}{
		"metric":   alert.Metric,
		"value":    alert.Value,
		"rule":     alert.Rule,
		"fired_at": alert.FiredAt.Format(time.RFC3339),
		"avg":      alert.Avg,
		"max":      alert.Max,
		"min":      alert.Min,
		"count":    alert.Count,
		"sum":      alert.Sum,
	}
	for k, v := range alert.Labels {
		data[k] = v
	}
	// Float-typed event fields are accessible too (e.g. duration_seconds
	// from the synthetic run.exit event, or any user-emitted numeric JSON
	// field). Skip-if-exists so Labels win when keys collide — matches the
	// "user-supplied labels are authoritative" principle.
	for k, v := range alert.Floats {
		if _, exists := data[k]; !exists {
			data[k] = v
		}
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return tmpl
	}
	return buf.String()
}
