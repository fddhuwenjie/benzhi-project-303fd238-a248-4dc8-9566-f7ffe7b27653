package observation

import (
	"github.com/benzhi/chao-sheng/internal/repository"
	"time"
)

type EvidenceReport struct {
	CaseID           string                  `json:"case_id"`
	Revision         int                     `json:"revision"`
	LatestRevision   int                     `json:"latest_submission_revision,omitempty"`
	Sensors          []SensorEvidenceSummary `json:"sensors"`
	Differences      []EvidenceDifference    `json:"differences"`
	Score            int                     `json:"score"`
	IntegrityScore   int                     `json:"integrity_score,omitempty"`
	Gaps             []EvidenceGap           `json:"gaps,omitempty"`
	Blocking         bool                    `json:"blocking"`
	PreviousRevision int                     `json:"previous_revision,omitempty"`
	Coverage         []SensorCoverage        `json:"coverage,omitempty"`
	CoverageScore    int                     `json:"coverage_score,omitempty"`
}

type SensorCoverage struct {
	SensorID        string    `json:"sensor_id"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds int64     `json:"duration_seconds"`
	Ratio           float64   `json:"ratio"`
	Warnings        []string  `json:"warnings,omitempty"`
}

type EvidenceGap struct {
	Code       string `json:"code"`
	EvidenceID string `json:"evidence_id,omitempty"`
	SensorID   string `json:"sensor_id,omitempty"`
	Blocking   bool   `json:"blocking"`
	Detail     string `json:"detail"`
}

type PrecheckResult struct {
	Valid     bool                 `json:"valid"`
	BuoyID    string               `json:"buoy_id"`
	Region    string               `json:"region"`
	StartedAt time.Time            `json:"started_at"`
	EndedAt   time.Time            `json:"ended_at"`
	Errors    map[string]string    `json:"errors,omitempty"`
	Conflicts []repository.Overlap `json:"conflicts"`
}

type SensorEvidenceSummary struct {
	SensorID      string    `json:"sensor_id"`
	Count         int       `json:"count"`
	Earliest      time.Time `json:"earliest_calibration"`
	Latest        time.Time `json:"latest_calibration"`
	SamplingRates []int     `json:"sampling_rates"`
}

type EvidenceDifference struct {
	Code       string `json:"code"`
	EvidenceID string `json:"evidence_id,omitempty"`
	SensorID   string `json:"sensor_id,omitempty"`
	Detail     string `json:"detail"`
}
