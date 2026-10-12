package cloud

import (
	"fmt"
	"golang.org/x/time/rate"
	"time"
)

type PushConfig struct {
	MinFreeBytes        int64    `json:"minFreeBytes,omitempty"`
	NativeWebhooks      bool     `json:"nativeWebhooks,omitempty"`
	Disabled            bool     `json:"disabled,omitempty"`
	Workspaces          []string `json:"workspaces,omitempty"`
	HostConcurrent      int      `json:"hostConcurrent,omitempty"`
	WorkspaceConcurrent int      `json:"workspaceConcurrent,omitempty"`
	WorkspaceRate       int      `json:"workspaceRate,omitempty"`
	WorkspaceBurst      int      `json:"workspaceBurst,omitempty"`
	ProducerRate        int      `json:"producerRate,omitempty"`
	ProducerBurst       int      `json:"producerBurst,omitempty"`
	BytesPerSecond      int      `json:"bytesPerSecond,omitempty"`
	ByteBurst           int      `json:"byteBurst,omitempty"`
	ReadSeconds         int      `json:"readSeconds,omitempty"`
	MaxDepth            int      `json:"maxDepth,omitempty"`
	MaxNodes            int      `json:"maxNodes,omitempty"`
	MaxReceipts         int      `json:"maxReceipts,omitempty"`
	ReceiptSeconds      int      `json:"receiptSeconds,omitempty"`
}

func (p PushConfig) effective() PushConfig {
	if p.MinFreeBytes == 0 {
		p.MinFreeBytes = 64 << 20
	}
	if p.HostConcurrent == 0 {
		p.HostConcurrent = 16
	}
	if p.WorkspaceConcurrent == 0 {
		p.WorkspaceConcurrent = 2
	}
	if p.WorkspaceRate == 0 {
		p.WorkspaceRate = 20
	}
	if p.WorkspaceBurst == 0 {
		p.WorkspaceBurst = 40
	}
	if p.ProducerRate == 0 {
		p.ProducerRate = 10
	}
	if p.ProducerBurst == 0 {
		p.ProducerBurst = 20
	}
	if p.BytesPerSecond == 0 {
		p.BytesPerSecond = 1 << 20
	}
	if p.ByteBurst == 0 {
		p.ByteBurst = 2 << 20
	}
	if p.ReadSeconds == 0 {
		p.ReadSeconds = 5
	}
	if p.MaxDepth == 0 {
		p.MaxDepth = 32
	}
	if p.MaxNodes == 0 {
		p.MaxNodes = 32000
	}
	if p.MaxReceipts == 0 {
		p.MaxReceipts = 100000
	}
	if p.ReceiptSeconds == 0 {
		p.ReceiptSeconds = 86400
	}
	return p
}
func (p PushConfig) Validate(body int) error {
	p = p.effective()
	if p.MinFreeBytes < 1<<20 || p.MinFreeBytes > 1<<40 {
		return fmt.Errorf("push disk reserve must be 1 MiB–1 TiB")
	}
	if p.HostConcurrent < 1 || p.HostConcurrent > 256 || p.WorkspaceConcurrent < 1 || p.WorkspaceConcurrent > p.HostConcurrent || p.WorkspaceRate < 1 || p.WorkspaceRate > 10000 || p.WorkspaceBurst < 1 || p.WorkspaceBurst > 10000 || p.ProducerRate < 1 || p.ProducerRate > p.WorkspaceRate || p.ProducerBurst < 1 || p.ProducerBurst > p.WorkspaceBurst || p.BytesPerSecond < 1024 || p.BytesPerSecond > 1<<30 || p.ByteBurst < body || p.ByteBurst > 1<<30 || p.ReadSeconds < 1 || p.ReadSeconds > 10 || p.MaxDepth < 1 || p.MaxDepth > 64 || p.MaxNodes < 1 || p.MaxNodes > 100000 || p.MaxReceipts < 1 || p.MaxReceipts > 1000000 || p.ReceiptSeconds < 60 || p.ReceiptSeconds > 604800 {
		return fmt.Errorf("invalid push admission configuration")
	}
	for _, id := range p.Workspaces {
		if !accountID.MatchString(id) {
			return fmt.Errorf("push workspaces must contain workspace IDs")
		}
	}
	return nil
}

type pushBudget struct {
	requests, bytes *rate.Limiter
	active          int
	used            time.Time
}

// Registry entries cannot be evicted while their rate debt or active work
// remains. Churning credentials therefore cannot reset a workspace budget.
func (s *Server) admitPush(workspace, producer string, bytes int) (func(), bool) {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	now := time.Now()
	p := s.Push.effective()
	keys := []string{"workspace:" + workspace, "producer:" + workspace + ":" + producer}
	entries := make([]*pushBudget, 0, 2)
	for i, key := range keys {
		b := s.pushBudgets[key]
		if b == nil {
			if len(s.pushBudgets) >= MaxAccounts*202 {
				for k, v := range s.pushBudgets {
					if v.active == 0 && now.Sub(v.used) > 10*time.Minute && v.requests.Tokens() >= float64(v.requests.Burst()) && v.bytes.Tokens() >= float64(v.bytes.Burst()) {
						delete(s.pushBudgets, k)
					}
				}
				if len(s.pushBudgets) >= MaxAccounts*202 {
					return nil, false
				}
			}
			r, burst := p.WorkspaceRate, p.WorkspaceBurst
			if i == 1 {
				r, burst = p.ProducerRate, p.ProducerBurst
			}
			b = &pushBudget{requests: rate.NewLimiter(rate.Limit(r), burst), bytes: rate.NewLimiter(rate.Limit(p.BytesPerSecond), p.ByteBurst)}
			s.pushBudgets[key] = b
		}
		b.used = now
		entries = append(entries, b)
	}
	// Charge attempts even when an authenticated producer is already busy.
	allowed := true
	for _, b := range entries {
		if !b.requests.AllowN(now, 1) {
			allowed = false
		}
		if !b.bytes.AllowN(now, bytes) {
			allowed = false
		}
	}
	if !allowed || entries[0].active >= p.WorkspaceConcurrent {
		return nil, false
	}
	select {
	case s.pushSlots <- struct{}{}:
	default:
		return nil, false
	}
	for _, b := range entries {
		b.active++
	}
	return func() {
		s.pushMu.Lock()
		defer s.pushMu.Unlock()
		for _, b := range entries {
			b.active--
		}
		<-s.pushSlots
	}, true
}

// Signature verification has a separate per-workspace slot, so one sender's
// stalled unsigned bodies do not consume every native webhook connection.
func (s *Server) admitWebhookBody(workspace string) (func(), bool) {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	key := "signature:" + workspace
	b := s.pushBudgets[key]
	if b == nil {
		if len(s.pushBudgets) >= MaxAccounts*202 {
			return nil, false
		}
		b = &pushBudget{requests: rate.NewLimiter(20, 40), bytes: rate.NewLimiter(1<<20, 2<<20)}
		s.pushBudgets[key] = b
	}
	b.used = time.Now()
	if !b.requests.Allow() || b.active > 0 {
		return nil, false
	}
	select {
	case s.hookSlots <- struct{}{}:
	default:
		return nil, false
	}
	b.active++
	return func() { s.pushMu.Lock(); defer s.pushMu.Unlock(); b.active--; <-s.hookSlots }, true
}
