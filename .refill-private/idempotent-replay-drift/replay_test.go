package idempotent_replay_drift

import (
	"context"
	"testing"

	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/observation"
	"github.com/benzhi/chao-sheng/internal/repository"
)

func TestCreateReplayReturnsOriginalSnapshot(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	service := observation.New(repo, audit.New(repo))
	input := observation.CreateInput{BuoyID: "B-REPLAY", Region: "黄海", SpeciesScope: "鲸类", StartedAt: "2026-01-01T00:00:00Z", EndedAt: "2026-01-01T01:00:00Z", CreatedBy: "observer"}
	created, err := service.Create(ctx, input, "same-create-request")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.AddVerificationNote(ctx, created.CaseID, "后续状态变化", "observer", "later-note-request", created.Revision)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Create(ctx, input, "same-create-request")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Revision != created.Revision || len(replayed.VerificationNotes) != len(created.VerificationNotes) {
		t.Fatalf("idempotent replay drifted to the resource's later state: first=%#v replay=%#v", created, replayed)
	}
}
