package evaluator

import (
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/identity"
)

// CooldownTracker tracks per-(rule, label-set) expiry in the evaluation clock.
type CooldownTracker struct {
	mu     sync.Mutex
	expiry map[string]time.Time
}

func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{expiry: make(map[string]time.Time)}
}

// TryAcquire checks and reserves a cooldown atomically. Zero duration allows
// every event and retains no state. Exact expiry is eligible to fire again.
func (ct *CooldownTracker) TryAcquire(rule, labelKey string, d time.Duration, now time.Time) bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	key := identity.Key(rule, labelKey)
	if now.Before(ct.expiry[key]) {
		return false
	}
	if d > 0 {
		ct.expiry[key] = now.Add(d)
	} else {
		delete(ct.expiry, key)
	}
	return true
}

func (ct *CooldownTracker) IsActive(rule, labelKey string, now time.Time) bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return now.Before(ct.expiry[identity.Key(rule, labelKey)])
}

func (ct *CooldownTracker) Set(rule, labelKey string, d time.Duration, now time.Time) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.expiry[identity.Key(rule, labelKey)] = now.Add(d)
}

func (ct *CooldownTracker) RemainingString(rule, labelKey string, now time.Time) string {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	exp := ct.expiry[identity.Key(rule, labelKey)]
	if !now.Before(exp) {
		return "ready"
	}
	return exp.Sub(now).Round(time.Second).String() + " remaining"
}
