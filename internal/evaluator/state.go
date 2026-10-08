package evaluator

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/ding-labs/ding/internal/identity"
)

const StateVersion = 2

type StateSnapshot struct {
	Version      int                             `json:"version"`
	SavedAt      time.Time                       `json:"saved_at"`
	Fingerprints map[string]string               `json:"fingerprints"`
	Seen         map[string]map[string]time.Time `json:"seen"`
	Buffers      map[string]BufferSnapshot       `json:"buffers"`
	Cooldowns    map[string]time.Time            `json:"cooldowns"`
}

type BufferSnapshot struct {
	Window     time.Duration   `json:"window_ns"`
	MaxSize    int             `json:"max_size"`
	RunBounded bool            `json:"run_bounded,omitempty"`
	Entries    []EntrySnapshot `json:"entries"`
}
type EntrySnapshot struct {
	Value float64   `json:"value"`
	At    time.Time `json:"at"`
}

type RestoreReport struct {
	ResetRules []string `json:"reset_rules,omitempty"`
}

func ruleFingerprint(r parsedRule, maxBuf int) string {
	// Presentation and destinations do not change the meaning of retained state.
	r.Message = ""
	r.Alerts = nil
	b, _ := json.Marshal(struct {
		Rule      EngineRule
		MaxBuffer int
	}{r.EngineRule, maxBuf})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// SnapshotEngine excludes in-progress evaluation for a coherent state boundary.
func SnapshotEngine(e *Engine) StateSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	snap := StateSnapshot{Version: StateVersion, SavedAt: time.Now().UTC(), Fingerprints: map[string]string{}, Seen: map[string]map[string]time.Time{}, Buffers: map[string]BufferSnapshot{}, Cooldowns: map[string]time.Time{}}
	for _, r := range e.rules {
		snap.Fingerprints[r.Name] = ruleFingerprint(r, e.maxBuf)
	}
	e.bufMu.Lock()
	for rule, labels := range e.seenLabelKeys {
		snap.Seen[rule] = map[string]time.Time{}
		for key, at := range labels {
			snap.Seen[rule][key] = at.UTC()
		}
	}
	for key, rb := range e.buffers {
		rb.mu.Lock()
		entries := make([]EntrySnapshot, len(rb.entries))
		for i, ent := range rb.entries {
			entries[i] = EntrySnapshot{ent.value, ent.at.UTC()}
		}
		snap.Buffers[key] = BufferSnapshot{rb.window, rb.maxSize, rb.runBounded, entries}
		rb.mu.Unlock()
	}
	e.bufMu.Unlock()
	e.cooldown.mu.Lock()
	for key, exp := range e.cooldown.expiry {
		snap.Cooldowns[key] = exp.UTC()
	}
	e.cooldown.mu.Unlock()
	return snap
}

// RestoreEngine builds replacement state before publishing it. Incompatible
// rules reset with a report; malformed state fails without changing the engine.
func RestoreEngine(e *Engine, snap StateSnapshot, now time.Time) (RestoreReport, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	report := RestoreReport{}
	if snap.Version != StateVersion || snap.Fingerprints == nil {
		return report, fmt.Errorf("incompatible snapshot version/metadata; preserve the file and explicitly reset state")
	}
	compatible := map[string]parsedRule{}
	current := map[string]parsedRule{}
	for _, r := range e.rules {
		current[r.Name] = r
	}
	for name, fp := range snap.Fingerprints {
		if r, ok := current[name]; ok && fp == ruleFingerprint(r, e.maxBuf) {
			compatible[name] = r
		} else {
			report.ResetRules = append(report.ResetRules, name)
		}
	}
	sort.Strings(report.ResetRules)
	buffers := map[string]*RingBuffer{}
	seen := map[string]map[string]time.Time{}
	cooldowns := map[string]time.Time{}
	for name, labels := range snap.Seen {
		if _, ok := snap.Fingerprints[name]; !ok {
			return report, fmt.Errorf("missing rule fingerprint for saved labels")
		}
		if _, ok := compatible[name]; !ok {
			continue
		}
		seen[name] = map[string]time.Time{}
		for key, at := range labels {
			if key != "" && LabelSetKey(parseLabelKey(key)) != key {
				return report, fmt.Errorf("invalid label identity in snapshot")
			}
			seen[name][key] = at
		}
	}
	remember := func(rule, key string, at time.Time) {
		if seen[rule] == nil {
			seen[rule] = map[string]time.Time{}
		}
		if prev, ok := seen[rule][key]; !ok || at.After(prev) {
			seen[rule][key] = at
		}
	}
	for key, bs := range snap.Buffers {
		parts, err := identity.Parts(key)
		if err != nil || len(parts) != 3 {
			return report, fmt.Errorf("invalid buffer identity in snapshot")
		}
		if _, ok := snap.Fingerprints[parts[0]]; !ok {
			return report, fmt.Errorf("missing rule fingerprint for saved buffer")
		}
		r, ok := compatible[parts[0]]
		if !ok {
			continue
		}
		leafID, err := strconv.Atoi(parts[1])
		if err != nil {
			return report, fmt.Errorf("invalid leaf identity in snapshot")
		}
		var leaf *windowedLeaf
		for _, l := range r.expr.collectWindowedLeaves() {
			if l.ID == leafID {
				copy := l
				leaf = &copy
				break
			}
		}
		if leaf == nil || bs.Window != leaf.Window || bs.RunBounded != leaf.RunBounded || bs.MaxSize != e.maxBuf || len(bs.Entries) > e.maxBuf {
			return report, fmt.Errorf("incompatible buffer metadata for rule %q", r.Name)
		}
		if parts[2] != "" && LabelSetKey(parseLabelKey(parts[2])) != parts[2] {
			return report, fmt.Errorf("invalid buffer labels")
		}
		rb := NewRingBuffer(leaf.Window, e.maxBuf, leaf.RunBounded)
		for _, ent := range bs.Entries {
			if ent.At.IsZero() {
				return report, fmt.Errorf("invalid entry timestamp")
			}
			if leaf.RunBounded || ent.At.After(now.Add(-leaf.Window)) {
				rb.entries = append(rb.entries, entry{ent.Value, ent.At})
				remember(r.Name, parts[2], ent.At)
			}
		}
		sort.SliceStable(rb.entries, func(i, j int) bool { return rb.entries[i].at.Before(rb.entries[j].at) })
		if len(rb.entries) > 0 {
			buffers[key] = rb
		}
	}
	for key, exp := range snap.Cooldowns {
		parts, err := identity.Parts(key)
		if err != nil || len(parts) != 2 {
			return report, fmt.Errorf("invalid cooldown identity in snapshot")
		}
		if _, ok := snap.Fingerprints[parts[0]]; !ok {
			return report, fmt.Errorf("missing rule fingerprint for saved cooldown")
		}
		if _, ok := compatible[parts[0]]; ok && exp.After(now) {
			if parts[1] != "" && LabelSetKey(parseLabelKey(parts[1])) != parts[1] {
				return report, fmt.Errorf("invalid cooldown labels")
			}
			cooldowns[key] = exp
			remember(parts[0], parts[1], now)
		}
	}
	// Apply limits before publishing. Expired idle groups cannot consume quota.
	count := 0
	for name, labels := range seen {
		for key, last := range labels {
			active := cooldowns[identity.Key(name, key)].After(now)
			for _, leaf := range compatible[name].expr.collectWindowedLeaves() {
				if buffers[identity.Key(name, strconv.Itoa(leaf.ID), key)] != nil {
					active = true
					break
				}
			}
			if !active && now.Sub(last) >= e.limits.IdleTTL {
				delete(labels, key)
			}
		}
		count += len(labels)
	}
	if count > e.limits.MaxLabelSets {
		return report, fmt.Errorf("snapshot exceeds label-set limit %d", e.limits.MaxLabelSets)
	}
	e.bufMu.Lock()
	e.buffers = buffers
	e.seenLabelKeys = seen
	e.labelCount = count
	e.bufMu.Unlock()
	e.cooldown.mu.Lock()
	e.cooldown.expiry = cooldowns
	e.cooldown.mu.Unlock()
	e.lastSweep = now
	return report, nil
}

// LoadSnapshot reads a snapshot from path.
// Returns nil, nil if the file does not exist (first start).
// Returns an error if the file exists but is corrupt or has an unsupported version.
func LoadSnapshot(path string) (*StateSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	var snap StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	if snap.Version != StateVersion || snap.Fingerprints == nil {
		return nil, fmt.Errorf("unsupported state version or metadata (%d); preserve the file and explicitly reset state", snap.Version)
	}
	return &snap, nil
}

// SaveSnapshot atomically replaces the file after syncing a private temporary
// file. The old snapshot survives encoding, writing, or rename failures.
func SaveSnapshot(path string, snap StateSnapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".ding-state-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
