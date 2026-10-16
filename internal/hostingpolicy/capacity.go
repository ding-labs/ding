package hostingpolicy

import (
	"fmt"
	"github.com/ding-labs/ding/internal/watchrun"
	"time"
)

// Capacity is operator configuration, separate from source admission switches.
// Zero values select documented defaults. The server additionally checks its
// backend backup envelope; these are operator bounds, not product tiers.
type Capacity struct {
	APIRate            int   `json:"apiRate,omitempty"`
	APIBurst           int   `json:"apiBurst,omitempty"`
	OutboundChecks     int   `json:"outboundChecks,omitempty"`
	OutboundDeliveries int   `json:"outboundDeliveries,omitempty"`
	OutboundBytes      int64 `json:"outboundBytes,omitempty"`

	MaxWatches      int   `json:"maxWatches,omitempty"`
	MaxDestinations int   `json:"maxDestinations,omitempty"`
	MaxPending      int   `json:"maxPending,omitempty"`
	MaxStoreBytes   int64 `json:"maxStoreBytes,omitempty"`
	RetentionDays   int   `json:"retentionDays,omitempty"`
	MinPollSeconds  int   `json:"minPollSeconds,omitempty"`
	MaxBodyBytes    int   `json:"maxBodyBytes,omitempty"`
	MaxEntities     int   `json:"maxEntities,omitempty"`
	MaxSamples      int   `json:"maxSamples,omitempty"`
	MaxOutputs      int   `json:"maxOutputs,omitempty"`
}

func (c Capacity) Effective() Capacity {
	if c.APIRate == 0 {
		c.APIRate = 20
	}
	if c.APIBurst == 0 {
		c.APIBurst = 100
	}
	if c.MaxWatches == 0 {
		c.MaxWatches = 1000
	}
	if c.MaxDestinations == 0 {
		c.MaxDestinations = 100
	}
	if c.MaxPending == 0 {
		c.MaxPending = 10000
	}
	if c.MaxStoreBytes == 0 {
		c.MaxStoreBytes = 128 << 20
	}
	if c.RetentionDays == 0 {
		c.RetentionDays = 7
	}
	if c.MinPollSeconds == 0 {
		c.MinPollSeconds = 1
	}
	if c.MaxBodyBytes == 0 {
		c.MaxBodyBytes = 1 << 20
	}
	if c.MaxEntities == 0 {
		c.MaxEntities = 1000
	}
	if c.MaxSamples == 0 {
		c.MaxSamples = 10000
	}
	if c.MaxOutputs == 0 {
		c.MaxOutputs = 100
	}
	return c
}
func (c Capacity) Validate() error {
	c = c.Effective()
	if c.APIRate < 1 || c.APIRate > 1000 || c.APIBurst < 1 || c.APIBurst > 10000 {
		return fmt.Errorf("invalid workspace API admission bounds")
	}
	if c.OutboundChecks < 0 || c.OutboundDeliveries < 0 || c.OutboundBytes < 0 {
		return fmt.Errorf("outbound budgets must be nonnegative; zero disables cumulative quotas")
	}
	if c.MaxWatches < 1 || c.MaxWatches > 10000 || c.MaxDestinations < 1 || c.MaxDestinations > 1000 || c.MaxPending < 1 || c.MaxPending > 100000 || c.MaxStoreBytes < 1<<20 || c.MaxStoreBytes > 1<<40 || c.RetentionDays < 1 || c.RetentionDays > 30 || c.MinPollSeconds < 1 || c.MinPollSeconds > 86400 || c.MaxBodyBytes < 1024 || c.MaxBodyBytes > 1<<20 || c.MaxEntities < 1 || c.MaxEntities > 10000 || c.MaxSamples < 1 || c.MaxSamples > 100000 || c.MaxOutputs < 1 || c.MaxOutputs > 100 {
		return fmt.Errorf("invalid cloud capacity; storage and watch bounds must fit their supported representation")
	}
	return nil
}
func (c Capacity) Limits() watchrun.Limits {
	c = c.Effective()
	return watchrun.Limits{MaxWatches: c.MaxWatches, MaxDestinations: c.MaxDestinations, MaxPending: c.MaxPending, MaxBytes: c.MaxStoreBytes, Retention: time.Duration(c.RetentionDays) * 24 * time.Hour}
}
