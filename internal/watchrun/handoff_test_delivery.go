package watchrun

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
)

type HandoffTestResult struct {
	Outcome string `json:"outcome"`
}

// TestHandoff tests a paused local target before moving back. The cloud boundary
// uses its own encrypted credential revisions and metered test receipts.
func (a *App) TestHandoff(ctx context.Context, id, key string) (HandoffTestResult, error) {
	out := HandoffTestResult{Outcome: "pending"}
	if !operationKey.MatchString(key) {
		return out, fmt.Errorf("stable test operation key required")
	}
	manifest, err := a.Export(ctx, func() string { h, _ := a.Handoff(ctx, id); return h.WatchID }())
	if err != nil {
		return out, err
	}
	b, err := plan.Parse([]byte(manifest))
	if err != nil || len(b.Watches) != 1 || len(b.Destinations) == 0 {
		return out, fmt.Errorf("prepared watch and remote destination required")
	}
	values := map[string]string{}
	add := func(ref *watch.SecretRef) error {
		if ref == nil {
			return nil
		}
		value, err := source.Resolve(ref, a.Lookup)
		if err != nil {
			return err
		}
		values[ref.Env] = value
		return nil
	}
	for _, ref := range b.Watches[0].Definition.Spec.Source.Headers {
		if err := add(&ref); err != nil {
			return out, err
		}
	}
	for _, d := range b.Destinations {
		if d.Definition.Spec.Type != "webhook" && d.Definition.Spec.Type != "slack" && d.Definition.Spec.Type != "discord" {
			return out, fmt.Errorf("return transfer requires a remote delivery destination")
		}
		if err := add(d.Definition.Spec.URLRef); err != nil {
			return out, err
		}
		for _, ref := range d.Definition.Spec.Headers {
			if err := add(&ref); err != nil {
				return out, err
			}
		}
	}
	digest, _ := watch.Revision(struct {
		Manifest    string
		Credentials map[string]string
	}{manifest, values})
	run := false
	err = a.Store.Update(ctx, func(tx *store.Tx) error {
		h, err := tx.Handoff(id)
		if err != nil {
			return err
		}
		if h.Role != "target" || h.Phase != "prepared" || !h.Held {
			return store.ErrConflict
		}
		if h.ProbeKey == key {
			if h.ProbeDigest != digest || a.Now().Sub(h.ProbeAt) > 30*time.Minute {
				return fmt.Errorf("test expired or credentials changed; explicitly start a new labeled test")
			}
			out.Outcome = h.ProbeOutcome
			return nil
		}
		h.ProbeKey = key
		h.ProbeDigest = digest
		h.ProbeAt = a.Now()
		h.ProbeOutcome = "pending"
		run = true
		return tx.SaveHandoff(h)
	})
	if err != nil || !run {
		return out, err
	}
	lookup := func(name string) (string, bool) { v, ok := values[name]; return v, ok }
	fetch := a.HTTP
	fetch.Lookup = lookup
	batch := fetch.Fetch(ctx, b.Watches[0], "", a.Now())
	accepted := len(batch.Observations) > 0
	for _, o := range batch.Observations {
		status, ok := o.Fields["http.status"].(int)
		if o.Health != "ok" || !ok || status < 200 || status >= 300 {
			accepted = false
		}
	}
	if accepted {
		for _, d := range b.Destinations {
			endpoint, err := source.Resolve(d.Definition.Spec.URLRef, lookup)
			if err != nil {
				accepted = false
				break
			}
			headers := http.Header{}
			for name, ref := range d.Definition.Spec.Headers {
				headers.Set(name, values[ref.Env])
			}
			payload, err := delivery.Render(d.Definition.Spec.Type, watch.Event{ID: key, WatchID: b.Watches[0].Definition.Metadata.ID, Type: "test", At: a.Now(), Message: "Ding transfer test: confirming local delivery before moving execution back."})
			if err != nil {
				accepted = false
				break
			}
			result := delivery.HTTP(ctx, a.HTTP.Client, delivery.Request{URL: endpoint, Body: payload, Header: headers, Provider: d.Definition.Spec.Type}, a.Now())
			if result.Outcome != delivery.Delivered {
				accepted = false
				break
			}
		}
	}
	out.Outcome = "failed"
	if accepted {
		out.Outcome = "accepted"
	}
	err = a.Store.Update(ctx, func(tx *store.Tx) error {
		h, err := tx.Handoff(id)
		if err != nil {
			return err
		}
		if h.ProbeKey != key || h.ProbeDigest != digest || h.Phase != "prepared" {
			return store.ErrConflict
		}
		h.ProbeOutcome = out.Outcome
		return tx.SaveHandoff(h)
	})
	return out, err
}
