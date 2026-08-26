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
