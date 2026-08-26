package observation

import (
	"github.com/benzhi/chao-sheng/internal/model"
)

type Status = model.Status

const (
	StatusRegistered = model.StatusRegistered
	StatusEvidence   = model.StatusEvidence
	StatusScreened   = model.StatusScreened
	StatusReviewing  = model.StatusReviewing
	StatusSigned     = model.StatusSigned
	StatusFrozen     = model.StatusFrozen
	StatusResample   = model.StatusResample
)

type ObservationCase = model.ObservationCase
type CalibrationEvidence = model.CalibrationEvidence
