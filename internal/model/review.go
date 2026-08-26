package model

import "time"

type QualityReview struct {
	ReviewID               string                 `json:"review_id"`
	CaseID                 string                 `json:"case_id"`
	RuleResults            []RuleResult           `json:"rule_results"`
	Anomalies              []string               `json:"anomalies"`
	SupplementRefs         []string               `json:"supplement_refs"`
	EvidenceByAnomaly      map[string][]string    `json:"evidence_by_anomaly,omitempty"`
	Grade                  string                 `json:"grade"`
	Reviewer               string                 `json:"reviewer"`
	Decision               string                 `json:"decision"`
	SubmittedBy            string                 `json:"submitted_by,omitempty"`
	ScreenedBy             string                 `json:"screened_by,omitempty"`
	ResampleReason         string                 `json:"resample_reason,omitempty"`
	Disposition            string                 `json:"disposition,omitempty"`
	TotalScore             int                    `json:"total_score"`
	RiskLevel              string                 `json:"risk_level"`
	SignedAt               *time.Time             `json:"signed_at,omitempty"`
	Aggregates             []RuleAggregate        `json:"aggregates,omitempty"`
	RunAt                  time.Time              `json:"run_at,omitempty"`
	PreviousReviewID       string                 `json:"previous_review_id,omitempty"`
	Comparison             interface{}            `json:"comparison,omitempty"`
	CaseRevision           int                    `json:"case_revision,omitempty"`
	RuleProfileID          string                 `json:"profile_id,omitempty"`
	RuleProfileVersion     int                    `json:"profile_version,omitempty"`
	RuleParameters         map[string]interface{} `json:"rule_parameters,omitempty"`
	RuleFingerprint        string                 `json:"rule_fingerprint,omitempty"`
	InvalidatedAt          *time.Time             `json:"invalidated_at,omitempty"`
	DeclarationDigest      string                 `json:"declaration_digest,omitempty"`
	DeclarationConfirmedAt *time.Time             `json:"declaration_confirmed_at,omitempty"`
}

type RuleAggregate struct {
	SensorID     string  `json:"sensor_id"`
	RuleName     string  `json:"rule_name"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	AverageScore float64 `json:"average_score"`
}
