package watchrun

import (
	"github.com/ding-labs/ding/internal/store"
	"strings"
	"testing"
	"time"
)

func transferFixture(t *testing.T) (*App, *App, HandoffPrepare, HandoffPause) {
	t.Helper()
	source, _ := setup(t)
	target, _ := setup(t)
	record := apply(t, source, "https://example.com")
	input(t, source, record, 1, 200)
	source.Now = func() time.Time { return start.Add(6 * time.Second) }
	var instance string
	_ = source.Store.View(ctx, func(tx *store.Tx) error { var e error; instance, e = tx.Identity(); return e })
	exported, err := source.Export(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := target.Apply(ctx, ApplyRequest{Manifest: exported, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	prepare := HandoffPrepare{ID: strings.Repeat("a", 64), Peer: instance, Manifest: exported, Review: preview.Review}
	h, err := target.PrepareHandoff(ctx, prepare)
	if err != nil {
		t.Fatal(err)
	}
	return source, target, prepare, HandoffPause{ID: h.ID, WatchID: "api", Peer: h.Instance, Revision: record.Plan.Revision, Generation: record.Generation, Destinations: h.Destinations}
}
func TestHandoffLostRepliesRestartAndNoDuplicateExecution(t *testing.T) {
	source, target, prepare, pause := transferFixture(t)
	record, _ := target.Record(ctx, "api")
	if record.Status != "paused" || !record.LastInputAt.IsZero() {
		t.Fatal("target executed before handoff")
	}
	if _, err := target.Lifecycle(ctx, "api", LifecycleRequest{Action: "resume"}); err == nil {
		t.Fatal("ordinary resume bypassed prepared hold")
	}
	if _, err := target.Apply(ctx, ApplyRequest{Manifest: prepare.Manifest}); err == nil {
		t.Fatal("ordinary apply bypassed hold")
	}
	if _, err := target.PrepareHandoff(ctx, prepare); err != nil {
		t.Fatal("lost prepare reply could not reconcile", err)
	}
	receipt, err := source.PauseForHandoff(ctx, pause)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.PauseForHandoff(ctx, pause); err != nil {
		t.Fatal("lost pause reply could not reconcile", err)
	}
	if _, err := source.Lifecycle(ctx, "api", LifecycleRequest{Action: "resume"}); err == nil {
		t.Fatal("source resumed after uncertain activation")
	}
	wrong := receipt
	wrong.Instance = "other"
	if _, err := target.ActivateHandoff(ctx, prepare.ID, wrong); err == nil {
		t.Fatal("wrong source accepted")
	}
	active, err := target.ActivateHandoff(ctx, prepare.ID, receipt)
	if err != nil || active.Phase != "active" {
		t.Fatal(active, err)
	}
	if _, err := target.ActivateHandoff(ctx, prepare.ID, receipt); err != nil {
		t.Fatal("lost activation reply could not reconcile", err)
	}
	if _, err := target.CancelPreparedHandoff(ctx, prepare.ID); err == nil {
		t.Fatal("active cloud incorrectly confirmed canceled")
	}
	record, _ = source.Record(ctx, "api")
	if record.Status != "paused" {
		t.Fatal("source still running")
	}
	record, _ = target.Record(ctx, "api")
	if record.Status != "running" {
		t.Fatal("target inactive")
	}
	// A fresh App over the durable store still enforces ownership, without any
	// in-memory knowledge of how the transfer arrived at this phase.
	restarted := New(source.Store)
	if _, err := restarted.Lifecycle(ctx, "api", LifecycleRequest{Action: "resume"}); err == nil {
		t.Fatal("restart lost ownership fence")
	}
}

func TestCanceledReturnKeepsOriginalLocalCopyFenced(t *testing.T) {
	local, cloud, original, pause := transferFixture(t)
	proof, err := local.PauseForHandoff(ctx, pause)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.ActivateHandoff(ctx, original.ID, proof); err != nil {
		t.Fatal(err)
	}
	r, _ := cloud.Record(ctx, "api")
	input(t, cloud, r, 1, 200)
	cloud.Now = func() time.Time { return start.Add(6 * time.Second) }
	back, err := cloud.PreflightHandoff(ctx, "api")
	if err != nil || !back.Ready {
		t.Fatal(back, err)
	}
	review, err := local.Apply(ctx, ApplyRequest{Manifest: back.Manifest, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("c", 64)
	if _, err := local.PrepareHandoff(ctx, HandoffPrepare{ID: id, ReturnOf: original.ID, Peer: back.Instance, Manifest: back.Manifest, Review: review.Review}); err != nil {
		t.Fatal(err)
	}
	if _, err := local.CancelPreparedHandoff(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := local.CancelPreparedHandoff(ctx, id); err != nil {
		t.Fatal("lost cancellation reply did not reconcile", err)
	}
	if _, err := local.Lifecycle(ctx, "api", LifecycleRequest{Action: "resume"}); err == nil {
		t.Fatal("canceled return enabled dual execution")
	}
	r, _ = cloud.Record(ctx, "api")
	if r.Status != "running" {
		t.Fatal("canceling a prepared return stopped its source")
	}
	r, _ = local.Record(ctx, "api")
	if r.Status != "paused" {
		t.Fatal("original local watch was deleted or resumed")
	}
	h, err := local.Handoff(ctx, original.ID)
	if err != nil || !h.Held || h.Phase != "paused" {
		t.Fatal("original ownership fence lost", h, err)
	}
}
func TestHandoffCanceledTargetAllowsExplicitSourceRecovery(t *testing.T) {
	source, target, prepare, pause := transferFixture(t)
	if _, err := source.PauseForHandoff(ctx, pause); err != nil {
		t.Fatal(err)
	}
	canceled, err := target.CancelPreparedHandoff(ctx, prepare.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ReleaseHandoff(ctx, prepare.ID, canceled); err != nil {
		t.Fatal(err)
	}
	record, _ := source.Record(ctx, "api")
	if record.Status != "paused" {
		t.Fatal("release silently resumed source")
	}
	if _, err := source.Lifecycle(ctx, "api", LifecycleRequest{Action: "resume"}); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ActivateHandoff(ctx, prepare.ID, store.Handoff{}); err == nil {
		t.Fatal("canceled target resurrected")
	}
}
func TestHandoffRefusesChangedRevisionAndUnhealthyState(t *testing.T) {
	source, _, _, pause := transferFixture(t)
	pause.Generation++
	if _, err := source.PauseForHandoff(ctx, pause); err == nil {
		t.Fatal("stale generation paused")
	}
	pause.Generation--
	record, _ := source.Record(ctx, "api")
	input(t, source, record, 2, 500)
	if _, err := source.PauseForHandoff(ctx, pause); err == nil {
		t.Fatal("partial incident state discarded")
	}
	record, _ = source.Record(ctx, "api")
	if record.Status != "running" {
		t.Fatal("failed preflight paused source")
	}
}

func TestHandoffRoundTripPreservesLocalHistoryAndFencesCloud(t *testing.T) {
	local, cloud, prepare, pause := transferFixture(t)
	source, err := local.PauseForHandoff(ctx, pause)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.ActivateHandoff(ctx, prepare.ID, source); err != nil {
		t.Fatal(err)
	}
	r, _ := cloud.Record(ctx, "api")
	input(t, cloud, r, 1, 200)
	cloud.Now = func() time.Time { return start.Add(6 * time.Second) }
	back, err := cloud.PreflightHandoff(ctx, "api")
	if err != nil || !back.Ready {
		t.Fatal(back, err)
	}
	review, err := local.Apply(ctx, ApplyRequest{Manifest: back.Manifest, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	reverseID := strings.Repeat("b", 64)
	prepared, err := local.PrepareHandoff(ctx, HandoffPrepare{ID: reverseID, ReturnOf: prepare.ID, Peer: back.Instance, Manifest: back.Manifest, Review: review.Review})
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := cloud.PauseForHandoff(ctx, HandoffPause{ID: reverseID, Peer: prepared.Instance, WatchID: "api", Revision: r.Plan.Revision, Generation: r.Generation, Destinations: back.Destinations})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.ActivateHandoff(ctx, reverseID, reverse); err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.Lifecycle(ctx, "api", LifecycleRequest{Action: "resume"}); err == nil {
		t.Fatal("cloud resumed after move back")
	}
	current, _ := local.Record(ctx, "api")
	if current.Status != "running" || !current.LastInputAt.IsZero() {
		t.Fatal("return did not start fresh")
	}
	var events int
	_ = local.Store.View(ctx, func(tx *store.Tx) error { history, err := tx.Events("api", 0, 100); events = len(history); return err })
	if events < 4 {
		t.Fatal("local lifecycle history lost", events)
	}
}
