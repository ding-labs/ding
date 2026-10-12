package watchrun

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/source"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/transform"
)

// PushInput is supplied only by authenticated server code. Do not decode it
// from a producer payload. Callers fence permission mutations during Replay
// and Commit; projection deliberately runs outside that fence.
var ErrProducerExpired = fmt.Errorf("producer credential expired before commit")

type PushInput struct {
	ValidUntil            time.Time
	Producer, Key, Digest string
	Bytes, MaxReceipts    int
	Horizon               time.Duration
	Receipt               store.PushReceipt
}

func NewPushInput(producer, key string, raw []byte, maxReceipts int, horizon time.Duration) PushInput {
	sum := sha256.Sum256(raw)
	return PushInput{Producer: producer, Key: key, Digest: hex.EncodeToString(sum[:]), Bytes: len(raw), MaxReceipts: maxReceipts, Horizon: horizon}
}

func (a *App) ReplayPush(ctx context.Context, record store.WatchRecord, input PushInput) (store.PushReceipt, error) {
	var out store.PushReceipt
	err := a.Store.View(ctx, func(tx *store.Tx) error {
		if !input.ValidUntil.IsZero() && !a.Now().Before(input.ValidUntil) {
			return ErrProducerExpired
		}
		current, err := tx.Watch(record.Plan.Definition.Metadata.ID)
		if err != nil {
			return err
		}
		if current.Status == "deleted" || current.Lifetime != record.Lifetime {
			return store.ErrStale
		}
		out, err = tx.PushReceipt(record.Lifetime, input.Producer, input.Key, input.Digest, a.Now())
		return err
	})
	return out, err
}

func PreparePush(ctx context.Context, record store.WatchRecord, raw []byte, maxDepth, maxNodes int) (source.Batch, error) {
	spec := record.Plan.Definition.Spec
	if spec.Source.Type != "push" || record.Status != "running" {
		return source.Batch{}, store.ErrStale
	}
	if spec.Source.JQ != "" {
		return source.Batch{}, fmt.Errorf("%w: hosted push requires declarative fields", ErrInput)
	}
	if err := transform.ValidateJSON(ctx, raw, maxDepth, maxNodes); err != nil {
		return source.Batch{}, fmt.Errorf("%w: %w", ErrInput, err)
	}
	outputs, err := transform.Project(ctx, raw, "", spec.Source.Fields, spec.Limits.MaxOutputs, spec.Limits.MaxBytes, spec.Source.ItemsPath)
	if err != nil {
		return source.Batch{}, fmt.Errorf("%w: %w", ErrInput, err)
	}
	for _, fields := range outputs {
		for _, group := range spec.GroupBy {
			if v, ok := fields[group]; !ok || v == nil {
				return source.Batch{}, fmt.Errorf("%w: grouping fields must be present and non-null", ErrInput)
			}
		}
	}
	observations, err := source.Observations(outputs, spec.Source.ObservedAtField)
	if err != nil {
		return source.Batch{}, fmt.Errorf("%w: invalid observation time", ErrInput)
	}
	return source.Batch{Cursor: record.Cursor, Observations: observations}, nil
}

func (a *App) CommitPush(ctx context.Context, record store.WatchRecord, batch source.Batch, input PushInput) (store.PushReceipt, error) {
	if input.Producer == "" || input.Key == "" || len(input.Key) > 256 || input.MaxReceipts < 1 || input.Horizon <= 0 {
		return store.PushReceipt{}, ErrInput
	}
	var id [24]byte
	if _, err := rand.Read(id[:]); err != nil {
		return store.PushReceipt{}, err
	}
	_, err := a.acceptPush(ctx, record, batch, hex.EncodeToString(id[:]), a.Now, &input)
	return input.Receipt, err
}
