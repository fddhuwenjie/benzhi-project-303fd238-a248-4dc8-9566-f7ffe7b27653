package web

import (
	"errors"
	"fmt"
	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/observation"
	"github.com/benzhi/chao-sheng/internal/quality"
	"github.com/benzhi/chao-sheng/internal/repository"
	"html/template"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	Obs           *observation.Service
	Quality       *quality.Service
	Audit         *audit.Service
	Repo          *repository.Repository
	PublishActors map[string]bool
}

func New(o *observation.Service, q *quality.Service, a *audit.Service, r *repository.Repository) *Server {
	return &Server{Obs: o, Quality: q, Audit: a, Repo: r, PublishActors: map[string]bool{"researcher": true, "observer": true, "qc": true}}
}
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/", s.index)
	m.HandleFunc("/health", s.health)
	m.HandleFunc("/api/v1/cases", s.cases)
	m.HandleFunc("/api/v1/cases/", s.caseRoutes)
	m.HandleFunc("/api/v1/rule-profiles", s.ruleProfiles)
	m.HandleFunc("/api/v1/audit/trace", s.auditTrace)
	return m
}
func (s *Server) ruleProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	write(w, map[string]interface{}{"rule_profiles": s.Quality.RuleProfiles()}, 200)
}
func (s *Server) auditTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("request_id"))
	if id == "" {
		write(w, map[string]string{"error": "request_id 不能为空"}, 400)
		return
	}
	write(w, s.Audit.TraceRequest(r.Context(), id), 200)
}
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	template.Must(template.New("index").Parse(page)).Execute(w, nil)
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if e := s.Repo.Health(r.Context()); e != nil {
		write(w, map[string]string{"error": e.Error()}, 500)
		return
	}
	write(w, map[string]string{"status": "ok"}, 200)
}
func (s *Server) cases(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		q := r.URL.Query()
		if q.Get("precheck") == "1" || strings.EqualFold(q.Get("precheck"), "true") || (q.Get("started_at") != "" && q.Get("ended_at") != "") {
			start, end := q.Get("started_at"), q.Get("ended_at")
			if start == "" {
				start = q.Get("start")
			}
			if end == "" {
				end = q.Get("end")
			}
			if start == "" {
				start = q.Get("from")
			}
			if end == "" {
				end = q.Get("to")
			}
			res := s.Obs.Precheck(r.Context(), observation.CreateInput{BuoyID: q.Get("buoy_id"), Region: q.Get("region"), StartedAt: start, EndedAt: end})
			write(w, res, 200)
			return
		}
		f := repository.CaseFilter{BuoyID: strings.ToUpper(strings.TrimSpace(q.Get("buoy_id"))), Region: strings.TrimSpace(q.Get("region")), Status: observation.Status(q.Get("status")), Limit: 20, Cursor: q.Get("cursor")}
		f.NeedsAction = q.Get("needs_action") == "1" || strings.EqualFold(q.Get("needs_action"), "true")
		if x := q.Get("limit"); x != "" {
			fmt.Sscanf(x, "%d", &f.Limit)
		}
		if x := q.Get("from"); x != "" {
			if t, e := time.Parse(time.RFC3339, x); e == nil {
				f.From = &t
			}
		}
		if x := q.Get("to"); x != "" {
			if t, e := time.Parse(time.RFC3339, x); e == nil {
				f.To = &t
			}
		}
		pg, e := s.Obs.Query(r.Context(), f)
		if e != nil {
			write(w, map[string]string{"error": e.Error()}, 400)
			return
		}
		write(w, map[string]interface{}{"cases": pg.Cases, "items": pg.Cases, "next_cursor": pg.NextCursor, "counts": pg.Counts, "total": pg.Total, "review_claims": pg.Claims, "request_trace_template": "/api/v1/audit/trace?request_id={request_id}"}, 200)
	case "POST":
		var in observation.CreateInput
		if body(r, &in) != nil {
			write(w, map[string]string{"error": "请求格式错误"}, 400)
			return
		}
		c, e := s.Obs.Create(r.Context(), in, r.Header.Get("X-Request-ID"))
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrIdempotency) {
				code = 409
			}
			if strings.Contains(e.Error(), "重叠") {
				code = 409
				pre := s.Obs.Precheck(r.Context(), in)
				write(w, map[string]interface{}{"error": e.Error(), "conflicts": pre.Conflicts, "valid": false}, code)
				return
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		write(w, c, 201)
	default:
		w.WriteHeader(405)
	}
}
func (s *Server) caseRoutes(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/cases/")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == "GET" {
		c, e := s.Obs.Get(r.Context(), id)
		if e != nil {
			write(w, map[string]string{"error": "不存在"}, 404)
			return
		}
		q, _ := s.Quality.GetReview(r.Context(), id)
		ev, _ := s.Obs.Evidence(r.Context(), id)
		b, _ := s.Quality.GetBundle(r.Context(), id)
		write(w, map[string]interface{}{"case": c, "evidence": ev, "review": q, "bundle": b}, 200)
		return
	}
	if len(parts) == 1 && r.Method == "PATCH" {
		if strings.TrimSpace(r.Header.Get("X-Request-ID")) == "" {
			write(w, map[string]string{"error": "缺少 X-Request-ID"}, 400)
			return
		}
		var in observation.MetadataPatchInput
		if body(r, &in) != nil {
			write(w, map[string]string{"error": "请求格式错误"}, 400)
			return
		}
		exp := 0
		fmt.Sscanf(r.Header.Get("X-Expected-Revision"), "%d", &exp)
		c, e := s.Obs.UpdateMetadata(r.Context(), id, in, exp, r.Header.Get("X-Request-ID"), r.Header.Get("X-Actor"))
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrRevision) || errors.Is(e, repository.ErrIdempotency) || strings.Contains(e.Error(), "重叠") || strings.Contains(e.Error(), "状态允许") {
				code = 409
			}
			if errors.Is(e, repository.ErrNotFound) {
				code = 404
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		write(w, c, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "evidence" && r.Method == "POST" {
		var in observation.EvidenceInput
		if body(r, &in) != nil {
			write(w, map[string]string{"error": "请求格式错误"}, 400)
			return
		}
		var x struct {
			ExpectedRevision int `json:"expected_revision"`
		}
		_ = x
		exp := r.Header.Get("X-Expected-Revision")
		fmt.Sscanf(exp, "%d", &x.ExpectedRevision)
		c, e := s.Obs.AddEvidence(r.Context(), id, in, x.ExpectedRevision, r.Header.Get("X-Request-ID"))
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrRevision) {
				code = 409
			}
			if errors.Is(e, repository.ErrNotFound) {
				code = 404
			}
			if errors.Is(e, repository.ErrIdempotency) {
				code = 409
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		write(w, c, 200)
		return
	}
	if len(parts) == 3 && parts[1] == "evidence" && parts[2] == "batch" && r.Method == "POST" {
		var in observation.EvidenceBatchInput
		if body(r, &in) != nil {
			write(w, map[string]string{"error": "请求格式错误"}, 400)
			return
		}
		exp := 0
		fmt.Sscanf(r.Header.Get("X-Expected-Revision"), "%d", &exp)
		c, e := s.Obs.AddEvidenceBatch(r.Context(), id, in, exp, r.Header.Get("X-Request-ID"))
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrRevision) {
				code = 409
			}
			if errors.Is(e, repository.ErrNotFound) {
				code = 404
			}
			if errors.Is(e, repository.ErrIdempotency) {
				code = 409
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		write(w, c, 200)
		return
	}
	if len(parts) == 3 && parts[1] == "evidence" && parts[2] == "supersede" && r.Method == "POST" {
		if strings.TrimSpace(r.Header.Get("X-Request-ID")) == "" {
			write(w, map[string]string{"error": "缺少 X-Request-ID"}, 400)
			return
		}
		var in observation.SupersedeInput
		if body(r, &in) != nil {
			write(w, map[string]string{"error": "请求格式错误"}, 400)
			return
		}
		exp := in.ExpectedRevision
		if exp == 0 {
			fmt.Sscanf(r.Header.Get("X-Expected-Revision"), "%d", &exp)
		}
		c, refs, e := s.Obs.SupersedeEvidence(r.Context(), id, in, exp, r.Header.Get("X-Request-ID"), r.Header.Get("X-Actor"))
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrRevision) || errors.Is(e, repository.ErrIdempotency) || len(refs) > 0 || strings.Contains(e.Error(), "不能撤回") {
				code = 409
			}
			if errors.Is(e, repository.ErrNotFound) {
				code = 404
			}
			write(w, map[string]interface{}{"error": e.Error(), "references": refs}, code)
			return
		}
		write(w, c, 200)
		return
	}
	if len(parts) >= 3 && parts[1] == "evidence" && (parts[len(parts)-1] == "note" || parts[len(parts)-1] == "notes") && r.Method == "POST" {
		var in struct {
			Note string `json:"note"`
		}
		if body(r, &in) != nil {
			write(w, map[string]string{"error": "请求格式错误"}, 400)
			return
		}
		exp := 0
		fmt.Sscanf(r.Header.Get("X-Expected-Revision"), "%d", &exp)
		actor := r.Header.Get("X-Actor")
		c, e := s.Obs.AddVerificationNote(r.Context(), id, in.Note, actor, r.Header.Get("X-Request-ID"), exp)
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrRevision) {
				code = 409
			}
			if errors.Is(e, repository.ErrNotFound) {
				code = 404
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		write(w, c, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "screen" && r.Method == "POST" {
		var screenIn struct {
			RuleProfile string `json:"rule_profile"`
		}
		_ = body(r, &screenIn)
		exp := 0
		fmt.Sscanf(r.Header.Get("X-Expected-Revision"), "%d", &exp)
		if exp == 0 {
			if c, er := s.Obs.Get(r.Context(), id); er == nil {
				exp = c.Revision
			}
		}
		actor := r.Header.Get("X-Actor")
		if actor == "" {
			actor = r.Header.Get("X-Operator")
		}
		q, e := s.Quality.ScreenExpectedProfile(r.Context(), id, actor, r.Header.Get("X-Request-ID"), exp, screenIn.RuleProfile)
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrRevision) {
				code = 409
			}
			if errors.Is(e, repository.ErrIdempotency) {
				code = 409
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		write(w, q, 200)
		return
	}
	if len(parts) == 3 && parts[1] == "review" && parts[2] == "claim" && r.Method == "POST" {
		exp := 0
		fmt.Sscanf(r.Header.Get("X-Expected-Revision"), "%d", &exp)
		claim, e := s.Quality.ClaimReview(r.Context(), id, r.Header.Get("X-Actor"), r.Header.Get("X-Request-ID"), exp)
		if e != nil {
			code := 400
			var ce *repository.ClaimConflictError
			if errors.As(e, &ce) || errors.Is(e, repository.ErrRevision) {
				code = 409
			}
			if errors.Is(e, repository.ErrNotFound) {
				code = 404
			}
			if ce != nil {
				write(w, map[string]interface{}{"error": e.Error(), "claim": ce.Claim, "remaining_seconds": int64(time.Until(ce.Claim.LeaseUntil).Seconds())}, code)
			} else {
				write(w, map[string]string{"error": e.Error()}, code)
			}
			return
		}
		write(w, claim, 200)
		return
	}
	if len(parts) == 3 && parts[1] == "review" && parts[2] == "claim" && r.Method == "DELETE" {
		claim, e := s.Quality.ReleaseReviewClaim(r.Context(), id, r.Header.Get("X-Actor"), r.Header.Get("X-Request-ID"))
		if e != nil {
			code := 400
			var ce *repository.ClaimConflictError
			if errors.As(e, &ce) {
				code = 409
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		write(w, claim, 200)
		return
	}
	if len(parts) == 3 && parts[1] == "review" && parts[2] == "history" && r.Method == "GET" || len(parts) == 2 && parts[1] == "review" && r.Method == "GET" {
		h, e := s.Quality.ReviewHistory(r.Context(), id)
		if e != nil {
			write(w, map[string]string{"error": e.Error()}, 404)
			return
		}
		start, lim := 0, 20
		fmt.Sscanf(r.URL.Query().Get("cursor"), "%d", &start)
		if x := r.URL.Query().Get("limit"); x != "" {
			fmt.Sscanf(x, "%d", &lim)
		}
		if lim <= 0 {
			lim = 20
		}
		if lim > 100 {
			lim = 100
		}
		if start < 0 || start > len(h) {
			write(w, map[string]string{"error": "游标格式错误"}, 400)
			return
		}
		end := start + lim
		if end > len(h) {
			end = len(h)
		}
		next := ""
		if end < len(h) {
			next = fmt.Sprint(end)
		}
		write(w, map[string]interface{}{"history": h[start:end], "total": len(h), "next_cursor": next}, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "sign-readiness" && r.Method == "GET" {
		candidate := r.URL.Query().Get("reviewer")
		if candidate == "" {
			candidate = r.Header.Get("X-Actor")
		}
		sr, e := s.Quality.SignReadiness(r.Context(), id, candidate)
		if e != nil {
			write(w, map[string]string{"error": e.Error()}, 404)
			return
		}
		write(w, sr, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "review" && r.Method == "POST" {
		var in quality.ReviewInput
		if body(r, &in) != nil {
			write(w, map[string]string{"error": "请求格式错误"}, 400)
			return
		}
		if in.Reviewer == "" {
			in.Reviewer = r.Header.Get("X-Actor")
		}
		var exp int
		fmt.Sscanf(r.Header.Get("X-Expected-Revision"), "%d", &exp)
		if exp == 0 {
			exp = in.ExpectedRevision
		}
		q, e := s.Quality.Review(r.Context(), id, in, exp, r.Header.Get("X-Request-ID"))
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrRevision) {
				code = 409
			}
			if errors.Is(e, repository.ErrIdempotency) {
				code = 409
			}
			var ce *repository.ClaimConflictError
			if errors.As(e, &ce) {
				write(w, map[string]interface{}{"error": e.Error(), "claim": ce.Claim, "remaining_seconds": int64(time.Until(ce.Claim.LeaseUntil).Seconds())}, 409)
				return
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		write(w, q, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "sign" && r.Method == "POST" {
		var in quality.SignInput
		if body(r, &in) != nil {
			write(w, map[string]string{"error": "请求格式错误"}, 400)
			return
		}
		if in.Reviewer == "" {
			in.Reviewer = r.Header.Get("X-Actor")
		}
		var exp int
		fmt.Sscanf(r.Header.Get("X-Expected-Revision"), "%d", &exp)
		if exp == 0 {
			exp = in.ExpectedRevision
		}
		q, e := s.Quality.Sign(r.Context(), id, in, exp, r.Header.Get("X-Request-ID"))
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrRevision) {
				code = 409
			}
			if errors.Is(e, repository.ErrIdempotency) {
				code = 409
			}
			if errors.Is(e, quality.ErrDeclarationConflict) {
				code = 409
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		write(w, q, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "freeze" && r.Method == "POST" {
		var bodyIn struct {
			PreviewDigest string            `json:"preview_digest"`
			PartDigests   map[string]string `json:"part_digests"`
		}
		_ = body(r, &bodyIn)
		var exp int
		fmt.Sscanf(r.Header.Get("X-Expected-Revision"), "%d", &exp)
		actor := r.Header.Get("X-Actor")
		if actor == "" {
			actor = r.Header.Get("X-Operator")
		}
		b, e := s.Quality.FreezeWithPartPreview(r.Context(), id, actor, r.Header.Get("X-Request-ID"), exp, bodyIn.PreviewDigest, bodyIn.PartDigests)
		if e != nil {
			var me *quality.ManifestMismatchError
			if errors.As(e, &me) {
				write(w, map[string]interface{}{"error": e.Error(), "preview_stale": true, "differences": me.Differences}, 409)
				return
			}
			write(w, map[string]string{"error": e.Error()}, 409)
			return
		}
		write(w, b, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "audit" && r.Method == "GET" {
		lim := 20
		if x := r.URL.Query().Get("limit"); x != "" {
			fmt.Sscanf(x, "%d", &lim)
		}
		q := r.URL.Query()
		allEvents, _ := s.Audit.Timeline(r.Context(), id)
		var from, to *time.Time
		if x := q.Get("from"); x != "" {
			if t, er := time.Parse(time.RFC3339, x); er == nil {
				from = &t
			}
		}
		if x := q.Get("to"); x != "" {
			if t, er := time.Parse(time.RFC3339, x); er == nil {
				to = &t
			}
		}
		v, next, integrity, count, e := s.Audit.TimelineQueryRequest(r.Context(), id, q.Get("action"), q.Get("actor"), q.Get("request_id"), from, to, q.Get("cursor"), lim)
		if e != nil {
			code := 400
			if errors.Is(e, repository.ErrNotFound) {
				code = 404
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		if q.Get("summary") == "1" {
			sm, er := s.Audit.SummaryFor(r.Context(), id, v, allEvents)
			if er != nil {
				write(w, map[string]string{"error": er.Error()}, 404)
				return
			}
			write(w, map[string]interface{}{"summary": sm, "stats": sm.Stats, "integrity": sm.Integrity, "first_invalid_index": sm.FirstInvalidIndex, "alert_level": sm.AlertLevel, "next_cursor": next, "count": count}, 200)
			return
		}
		all, _ := s.Audit.Timeline(r.Context(), id)
		_, bad := s.Audit.VerifyWithIndex(all)
		write(w, map[string]interface{}{"events": v, "next_cursor": next, "integrity": integrity, "first_invalid_index": bad, "count": count, "total": count}, 200)
		return
	}
	if len(parts) >= 2 && parts[1] == "evidence" && r.Method == "GET" {
		if len(parts) == 3 && parts[2] != "report" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		coverage := q.Get("coverage") == "1" || strings.EqualFold(q.Get("coverage"), "true")
		rep, item, e := s.Obs.EvidenceReportCoverage(r.Context(), id, q.Get("sensor_id"), q.Get("evidence_id"), coverage)
		if e != nil {
			code := 404
			if !errors.Is(e, repository.ErrNotFound) {
				code = 400
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		if item != nil {
			write(w, map[string]interface{}{"evidence": item, "revision": rep.Revision}, 200)
		} else {
			_, _ = s.Audit.Append(r.Context(), id, r.Header.Get("X-Request-ID"), "evidence_report_view", r.Header.Get("X-Actor"), "", "", map[string]interface{}{"score": rep.Score, "gaps": len(rep.Gaps)})
			write(w, rep, 200)
		}
		return
	}
	if len(parts) == 2 && parts[1] == "manifest" && r.Method == "GET" {
		part := r.URL.Query().Get("part")
		if part != "" {
			content, digest, b, e := s.Quality.ManifestPart(r.Context(), id, part, r.URL.Query().Get("root_digest"))
			if e != nil {
				code := 400
				if errors.Is(e, repository.ErrNotFound) {
					code = 404
				} else if strings.Contains(e.Error(), "根摘要") {
					code = 409
				}
				write(w, map[string]string{"error": e.Error()}, code)
				return
			}
			write(w, map[string]interface{}{"part": part, "content": content, "part_digest": digest, "root_digest": b.RootDigest, "frozen_revision": b.FrozenRevision, "verified": true}, 200)
			return
		}
		b, e := s.Quality.GetBundle(r.Context(), id)
		if e != nil {
			write(w, map[string]string{"error": "发布包不存在"}, 404)
			return
		}
		write(w, map[string]interface{}{"manifest": b.ManifestJSON, "content_digest": b.ContentDigest, "root_digest": b.RootDigest, "part_digests": b.PartDigests, "part_order": b.PartOrder, "frozen_revision": b.FrozenRevision, "download_count": b.DownloadCount, "manifest_schema_version": b.ManifestSchemaVersion, "version_label": b.VersionLabel}, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "preview" && r.Method == "GET" {
		m, d, e := s.Quality.Preview(r.Context(), id)
		if e != nil {
			write(w, map[string]string{"error": e.Error()}, 409)
			return
		}
		rev := 0
		if x, ok := m["revision"].(int); ok {
			rev = x
		}
		write(w, map[string]interface{}{"manifest": m, "content_digest": d, "root_digest": d, "part_digests": m["part_digests"], "part_order": m["part_order"], "manifest_schema_version": m["manifest_schema_version"], "revision": rev, "differences": []string{}}, 200)
		return
	}
	if len(parts) == 2 && parts[1] == "download" && r.Method == "GET" {
		actor := strings.TrimSpace(r.Header.Get("X-Actor"))
		if actor == "" || !s.PublishActors[actor] {
			write(w, map[string]string{"error": "未授权的发布访问者"}, 403)
			return
		}
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			write(w, map[string]string{"error": "缺少 X-Request-ID"}, 400)
			return
		}
		c, ce := s.Obs.Get(r.Context(), id)
		if ce != nil || c.Status != observation.StatusFrozen {
			write(w, map[string]string{"error": "个案尚未冻结，不可下载"}, 409)
			return
		}
		b, e := s.Quality.GetBundle(r.Context(), id)
		if e != nil {
			write(w, map[string]string{"error": "发布包不存在"}, 404)
			return
		}
		if b.FrozenRevision != c.Revision-1 {
			write(w, map[string]string{"error": "冻结版本不一致"}, 409)
			return
		}
		fp := repository.Fingerprint(struct{ ID, Actor, Digest string }{id, actor, b.ContentDigest})
		var inc bool
		b, inc, e = s.Repo.RecordDownload(r.Context(), id, requestID, fp, actor)
		if e != nil {
			code := 409
			if errors.Is(e, repository.ErrNotFound) {
				code = 404
			}
			write(w, map[string]string{"error": e.Error()}, code)
			return
		}
		if inc {
			_, _ = s.Audit.Append(r.Context(), id, requestID, "download_release", actor, string(c.Status), string(c.Status), map[string]interface{}{"digest": b.ContentDigest})
		}
		w.Header().Set("X-Content-Digest", b.ContentDigest)
		w.Header().Set("Content-Disposition", "attachment; filename=release-manifest.json")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(b.ManifestJSON))
		return
	}
	if len(parts) == 3 && parts[1] == "download" && parts[2] == "stats" && r.Method == "GET" {
		b, e := s.Quality.GetBundle(r.Context(), id)
		if e != nil {
			write(w, map[string]string{"error": e.Error()}, 404)
			return
		}
		write(w, map[string]interface{}{"download_count": b.DownloadCount, "last_download_at": b.LastDownloadAt, "by_actor": b.DownloadActors}, 200)
		return
	}
	http.NotFound(w, r)
}

var page = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>潮声观测质检台</title><style>body{font-family:system-ui;margin:2rem;background:#eef5f7;color:#17324d}main{max-width:900px;margin:auto;background:#fff;padding:2rem;border-radius:12px}input,button{padding:.55rem;margin:.25rem}button{background:#176b87;color:#fff;border:0;border-radius:4px}.card{border:1px solid #ccdce2;padding:1rem;margin:1rem 0}</style></head><body><main><h1>潮声观测质检台</h1><p>登记观测、提交证据并推进质量审核与发布冻结。</p><section class="card"><h2>创建观测</h2><input id="buoy" placeholder="浮标编号"><input id="region" placeholder="海域"><button onclick="create()">创建</button></section><section class="card"><h2>观测列表</h2><button onclick="load()">刷新</button><pre id="out"></pre></section></main><script>async function create(){let d={buoy_id:buoy.value,region:region.value,species_scope:'海洋哺乳动物',started_at:new Date(Date.now()-3600000).toISOString(),ended_at:new Date().toISOString(),created_by:'观测员'};let r=await fetch('/api/v1/cases',{method:'POST',headers:{'Content-Type':'application/json','X-Request-ID':crypto.randomUUID()},body:JSON.stringify(d)});out.textContent=await r.text();load()}async function load(){let r=await fetch('/api/v1/cases');out.textContent=JSON.stringify(await r.json(),null,2)}load()</script></body></html>`
var _ = time.Now
