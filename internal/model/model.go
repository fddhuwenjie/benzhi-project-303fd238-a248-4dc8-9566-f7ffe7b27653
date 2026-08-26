package model

import "time"

type ObservationCase struct {
	CaseID            string    `json:"case_id"`
	BuoyID            string    `json:"buoy_id"`
	Region            string    `json:"region"`
	SpeciesScope      string    `json:"species_scope"`
	CreatedBy         string    `json:"created_by"`
	StartedAt         time.Time `json:"started_at"`
	EndedAt           time.Time `json:"ended_at"`
	Status            Status    `json:"status"`
	Revision          int       `json:"revision"`
	CreatedAt         time.Time `json:"created_at"`
	VerificationNotes []string  `json:"verification_notes,omitempty"`
}
