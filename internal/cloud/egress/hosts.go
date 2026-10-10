package egress

import (
	"fmt"
	"golang.org/x/time/rate"
	"net/http"
	"strings"
	"sync"
	"time"
)

type hostEntry struct {
	limiter *rate.Limiter
	last    time.Time
}
type HostLimits struct {
	next  http.RoundTripper
	mu    sync.Mutex
	hosts map[string]*hostEntry
}

func NewHostLimits(next http.RoundTripper) *HostLimits {
	return &HostLimits{next: next, hosts: map[string]*hostEntry{}}
}
func (h *HostLimits) RoundTrip(r *http.Request) (*http.Response, error) {
	host := strings.ToLower(r.URL.Hostname())
	now := time.Now()
	h.mu.Lock()
	entry := h.hosts[host]
	if entry == nil {
		if len(h.hosts) >= 4096 {
			for key, value := range h.hosts {
				if now.Sub(value.last) > 10*time.Minute {
					delete(h.hosts, key)
				}
			}
		}
		if len(h.hosts) >= 4096 {
			h.mu.Unlock()
			return nil, fmt.Errorf("outbound host capacity reached")
		}
		entry = &hostEntry{limiter: rate.NewLimiter(2, 2)}
		h.hosts[host] = entry
	}
	entry.last = now
	h.mu.Unlock()
	// Wait before taking a global connection slot. A hot destination must not
	// occupy every slot while its rate allowance replenishes.
	if err := entry.limiter.Wait(r.Context()); err != nil {
		return nil, err
	}
	return h.next.RoundTrip(r)
}
