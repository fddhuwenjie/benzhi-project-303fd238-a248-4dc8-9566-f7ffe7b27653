package observation

type CreateInput struct {
	BuoyID       string `json:"buoy_id"`
	Region       string `json:"region"`
	SpeciesScope string `json:"species_scope"`
	StartedAt    string `json:"started_at"`
	EndedAt      string `json:"ended_at"`
	CreatedBy    string `json:"created_by"`
}

type EvidenceInput struct {
	SensorID       string `json:"sensor_id"`
	CalibrationRef string `json:"calibration_ref"`
	AudioDigest    string `json:"audio_digest"`
	Operator       string `json:"operator"`
	CalibratedAt   string `json:"calibrated_at"`
	SamplingRate   int    `json:"sampling_rate"`
}

type EvidenceBatchInput struct {
	Items []EvidenceInput `json:"items"`
}

type MetadataPatchInput struct {
	BuoyID       *string `json:"buoy_id,omitempty"`
	Region       *string `json:"region,omitempty"`
	SpeciesScope *string `json:"species_scope,omitempty"`
	StartedAt    *string `json:"started_at,omitempty"`
	EndedAt      *string `json:"ended_at,omitempty"`
}

type SupersedeInput struct {
	EvidenceID          string        `json:"evidence_id"`
	Replacement         EvidenceInput `json:"replacement"`
	ReplacementEvidence EvidenceInput `json:"replacement_evidence,omitempty"`
	Evidence            EvidenceInput `json:"evidence,omitempty"`
	Reason              string        `json:"reason"`
	ExpectedRevision    int           `json:"expected_revision,omitempty"`
}
