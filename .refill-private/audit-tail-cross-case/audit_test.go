package audittailcrosscase

import (
	"context"
	"testing"

	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/repository"
)

func TestAuditTailIsIsolatedPerCase(t *testing.T) {
	r, err := repository.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a := audit.New(r)
	ctx := context.Background()
	if _, err = a.Append(ctx, "case-a", "request-a", "create_case", "observer-a", "", "registered", map[string]string{"case": "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Append(ctx, "case-b", "request-b", "create_case", "observer-b", "", "registered", map[string]string{"case": "b"}); err != nil {
		t.Fatal(err)
	}

	events, err := a.Timeline(ctx, "case-b")
	if err != nil {
		t.Fatal(err)
	}
	if !a.Verify(events) {
		t.Fatalf("case-b audit chain inherited another case tail: %+v", events)
	}
}
