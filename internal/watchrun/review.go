package watchrun

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watch"
	"gopkg.in/yaml.v3"
	"reflect"
	"sort"
)

type WatchPrecondition struct {
	Revision   string `json:"revision"`
	Generation int64  `json:"generation"`
	Status     string `json:"status"`
}
type ApplyPreconditions struct {
	Instance     string                       `json:"instance"`
	ManifestHash string                       `json:"manifestHash"`
	Watches      map[string]WatchPrecondition `json:"watches"`
	Destinations map[string]string            `json:"destinations"`
}

func captureReview(tx *store.Tx, b plan.Bundle, manifest string) (*ApplyPreconditions, error) {
	r := &ApplyPreconditions{ManifestHash: fmt.Sprintf("%x", sha256.Sum256([]byte(manifest))), Watches: map[string]WatchPrecondition{}, Destinations: map[string]string{}}
	var err error
	r.Instance, err = tx.Identity()
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, w := range b.Watches {
		old, err := tx.Watch(w.Definition.Metadata.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		r.Watches[w.Definition.Metadata.ID] = WatchPrecondition{old.Plan.Revision, old.Generation, old.Status}
		for _, d := range w.Definition.Spec.Destinations {
			ids[d.Ref] = true
		}
	}
	for _, d := range b.Destinations {
		ids[d.Definition.Metadata.ID] = true
	}
	for id := range ids {
		old, err := tx.Destination(id, "")
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		r.Destinations[id] = old.Revision
	}
	return r, nil
}
func sameReview(a, b *ApplyPreconditions) bool { return reflect.DeepEqual(a, b) }
func yamlDefinition(v any) string              { b, _ := yaml.Marshal(v); return string(b) }
func (a *App) BundleCredentials(b plan.Bundle, extra []watch.Destination) []CredentialHealth {
	names := map[string]bool{}
	add := func(r *watch.SecretRef) {
		if r != nil {
			names[r.Env] = true
		}
	}
	for _, w := range b.Watches {
		s := w.Definition.Spec.Source
		add(s.URLRef)
		for _, r := range s.Headers {
			add(&r)
		}
		for _, r := range s.Env {
			add(&r)
		}
	}
	for _, d := range b.Destinations {
		extra = append(extra, d.Definition)
	}
	for _, d := range extra {
		add(d.Spec.URLRef)
		for _, r := range d.Spec.Headers {
			add(&r)
		}
	}
	out := []CredentialHealth{}
	for name := range names {
		value, present := a.Lookup(name)
		out = append(out, CredentialHealth{Environment: name, Present: present && value != ""})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Environment < out[j].Environment })
	return out
}

// Bound the combined review as well as each input manifest. This is checked
// inside the transaction, so an oversized review never leaves partial writes.
func reviewSize(r ApplyResult) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if len(b) > 8<<20 {
		return fmt.Errorf("review exceeds 8 MiB; apply a smaller bundle")
	}
	return nil
}
