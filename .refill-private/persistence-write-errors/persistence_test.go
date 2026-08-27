package persistence_write_errors

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/benzhi/chao-sheng/internal/model"
	"github.com/benzhi/chao-sheng/internal/repository"
)

func TestPersistenceFailureIsReturned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-parent", "state.json")
	repo, err := repository.Open(path)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	err = repo.SaveCase(context.Background(), model.ObservationCase{
		CaseID: "case-persist", BuoyID: "B-PERSIST", Region: "黄海",
		StartedAt: now, EndedAt: now.Add(time.Hour), Status: model.StatusRegistered, Revision: 1,
	}, 0)
	if err == nil {
		t.Fatal("repository reported success after persistence write failed")
	}
}
