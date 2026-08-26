package repository

import (
	"github.com/benzhi/chao-sheng/internal/model"
	"time"
)

type CaseFilter struct {
	BuoyID, Region string
	Status         model.Status
	From, To       *time.Time
	Limit          int
	Cursor         string
	NeedsAction    bool
}

type CasePage struct {
	Cases      []model.ObservationCase
	NextCursor string
	Counts     map[string]int
	Total      int
	Claims     map[string]ReviewClaim
}

type Overlap struct {
	CaseID     string       `json:"case_id"`
	Status     model.Status `json:"status"`
	StartedAt  time.Time    `json:"started_at"`
	EndedAt    time.Time    `json:"ended_at"`
	OccupiedBy string       `json:"occupied_by"`
}
