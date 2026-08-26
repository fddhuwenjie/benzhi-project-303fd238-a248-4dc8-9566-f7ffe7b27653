package model

import "time"

type AuditEvent struct {
	EventID        string      `json:"event_id"`
	CaseID         string      `json:"case_id"`
	RequestID      string      `json:"request_id"`
	Action         string      `json:"action"`
	Actor          string      `json:"actor"`
	FromStatus     string      `json:"from_status"`
	ToStatus       string      `json:"to_status"`
	PayloadDigest  string      `json:"payload_digest"`
	PreviousDigest string      `json:"previous_digest"`
	CreatedAt      time.Time   `json:"created_at"`
	Payload        interface{} `json:"payload,omitempty"`
}
