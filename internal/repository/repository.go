package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/benzhi/chao-sheng/internal/model"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")
var ErrRevision = errors.New("revision conflict")
var ErrIdempotency = errors.New("request replay conflict")

type Repository struct {
	mu        sync.RWMutex
	path      string
	cases     map[string]model.ObservationCase
	evidence  map[string][]model.CalibrationEvidence
	reviews   map[string]model.QualityReview
	bundles   map[string]Bundle
	audits    map[string][]model.AuditEvent
	idem      map[string]entry
	history   map[string][]model.QualityReview
	downloads map[string]map[string]string
	claims    map[string]ReviewClaim
}
type entry struct {
	Fingerprint string `json:"fingerprint"`
	Response    string `json:"response"`
}
type persistentState struct {
	Cases       map[string]model.ObservationCase       `json:"cases"`
	Evidence    map[string][]model.CalibrationEvidence `json:"evidence"`
	Reviews     map[string]model.QualityReview         `json:"reviews"`
	Bundles     map[string]Bundle                      `json:"bundles"`
	Audits      map[string][]model.AuditEvent          `json:"audits"`
	Idempotency map[string]entry                       `json:"idempotency"`
	History     map[string][]model.QualityReview       `json:"review_history"`
	Downloads   map[string]map[string]string           `json:"downloads"`
	Claims      map[string]ReviewClaim                 `json:"review_claims"`
}

func (r *Repository) MarshalJSON() ([]byte, error) {
	return json.Marshal(persistentState{r.cases, r.evidence, r.reviews, r.bundles, r.audits, r.idem, r.history, r.downloads, r.claims})
}
func (r *Repository) UnmarshalJSON(b []byte) error {
	var s persistentState
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s.Cases != nil {
		r.cases = s.Cases
	}
	if s.Evidence != nil {
		r.evidence = s.Evidence
	}
	if s.Reviews != nil {
		r.reviews = s.Reviews
	}
	if s.Bundles != nil {
		r.bundles = s.Bundles
	}
	if s.Audits != nil {
		r.audits = s.Audits
	}
	if s.Idempotency != nil {
		r.idem = s.Idempotency
	}
	if s.History != nil {
		r.history = s.History
	}
	if s.Downloads != nil {
		r.downloads = s.Downloads
	}
	if s.Claims != nil {
		r.claims = s.Claims
	}
	return nil
}

func Open(path string) (*Repository, error) {
	r := &Repository{path: path, cases: map[string]model.ObservationCase{}, evidence: map[string][]model.CalibrationEvidence{}, reviews: map[string]model.QualityReview{}, bundles: map[string]Bundle{}, audits: map[string][]model.AuditEvent{}, idem: map[string]entry{}, history: map[string][]model.QualityReview{}, downloads: map[string]map[string]string{}, claims: map[string]ReviewClaim{}}
	if path != "" && path != ":memory:" {
		if b, e := os.ReadFile(path); e == nil {
			_ = json.Unmarshal(b, r)
		}
	}
	if r.history == nil {
		r.history = map[string][]model.QualityReview{}
	}
	if r.downloads == nil {
		r.downloads = map[string]map[string]string{}
	}
	if r.claims == nil {
		r.claims = map[string]ReviewClaim{}
	}
	return r, nil
}
func (r *Repository) Close() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.path == "" || r.path == ":memory:" {
		return nil
	}
	b, _ := json.Marshal(r)
	return os.WriteFile(r.path, b, 0600)
}
func (r *Repository) DB() interface{} { return nil }
func (r *Repository) SaveCase(_ context.Context, c model.ObservationCase, expected int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if expected > 0 && r.cases[c.CaseID].Revision != expected {
		return ErrRevision
	}
	r.cases[c.CaseID] = c
	r.persist()
	return nil
}
func (r *Repository) SaveCaseNoOverlap(_ context.Context, c model.ObservationCase, expected int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if expected > 0 && r.cases[c.CaseID].Revision != expected {
		return ErrRevision
	}
	for _, x := range r.cases {
		if x.CaseID == c.CaseID {
			continue
		}
		if x.BuoyID == c.BuoyID && c.StartedAt.Before(x.EndedAt) && c.EndedAt.After(x.StartedAt) {
			return errors.New("浮标与既有个案时段重叠")
		}
	}
	r.cases[c.CaseID] = c
	r.persist()
	return nil
}

func (r *Repository) SaveMetadataNoOverlap(_ context.Context, c model.ObservationCase, expected int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.cases[c.CaseID]
	if !ok {
		return ErrNotFound
	}
	if old.Revision != expected {
		return ErrRevision
	}
	for _, x := range r.cases {
		if x.CaseID != c.CaseID && x.BuoyID == c.BuoyID && c.StartedAt.Before(x.EndedAt) && c.EndedAt.After(x.StartedAt) {
			return errors.New("浮标与既有个案时段重叠")
		}
	}
	r.cases[c.CaseID] = c
	r.persist()
	return nil
}
func (r *Repository) GetCase(_ context.Context, id string) (model.ObservationCase, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.cases[id]
	if !ok {
		return c, ErrNotFound
	}
	return c, nil
}
func (r *Repository) ListCases(_ context.Context) ([]model.ObservationCase, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o := make([]model.ObservationCase, 0, len(r.cases))
	for _, c := range r.cases {
		o = append(o, c)
	}
	return o, nil
}

func (r *Repository) QueryCases(_ context.Context, f CaseFilter) (CasePage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]model.ObservationCase, 0, len(r.cases))
	base := make([]model.ObservationCase, 0, len(r.cases))
	for _, c := range r.cases {
		if f.BuoyID != "" && c.BuoyID != f.BuoyID || f.Region != "" && c.Region != f.Region {
			continue
		}
		if f.From != nil && c.StartedAt.Before(*f.From) {
			continue
		}
		if f.To != nil && c.EndedAt.After(*f.To) {
			continue
		}
		base = append(base, c)
		if f.Status == "" || c.Status == f.Status {
			if f.NeedsAction {
				q, ok := r.reviews[c.CaseID]
				// 待办仅包含尚未完成复核的 screened 个案，以及明确退回重采的个案。
				// 已批准并进入 reviewing 的个案即使保留历史异常，也不应继续出现在队列中。
				needs := c.Status == model.StatusResample
				if c.Status == model.StatusScreened {
					needs = !ok || q.Decision == "pending" || len(q.Anomalies) > 0
				}
				if !needs {
					continue
				}
			}
			all = append(all, c)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if f.NeedsAction {
			ri, oki := r.reviews[all[i].CaseID]
			rj, okj := r.reviews[all[j].CaseID]
			rank := func(q model.QualityReview, ok bool) int {
				if !ok {
					return 0
				}
				switch q.RiskLevel {
				case "high":
					return 3
				case "medium":
					return 2
				default:
					return 1
				}
			}
			if rank(ri, oki) != rank(rj, okj) {
				return rank(ri, oki) > rank(rj, okj)
			}
			if len(ri.Anomalies) != len(rj.Anomalies) {
				return len(ri.Anomalies) > len(rj.Anomalies)
			}
		}
		if all[i].StartedAt.Equal(all[j].StartedAt) {
			return all[i].CaseID < all[j].CaseID
		}
		return all[i].StartedAt.Before(all[j].StartedAt)
	})
	start := 0
	if f.Cursor != "" {
		if _, err := fmt.Sscanf(f.Cursor, "%d", &start); err != nil || start < 0 || start > len(all) {
			return CasePage{}, errors.New("游标格式错误")
		}
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		return CasePage{}, errors.New("页大小超限")
	}
	end := start + limit
	if end > len(all) {
		end = len(all)
	}
	page := append([]model.ObservationCase(nil), all[start:end]...)
	next := ""
	if end < len(all) {
		next = fmt.Sprintf("%d", end)
	}
	counts := map[string]int{}
	for _, c := range base {
		counts[string(c.Status)]++
	}
	claims := map[string]ReviewClaim{}
	now := time.Now().UTC()
	for _, c := range page {
		if cl, ok := r.claims[c.CaseID]; ok && cl.LeaseUntil.After(now) {
			claims[c.CaseID] = cl
		}
	}
	return CasePage{Cases: page, NextCursor: next, Counts: counts, Total: len(all), Claims: claims}, nil
}
func (r *Repository) SaveEvidence(_ context.Context, e model.CalibrationEvidence) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evidence[e.CaseID] = append(r.evidence[e.CaseID], e)
	r.persist()
	return nil
}
func (r *Repository) SaveEvidenceBatch(_ context.Context, id string, evs []model.CalibrationEvidence) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evidence[id] = append(r.evidence[id], evs...)
	r.persist()
	return nil
}
func (r *Repository) SaveEvidenceAndCase(_ context.Context, id string, evs []model.CalibrationEvidence, c model.ObservationCase, expected int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if expected > 0 && r.cases[id].Revision != expected {
		return ErrRevision
	}
	r.evidence[id] = append(r.evidence[id], evs...)
	r.cases[id] = c
	r.persist()
	return nil
}
func (r *Repository) HasOverlap(_ context.Context, buoy string, start, end time.Time) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.cases {
		if c.BuoyID == buoy && start.Before(c.EndedAt) && end.After(c.StartedAt) {
			return true, nil
		}
	}
	return false, nil
}

func (r *Repository) FindOverlaps(_ context.Context, buoy string, start, end time.Time) ([]Overlap, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Overlap{}
	for _, c := range r.cases {
		if c.BuoyID == buoy && start.Before(c.EndedAt) && end.After(c.StartedAt) {
			out = append(out, Overlap{c.CaseID, c.Status, c.StartedAt, c.EndedAt, c.CreatedBy})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].CaseID < out[j].CaseID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}
func (r *Repository) QueryOverlaps(ctx context.Context, buoy string, start, end time.Time) ([]Overlap, error) {
	return r.FindOverlaps(ctx, buoy, start, end)
}
func (r *Repository) ListEvidence(_ context.Context, id string) ([]model.CalibrationEvidence, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]model.CalibrationEvidence(nil), r.evidence[id]...), nil
}

func (r *Repository) ListEvidenceSorted(_ context.Context, id string) ([]model.CalibrationEvidence, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]model.CalibrationEvidence(nil), r.evidence[id]...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SensorID != out[j].SensorID {
			return out[i].SensorID < out[j].SensorID
		}
		if !out[i].CalibratedAt.Equal(out[j].CalibratedAt) {
			return out[i].CalibratedAt.Before(out[j].CalibratedAt)
		}
		return out[i].EvidenceID < out[j].EvidenceID
	})
	return out, nil
}

func (r *Repository) SupersedeEvidence(_ context.Context, id, oldID string, replacement model.CalibrationEvidence, c model.ObservationCase, expected int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cases[id].Revision != expected {
		return ErrRevision
	}
	idx := -1
	for i := range r.evidence[id] {
		if r.evidence[id][i].EvidenceID == oldID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	if r.evidence[id][idx].Withdrawn {
		return errors.New("证据已撤回")
	}
	old := r.evidence[id][idx]
	old.Withdrawn = true
	old.WithdrawnReason = replacement.WithdrawnReason
	old.WithdrawnBy = replacement.WithdrawnBy
	old.WithdrawnAt = replacement.WithdrawnAt
	old.SupersededBy = replacement.EvidenceID
	replacement.WithdrawnReason, replacement.WithdrawnBy, replacement.WithdrawnAt = "", "", nil
	replacement.Supersedes = oldID
	r.evidence[id][idx] = old
	r.evidence[id] = append(r.evidence[id], replacement)
	if q, ok := r.reviews[id]; ok {
		now := time.Now().UTC()
		q.InvalidatedAt = &now
		r.reviews[id] = q
	}
	r.cases[id] = c
	r.persist()
	return nil
}

func (r *Repository) EvidenceReferences(_ context.Context, id, evidenceID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	refs := []string{}
	if q, ok := r.reviews[id]; ok {
		for _, rr := range q.RuleResults {
			if rr.EvidenceID == evidenceID {
				refs = append(refs, "review.rule_results")
				if q.SignedAt != nil {
					refs = append(refs, "signing.conclusion")
				}
			}
		}
		for _, x := range q.SupplementRefs {
			if x == evidenceID {
				refs = append(refs, "review.supplement_refs")
			}
		}
		for anomaly, xs := range q.EvidenceByAnomaly {
			for _, x := range xs {
				if x == evidenceID {
					refs = append(refs, "review.evidence_by_anomaly."+anomaly)
				}
			}
		}
	}
	if b, ok := r.bundles[id]; ok && strings.Contains(b.ManifestJSON, evidenceID) {
		refs = append(refs, "manifest.evidence")
	}
	sort.Strings(refs)
	return refs
}
func (r *Repository) SaveReview(_ context.Context, q model.QualityReview) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reviews[q.CaseID] = q
	r.persist()
	return nil
}

func (r *Repository) SaveReviewAndCase(_ context.Context, q model.QualityReview, c model.ObservationCase, expected int, clearClaim bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cases[c.CaseID].Revision != expected {
		return ErrRevision
	}
	r.reviews[q.CaseID] = q
	r.cases[c.CaseID] = c
	if clearClaim {
		delete(r.claims, c.CaseID)
	}
	r.persist()
	return nil
}

func (r *Repository) ClaimReview(_ context.Context, id, actor string, expected int, now time.Time, lease time.Duration) (ReviewClaim, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cases[id]
	if !ok {
		return ReviewClaim{}, false, ErrNotFound
	}
	if c.Revision != expected {
		return ReviewClaim{}, false, ErrRevision
	}
	old, exists := r.claims[id]
	if exists && old.LeaseUntil.After(now) && old.Actor != actor {
		return old, false, &ClaimConflictError{Claim: old}
	}
	actionReassign := exists && old.Actor != actor
	claimedAt := now
	if exists && old.Actor == actor && old.ClaimedRevision == expected {
		claimedAt = old.ClaimedAt
	}
	claim := ReviewClaim{CaseID: id, Actor: actor, ClaimedRevision: expected, ClaimedAt: claimedAt, LeaseUntil: now.Add(lease)}
	r.claims[id] = claim
	r.persist()
	return claim, actionReassign, nil
}

func (r *Repository) GetReviewClaim(_ context.Context, id string, now time.Time) (ReviewClaim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.claims[id]
	if !ok || !c.LeaseUntil.After(now) {
		return ReviewClaim{}, ErrNotFound
	}
	return c, nil
}
func (r *Repository) PeekReviewClaim(_ context.Context, id string) (ReviewClaim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.claims[id]
	if !ok {
		return c, ErrNotFound
	}
	return c, nil
}

func (r *Repository) ReleaseReviewClaim(_ context.Context, id, actor string) (ReviewClaim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.claims[id]
	if !ok {
		return c, ErrNotFound
	}
	if c.Actor != actor {
		return c, &ClaimConflictError{Claim: c}
	}
	delete(r.claims, id)
	r.persist()
	return c, nil
}
func (r *Repository) SaveReviewSnapshot(_ context.Context, q model.QualityReview) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history[q.CaseID] = append(r.history[q.CaseID], q)
	r.persist()
	return nil
}
func (r *Repository) SaveScreenSnapshot(ctx context.Context, q model.QualityReview) error {
	return r.SaveReviewSnapshot(ctx, q)
}
func (r *Repository) ListReviewHistory(_ context.Context, id string) ([]model.QualityReview, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]model.QualityReview(nil), r.history[id]...)
	sort.Slice(out, func(i, j int) bool { return out[i].RunAt.Before(out[j].RunAt) })
	return out, nil
}
func (r *Repository) GetReview(_ context.Context, id string) (model.QualityReview, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	q, ok := r.reviews[id]
	if !ok {
		return q, ErrNotFound
	}
	return q, nil
}
func (r *Repository) SaveBundle(_ context.Context, b Bundle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bundles[b.CaseID] = b
	r.persist()
	return nil
}
func (r *Repository) SaveBundleAndCase(_ context.Context, b Bundle, c model.ObservationCase, expected int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if expected > 0 && r.cases[c.CaseID].Revision != expected {
		return ErrRevision
	}
	r.bundles[b.CaseID] = b
	r.cases[c.CaseID] = c
	r.persist()
	return nil
}
func (r *Repository) GetBundle(_ context.Context, id string) (Bundle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.bundles[id]
	if !ok {
		return b, ErrNotFound
	}
	return b, nil
}
func (r *Repository) IncrementDownload(_ context.Context, id string) (Bundle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.bundles[id]
	if !ok {
		return b, ErrNotFound
	}
	b.DownloadCount++
	r.bundles[id] = b
	r.persist()
	return b, nil
}
func (r *Repository) RecordDownload(_ context.Context, id, requestID, fingerprint, actor string) (Bundle, bool, error) {
	r.mu.Lock()
	b, ok := r.bundles[id]
	if !ok {
		r.mu.Unlock()
		return b, false, ErrNotFound
	}
	if r.downloads[id] == nil {
		r.downloads[id] = map[string]string{}
	}
	if requestID != "" {
		if old, ex := r.downloads[id][requestID]; ex {
			if old != fingerprint {
				r.mu.Unlock()
				return b, false, ErrIdempotency
			}
			r.mu.Unlock()
			return b, false, nil
		}
		r.downloads[id][requestID] = fingerprint
	}
	actors := make(map[string]int, len(b.DownloadActors)+1)
	for name, count := range b.DownloadActors {
		actors[name] = count
	}
	r.mu.Unlock()

	b.DownloadCount++
	now := time.Now().UTC()
	if b.LastDownloadAt == nil {
		b.LastDownloadAt = &now
	} else {
		*b.LastDownloadAt = now
	}
	b.DownloadActors = actors
	b.DownloadActors[actor]++

	r.mu.Lock()
	r.bundles[id] = b
	r.persist()
	r.mu.Unlock()
	return b, true, nil
}
func (r *Repository) AddAudit(_ context.Context, e model.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audits[e.CaseID] = append(r.audits[e.CaseID], e)
	r.persist()
	return nil
}
func (r *Repository) ListAudit(_ context.Context, id string) ([]model.AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]model.AuditEvent(nil), r.audits[id]...), nil
}

func (r *Repository) FindAuditByRequest(_ context.Context, requestID string) []model.AuditEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []model.AuditEvent{}
	for _, events := range r.audits {
		for _, ev := range events {
			if ev.RequestID == requestID {
				out = append(out, ev)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CaseID < out[j].CaseID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}
func (r *Repository) Idempotent(_ context.Context, id, fp string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	x, ok := r.idem[id]
	if !ok {
		return "", nil
	}
	if x.Fingerprint != fp {
		return "", ErrIdempotency
	}
	return x.Response, nil
}
func (r *Repository) PutIdempotent(_ context.Context, id, fp, resp string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idem[id] = entry{fp, resp}
	r.persist()
	return nil
}
func (r *Repository) Health(_ context.Context) error { return nil }
func (r *Repository) persist() {
	if r.path == "" || r.path == ":memory:" {
		return
	}
	b, _ := json.Marshal(r)
	_ = os.WriteFile(r.path, b, 0600)
}
func Fingerprint(v interface{}) string { b, _ := json.Marshal(v); return fmt.Sprintf("%x", b) }
