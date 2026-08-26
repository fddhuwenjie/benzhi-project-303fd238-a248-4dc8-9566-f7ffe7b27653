package quality

import "time"

type ReviewInput struct {
	Anomalies         []string            `json:"anomalies"`
	SupplementRefs    []string            `json:"supplement_refs"`
	Reviewer          string              `json:"reviewer"`
	Decision          string              `json:"decision"`
	ResampleReason    string              `json:"resample_reason"`
	Disposition       string              `json:"disposition"`
	EvidenceByAnomaly map[string][]string `json:"evidence_by_anomaly,omitempty"`
	ExpectedRevision  int                 `json:"expected_revision,omitempty"`
}

type SignInput struct {
	Grade            string `json:"grade"`
	Reviewer         string `json:"reviewer"`
	ExpectedRevision int    `json:"expected_revision,omitempty"`
	DeclarationText  string `json:"declaration_text"`
	Token            string `json:"token"`
	DeclarationToken string `json:"declaration_token,omitempty"`
}

type RuleChange struct {
	Rule          string `json:"rule"`
	SensorID      string `json:"sensor_id,omitempty"`
	PreviousScore int    `json:"previous_score"`
	CurrentScore  int    `json:"current_score"`
	Change        string `json:"change"`
}

type ScreenComparison struct {
	PreviousRunAt   *time.Time   `json:"previous_run_at,omitempty"`
	PreviousScore   int          `json:"previous_score,omitempty"`
	CurrentScore    int          `json:"current_score"`
	RiskBefore      string       `json:"risk_before,omitempty"`
	RiskAfter       string       `json:"risk_after"`
	Changes         []RuleChange `json:"changes"`
	ProfileChanged  bool         `json:"profile_changed,omitempty"`
	PreviousProfile string       `json:"previous_profile,omitempty"`
	CurrentProfile  string       `json:"current_profile,omitempty"`
}

type SignReadiness struct {
	Ready            bool     `json:"ready"`
	Revision         int      `json:"revision"`
	Failures         []string `json:"failures"`
	RiskLevel        string   `json:"risk_level"`
	GradeOptions     []string `json:"grade_options"`
	DeclarationToken string   `json:"declaration_token"`
}

type RuleProfile struct {
	ProfileID           string         `json:"profile_id"`
	Version             int            `json:"version"`
	Active              bool           `json:"active"`
	SamplingRateMinimum int            `json:"sampling_rate_minimum"`
	Weights             map[string]int `json:"weights"`
}
