package evaluator

import (
	"strconv"
	"time"

	"github.com/ding-labs/ding/internal/identity"
)

// StateLimits bounds the number of rule/group combinations, not just samples
// within each rolling buffer. IdleTTL never overrides active windows/cooldowns.
type StateLimits struct {
	MaxLabelSets int
	IdleTTL      time.Duration
}

type StateStats struct {
	LabelSets int    `json:"label_sets"`
	Buffers   int    `json:"buffers"`
	Rejected  uint64 `json:"rejected"`
}

func (e *Engine) StateStats() StateStats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return StateStats{e.labelCount, len(e.buffers), e.rejected}
}

func (e *Engine) Sweep(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sweepLocked(now)
}

// Caller holds the engine write lock, so a sweep cannot remove a buffer while
// an event is being evaluated or a snapshot is being captured.
func (e *Engine) sweepLocked(now time.Time) {
	e.bufMu.Lock()
	defer e.bufMu.Unlock()
	for key, buf := range e.buffers {
		if !buf.HasEntries(now) {
			delete(e.buffers, key)
		}
	}
	e.cooldown.Prune(now)
	for _, rule := range e.rules {
		for key, last := range e.seenLabelKeys[rule.Name] {
			if now.Sub(last) < e.limits.IdleTTL || e.cooldown.IsActive(rule.Name, key, now) {
				continue
			}
			active := false
			for _, leaf := range rule.expr.collectWindowedLeaves() {
				if e.buffers[identity.Key(rule.Name, strconv.Itoa(leaf.ID), key)] != nil {
					active = true
					break
				}
			}
			if !active {
				delete(e.seenLabelKeys[rule.Name], key)
				e.labelCount--
			}
		}
	}
	e.lastSweep = now
}
