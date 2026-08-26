package model

type Status string

const (
	StatusRegistered Status = "registered"
	StatusEvidence   Status = "evidence"
	StatusScreened   Status = "screened"
	StatusReviewing  Status = "reviewing"
	StatusSigned     Status = "signed"
	StatusFrozen     Status = "frozen"
	StatusResample   Status = "resample"
)
