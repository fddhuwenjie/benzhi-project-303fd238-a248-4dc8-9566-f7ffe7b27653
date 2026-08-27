package canceled_context_write

import (
	"context"
	"errors"
	"testing"

	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/observation"
	"github.com/benzhi/chao-sheng/internal/repository"
)

func TestCanceledCreateDoesNotCommit(t *testing.T) {
	repo, err := repository.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := observation.New(repo, audit.New(repo))
	_, err = service.Create(ctx, observation.CreateInput{
		BuoyID: "B-CANCELED", Region: "黄海", SpeciesScope: "鲸类",
		StartedAt: "2026-01-01T00:00:00Z", EndedAt: "2026-01-01T01:00:00Z", CreatedBy: "observer",
	}, "canceled-create")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("canceled request committed instead of returning context.Canceled: %v", err)
	}
	if err := repo.Health(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled health check ignored context cancellation: %v", err)
	}
	if _, err := repo.ListCases(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled read ignored context cancellation: %v", err)
	}
	cases, err := repo.ListCases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 0 {
		t.Fatalf("canceled request left %d committed cases", len(cases))
	}
}
