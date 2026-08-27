package reopen_path_cache_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benzhi/chao-sheng/internal/model"
	"github.com/benzhi/chao-sheng/internal/repository"
)

func TestReopenLoadsRestoredRepositoryFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	livePath := filepath.Join(dir, "live.json")
	backupPath := filepath.Join(dir, "backup.json")

	live, err := repository.Open(livePath)
	if err != nil {
		t.Fatalf("open live repository: %v", err)
	}
	oldCase := model.ObservationCase{
		CaseID: "case-before-restore", BuoyID: "BUOY-OLD", Region: "黄海",
		StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
		Status:    model.StatusRegistered, Revision: 1,
	}
	if err := live.SaveCase(ctx, oldCase, 0); err != nil {
		t.Fatalf("seed live repository: %v", err)
	}
	if err := live.Close(); err != nil {
		t.Fatalf("close live repository: %v", err)
	}

	backup, err := repository.Open(backupPath)
	if err != nil {
		t.Fatalf("open backup repository: %v", err)
	}
	restoredCase := model.ObservationCase{
		CaseID: "case-from-restored-backup", BuoyID: "BUOY-NEW", Region: "东海",
		StartedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 2, 1, 1, 0, 0, 0, time.UTC),
		Status:    model.StatusRegistered, Revision: 1,
	}
	if err := backup.SaveCase(ctx, restoredCase, 0); err != nil {
		t.Fatalf("seed backup repository: %v", err)
	}
	if err := backup.Close(); err != nil {
		t.Fatalf("close backup repository: %v", err)
	}
	restoredBytes, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if err := os.WriteFile(livePath, restoredBytes, 0600); err != nil {
		t.Fatalf("restore live path: %v", err)
	}

	reopened, err := repository.Open(livePath)
	if err != nil {
		t.Fatalf("reopen restored repository: %v", err)
	}
	if _, err := reopened.GetCase(ctx, restoredCase.CaseID); err != nil {
		t.Fatalf("reopened repository did not load restored case: %v", err)
	}
	if _, err := reopened.GetCase(ctx, oldCase.CaseID); err == nil {
		t.Fatalf("reopened repository retained pre-restore case")
	}
}
