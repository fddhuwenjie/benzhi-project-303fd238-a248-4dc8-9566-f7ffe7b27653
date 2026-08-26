package model

import "time"

type CalibrationEvidence struct {
	EvidenceID        string     `json:"evidence_id"`
	CaseID            string     `json:"case_id"`
	SensorID          string     `json:"sensor_id"`
	CalibrationRef    string     `json:"calibration_ref"`
	AudioDigest       string     `json:"audio_digest"`
	Operator          string     `json:"operator"`
	CalibratedAt      time.Time  `json:"calibrated_at"`
	SubmittedAt       time.Time  `json:"submitted_at"`
	SamplingRate      int        `json:"sampling_rate"`
	SubmittedRevision int        `json:"submitted_revision"`
	Withdrawn         bool       `json:"withdrawn"`
	WithdrawnReason   string     `json:"withdrawn_reason,omitempty"`
	WithdrawnBy       string     `json:"withdrawn_by,omitempty"`
	WithdrawnAt       *time.Time `json:"withdrawn_at,omitempty"`
	SupersededBy      string     `json:"superseded_by,omitempty"`
	Supersedes        string     `json:"supersedes,omitempty"`
}

type RuleResult struct {
	Name        string `json:"name"`
	EvidenceID  string `json:"evidence_id,omitempty"`
	SensorID    string `json:"sensor_id,omitempty"`
	Passed      bool   `json:"passed"`
	Score       int    `json:"score"`
	Explanation string `json:"explanation"`
}
