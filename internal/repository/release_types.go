package repository

import (
	"fmt"
	"time"
)

type Bundle struct {
	BundleID              string            `json:"bundle_id"`
	CaseID                string            `json:"case_id"`
	ManifestJSON          string            `json:"manifest_json"`
	ContentDigest         string            `json:"content_digest"`
	FrozenBy              string            `json:"frozen_by"`
	FrozenRevision        int               `json:"frozen_revision"`
	FrozenAt              time.Time         `json:"frozen_at"`
	DownloadCount         int               `json:"download_count"`
	ManifestSchemaVersion string            `json:"manifest_schema_version,omitempty"`
	VersionLabel          string            `json:"version_label,omitempty"`
	PreviewDigest         string            `json:"preview_digest,omitempty"`
	LastDownloadAt        *time.Time        `json:"last_download_at,omitempty"`
	DownloadActors        map[string]int    `json:"download_actors,omitempty"`
	PartDigests           map[string]string `json:"part_digests,omitempty"`
	PartOrder             []string          `json:"part_order,omitempty"`
	RootDigest            string            `json:"root_digest,omitempty"`
}

type ReviewClaim struct {
	CaseID          string    `json:"case_id"`
	Actor           string    `json:"actor"`
	ClaimedRevision int       `json:"claimed_revision"`
	ClaimedAt       time.Time `json:"claimed_at"`
	LeaseUntil      time.Time `json:"lease_until"`
}

type ClaimConflictError struct{ Claim ReviewClaim }

func (e *ClaimConflictError) Error() string {
	return fmt.Sprintf("复核任务由 %s 占用，租约截止 %s", e.Claim.Actor, e.Claim.LeaseUntil.Format(time.RFC3339))
}

// cloneBundle returns a deep copy of b whose slice, map, and pointer fields do
// not share backing storage with the original. Callers must never observe or
// mutate the repository's internal mutable objects through values handed back
// to them, and the repository must never retain references to caller-owned
// mutable objects. Keeping the frozen release bundle hermetic on both sides of
// the boundary prevents accidental tampering of digests, counts, or part order
// and avoids data races under concurrent read/write access.
func cloneBundle(b Bundle) Bundle {
	out := b
	if b.PartDigests != nil {
		c := make(map[string]string, len(b.PartDigests))
		for k, v := range b.PartDigests {
			c[k] = v
		}
		out.PartDigests = c
	}
	if b.DownloadActors != nil {
		c := make(map[string]int, len(b.DownloadActors))
		for k, v := range b.DownloadActors {
			c[k] = v
		}
		out.DownloadActors = c
	}
	if b.PartOrder != nil {
		out.PartOrder = append([]string(nil), b.PartOrder...)
	}
	if b.LastDownloadAt != nil {
		ts := *b.LastDownloadAt
		out.LastDownloadAt = &ts
	}
	return out
}
