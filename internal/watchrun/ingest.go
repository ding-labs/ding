package watchrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/transform"
)

var ErrBusy = errors.New("acquisition workers are busy")
var ErrInput = errors.New("invalid push input")

func (a *App) Reserve() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return ErrClosing
	}
	if a.inflight >= a.AcquisitionWorkers {
		return ErrBusy
	}
	a.inflight++
	return nil
}
func (a *App) Release() { a.mu.Lock(); a.inflight--; a.mu.Unlock() }
func (a *App) Record(ctx context.Context, id string) (store.WatchRecord, error) {
	var r store.WatchRecord
	err := a.Store.View(ctx, func(tx *store.Tx) error { var err error; r, err = tx.Watch(id); return err })
	return r, err
}

// IngestReserved assumes a shared acquisition slot is held by the HTTP handler
// while it reads the bounded body and calls this method.
func (a *App) IngestReserved(ctx context.Context, r store.WatchRecord, raw []byte, inputID string) (Receipt, error) {
	if r.Plan.Definition.Spec.Source.Type != "push" || r.Status != "running" {
		return Receipt{}, store.ErrStale
	}
	if inputID == "" {
		var token [24]byte
		if _, err := rand.Read(token[:]); err != nil {
			return Receipt{}, err
		}
		inputID = hex.EncodeToString(token[:])
	}
	if len(inputID) > 256 {
		return Receipt{}, ErrInput
	}
	spec := r.Plan.Definition.Spec
	outputs, err := transform.Project(ctx, raw, spec.Source.JQ, spec.Source.Fields, spec.Limits.MaxOutputs, spec.Limits.MaxBytes)
	if err != nil {
		return Receipt{}, fmt.Errorf("%w: invalid JSON projection or input limit", ErrInput)
	}
	observations, err := source.Observations(outputs, spec.Source.ObservedAtField)
	if err != nil {
		return Receipt{}, fmt.Errorf("%w: invalid observed time", ErrInput)
	}
	batch := source.Batch{Cursor: r.Cursor, Observations: observations}

	return a.accept(ctx, r, batch, inputID, a.Now)
}
