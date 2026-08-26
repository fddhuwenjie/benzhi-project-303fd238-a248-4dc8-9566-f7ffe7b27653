package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/benzhi/chao-sheng/internal/repository"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Service struct {
	Repo       *repository.Repository
	mu         sync.Mutex
	lastDigest string
}

type ActionStat struct {
	Action     string    `json:"action"`
	Actor      string    `json:"actor"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Count      int       `json:"count"`
	FirstAt    time.Time `json:"first_at"`
	LastAt     time.Time `json:"last_at"`
}
type Summary struct {
	Stats             []ActionStat `json:"stats"`
	Integrity         string       `json:"integrity"`
	FirstInvalidIndex int          `json:"first_invalid_index"`
	PreviousDigest    string       `json:"previous_digest,omitempty"`
	InvalidDigest     string       `json:"invalid_digest,omitempty"`
	AlertLevel        string       `json:"alert_level"`
}

func New(r *repository.Repository) *Service { return &Service{Repo: r} }
func digest(v interface{}) string {
	b, _ := json.Marshal(v)
	var canonical interface{}
	if json.Unmarshal(b, &canonical) == nil {
		b, _ = json.Marshal(canonical)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) Append(ctx context.Context, caseID, requestID, action, actor, from, to string, payload interface{}) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev := Event{EventID: fmt.Sprintf("evt-%d", time.Now().UnixNano()), CaseID: caseID, RequestID: requestID, Action: action, Actor: actor, FromStatus: from, ToStatus: to, PayloadDigest: digest(payload), PreviousDigest: s.lastDigest, CreatedAt: time.Now().UTC(), Payload: payload}
	if err := s.Repo.AddAudit(ctx, ev); err != nil {
		return Event{}, err
	}
	s.lastDigest = ev.PayloadDigest
	return ev, nil
}
func (s *Service) Timeline(ctx context.Context, id string) ([]Event, error) {
	return s.Repo.ListAudit(ctx, id)
}
func (s *Service) TimelinePage(ctx context.Context, id, action, cursor string, limit int) ([]Event, string, bool, int, error) {
	return s.TimelineQuery(ctx, id, action, "", nil, nil, cursor, limit)
}
func (s *Service) TimelineQuery(ctx context.Context, id, action, actor string, from, to *time.Time, cursor string, limit int) ([]Event, string, bool, int, error) {
	return s.TimelineQueryRequest(ctx, id, action, actor, "", from, to, cursor, limit)
}
func (s *Service) TimelineQueryRequest(ctx context.Context, id, action, actor, requestID string, from, to *time.Time, cursor string, limit int) ([]Event, string, bool, int, error) {
	if _, err := s.Repo.GetCase(ctx, id); err != nil {
		return nil, "", false, 0, repository.ErrNotFound
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		return nil, "", false, 0, fmt.Errorf("页大小超限")
	}
	all, e := s.Repo.ListAudit(ctx, id)
	if e != nil {
		return nil, "", false, 0, e
	}
	start := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 0 || n > len(all) {
			return nil, "", false, 0, fmt.Errorf("游标格式错误")
		}
		start = n
	}
	filtered := make([]Event, 0)
	for _, ev := range all {
		if action != "" && ev.Action != action {
			continue
		}
		if actor != "" && ev.Actor != actor {
			continue
		}
		if requestID != "" && ev.RequestID != requestID {
			continue
		}
		if from != nil && ev.CreatedAt.Before(*from) {
			continue
		}
		if to != nil && ev.CreatedAt.After(*to) {
			continue
		}
		filtered = append(filtered, ev)
	}
	if start > len(filtered) {
		return nil, "", false, 0, fmt.Errorf("游标格式错误")
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page := filtered[start:end]
	next := ""
	if end < len(filtered) {
		next = strconv.Itoa(end)
	}
	return page, next, s.Verify(all), len(filtered), nil
}

type RequestTrace struct {
	RequestID         string   `json:"request_id"`
	Events            []Event  `json:"events"`
	Conflict          bool     `json:"conflict"`
	CaseIDs           []string `json:"case_ids"`
	PayloadDigests    []string `json:"payload_digests"`
	Integrity         bool     `json:"integrity"`
	FirstInvalidIndex int      `json:"first_invalid_index"`
}

func (s *Service) TraceRequest(ctx context.Context, requestID string) RequestTrace {
	events := s.Repo.FindAuditByRequest(ctx, requestID)
	cases := map[string]bool{}
	digests := map[string]bool{}
	integrity := true
	first := -1
	for _, ev := range events {
		cases[ev.CaseID] = true
		digests[ev.PayloadDigest] = true
	}
	for caseID := range cases {
		xs, _ := s.Repo.ListAudit(ctx, caseID)
		if ok, i := s.VerifyWithIndex(xs); !ok && integrity {
			integrity = false
			first = i
		}
	}
	caseIDs := []string{}
	for id := range cases {
		caseIDs = append(caseIDs, id)
	}
	sort.Strings(caseIDs)
	ds := []string{}
	for d := range digests {
		ds = append(ds, d)
	}
	sort.Strings(ds)
	return RequestTrace{RequestID: requestID, Events: events, Conflict: len(cases) > 1 && len(digests) > 1, CaseIDs: caseIDs, PayloadDigests: ds, Integrity: integrity, FirstInvalidIndex: first}
}
func (s *Service) Verify(events []Event) bool {
	prev := ""
	for _, e := range events {
		if e.PreviousDigest != prev || e.Payload != nil && digest(e.Payload) != e.PayloadDigest {
			return false
		}
		prev = e.PayloadDigest
	}
	return true
}
func (s *Service) VerifyWithIndex(events []Event) (bool, int) {
	prev := ""
	for i, e := range events {
		if e.PreviousDigest != prev || e.Payload != nil && digest(e.Payload) != e.PayloadDigest {
			return false, i
		}
		prev = e.PayloadDigest
	}
	return true, -1
}
func (s *Service) Summary(ctx context.Context, id string, events []Event) (Summary, error) {
	return s.SummaryFor(ctx, id, events, events)
}
func (s *Service) SummaryFor(ctx context.Context, id string, events, integrityEvents []Event) (Summary, error) {
	if _, e := s.Repo.GetCase(ctx, id); e != nil {
		return Summary{}, e
	}
	m := map[string]*ActionStat{}
	for _, e := range events {
		k := e.Action + "\x00" + e.Actor + "\x00" + e.FromStatus + "\x00" + e.ToStatus
		a := m[k]
		if a == nil {
			a = &ActionStat{Action: e.Action, Actor: e.Actor, FromStatus: e.FromStatus, ToStatus: e.ToStatus, FirstAt: e.CreatedAt, LastAt: e.CreatedAt}
			m[k] = a
		}
		a.Count++
		if e.CreatedAt.Before(a.FirstAt) {
			a.FirstAt = e.CreatedAt
		}
		if e.CreatedAt.After(a.LastAt) {
			a.LastAt = e.CreatedAt
		}
	}
	out := Summary{Stats: []ActionStat{}, Integrity: "valid", FirstInvalidIndex: -1, AlertLevel: "none"}
	for _, a := range m {
		out.Stats = append(out.Stats, *a)
	}
	sort.Slice(out.Stats, func(i, j int) bool {
		a, b := out.Stats[i], out.Stats[j]
		if a.Action != b.Action {
			return a.Action < b.Action
		}
		if a.Actor != b.Actor {
			return a.Actor < b.Actor
		}
		if a.FromStatus != b.FromStatus {
			return a.FromStatus < b.FromStatus
		}
		return a.ToStatus < b.ToStatus
	})
	ok, idx := s.VerifyWithIndex(integrityEvents)
	if !ok {
		out.Integrity = "invalid"
		out.FirstInvalidIndex = idx
		out.AlertLevel = "high"
		if idx > 0 {
			out.PreviousDigest = integrityEvents[idx-1].PayloadDigest
		}
		out.InvalidDigest = integrityEvents[idx].PreviousDigest
	}
	return out, nil
}
func ManifestDigest(v interface{}) string { return digest(v) }
