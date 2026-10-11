package cloud

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeleteRevokesOnlySelectedWorkspaceAndRemovesWorkerState(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	tenant, err := s.Pool.Get(context.Background(), sessions[0].Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Vault.Put(context.Background(), tenant.Workspace.ID, "SECRET", "synthetic"); err != nil {
		t.Fatal(err)
	}
	w := cloudRequest(h, "DELETE", "/v1/workspace", `{"confirmWorkspace":"wrong"}`, sessions[0], true)
	if w.Code != 400 {
		t.Fatal("missing confirmation accepted")
	}
	w = cloudRequest(h, "GET", "/v1/cloud/export", "", sessions[0], false)
	if w.Code != 200 || strings.Contains(w.Body.String(), "synthetic") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = cloudRequest(h, "DELETE", "/v1/workspace", fmt.Sprintf(`{"confirmWorkspace":%q}`, tenant.Workspace.ID), sessions[0], true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := s.DB.Session(context.Background(), sessions[0].Token, time.Now()); err == nil {
		t.Fatal("deleted session works")
	}
	if _, err := s.Pool.Get(context.Background(), tenant.Workspace.ID); err == nil {
		t.Fatal("deleted worker resurrected")
	}
	if _, err := s.Vault.Get(context.Background(), tenant.Workspace.ID, "SECRET"); err == nil {
		t.Fatal("deleted secret retained")
	}
	if _, err := os.Stat(filepath.Join(s.Pool.root, "workspaces", tenant.Workspace.ID)); !os.IsNotExist(err) {
		t.Fatal("workspace files survived")
	}
	w = cloudRequest(h, "GET", "/v1/cloud/usage", "", sessions[1], false)
	if w.Code != 200 {
		t.Fatal("other workspace affected", w.Code)
	}
}
func TestDeletionFenceSurvivesInterruptedCleanup(t *testing.T) {
	s, _, sessions := testCloudServer(t)
	id := sessions[0].Workspace
	if err := s.DB.BeginWorkspaceDelete(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Session(context.Background(), sessions[0].Token, time.Now()); err == nil {
		t.Fatal("fenced session accepted")
	}
	if err := s.Pool.FinishPendingDeletes(context.Background()); err != nil {
		t.Fatal(err)
	}
	ids, err := s.DB.DeletingWorkspaces(context.Background())
	if err != nil || len(ids) != 0 {
		t.Fatal("deletion not finalized", err)
	}
}
