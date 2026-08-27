package claim_replay_duplicates

import (
	"context"
	"testing"
	"time"

	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/model"
	"github.com/benzhi/chao-sheng/internal/quality"
	"github.com/benzhi/chao-sheng/internal/repository"
)

func TestClaimReplayDoesNotRenewOrDuplicateAudit(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	c := model.ObservationCase{CaseID: "case-claim", BuoyID: "B-CLAIM", Region: "黄海", StartedAt: now.Add(-time.Hour), EndedAt: now, Status: model.StatusScreened, Revision: 5}
	if err = repo.SaveCase(ctx, c, 0); err != nil {
		t.Fatal(err)
	}
	service := quality.New(repo, audit.New(repo))
	first, err := service.ClaimReview(ctx, c.CaseID, "qc-a", "same-claim-request", c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ClaimReview(ctx, c.CaseID, "qc-a", "same-claim-request", c.Revision)
	if err != nil {
		t.Fatal(err)
	}
	events, err := repo.ListAudit(ctx, c.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !second.LeaseUntil.Equal(first.LeaseUntil) {
		t.Fatalf("claim replay renewed state or duplicated audit: first=%s second=%s events=%d", first.LeaseUntil, second.LeaseUntil, len(events))
	}
}
