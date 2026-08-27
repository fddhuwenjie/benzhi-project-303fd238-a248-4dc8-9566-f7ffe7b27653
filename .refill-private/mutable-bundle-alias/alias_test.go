package mutable_bundle_alias

import (
	"context"
	"testing"

	"github.com/benzhi/chao-sheng/internal/repository"
)

func TestReturnedBundleCannotMutateRepository(t *testing.T) {
	repo, err := repository.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	wantDigest := "original-digest"
	wantActorCount := 1
	err = repo.SaveBundle(context.Background(), repository.Bundle{
		CaseID: "case-alias", PartDigests: map[string]string{"evidence": wantDigest},
		DownloadActors: map[string]int{"researcher": wantActorCount}, PartOrder: []string{"metadata", "evidence", "quality"},
	})
	if err != nil {
		t.Fatal(err)
	}
	returned, err := repo.GetBundle(context.Background(), "case-alias")
	if err != nil {
		t.Fatal(err)
	}
	returned.PartDigests["evidence"] = "tampered"
	returned.DownloadActors["researcher"] = 99
	returned.PartOrder[0] = "tampered"

	stored, err := repo.GetBundle(context.Background(), "case-alias")
	if err != nil {
		t.Fatal(err)
	}
	if stored.PartDigests["evidence"] != wantDigest || stored.DownloadActors["researcher"] != wantActorCount || stored.PartOrder[0] != "metadata" {
		t.Fatalf("mutating a returned bundle polluted repository state: %#v", stored)
	}
}
