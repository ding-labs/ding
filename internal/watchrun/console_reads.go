package watchrun

import (
	"context"
	"github.com/ding-labs/ding/internal/store"
	"time"
)

type Status struct {
	Instance     string    `json:"instance"`
	Running      bool      `json:"running"`
	Closing      bool      `json:"closing"`
	Acquisitions int       `json:"acquisitions"`
	LastError    string    `json:"lastError"`
	At           time.Time `json:"at"`
}

func (a *App) Status(ctx context.Context) (Status, error) {
	a.mu.Lock()
	s := Status{Running: a.running, Closing: a.closing, Acquisitions: a.inflight, LastError: a.lastError, At: a.Now()}
	a.mu.Unlock()
	err := a.Store.View(ctx, func(tx *store.Tx) error { var err error; s.Instance, err = tx.Identity(); return err })
	return s, err
}
func (a *App) ConsoleWatches(ctx context.Context, q store.ConsoleQuery) (store.WatchPage, error) {
	var page store.WatchPage
	err := a.Store.View(ctx, func(tx *store.Tx) error {
		refs, err := tx.SecretNames()
		if err != nil {
			return err
		}
		missing := map[string][]string{}
		for id, names := range refs {
			for _, name := range names {
				value, present := a.Lookup(name)
				if !present || value == "" {
					missing[id] = append(missing[id], name)
				}
			}
		}
		page, err = tx.ConsoleWatches(q, missing)
		return err
	})
	return page, err
}
