package quality

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/model"
	"github.com/benzhi/chao-sheng/internal/observation"
	"github.com/benzhi/chao-sheng/internal/repository"
	"sort"
	"strings"
	"time"
)

type Service struct {
	Repo              *repository.Repository
	Audit             *audit.Service
	manifestPartCache []manifestPartCacheEntry
}

type manifestPartCacheEntry struct {
	Part    string
	Content interface{}
	Digest  string
}

var ErrDeclarationConflict = errors.New("签署声明确认码不匹配")

func addAgg(m map[string]*model.RuleAggregate, sensor, rule string, passed bool, score int) {
	k := sensor + "\x00" + rule
	a := m[k]
	if a == nil {
		a = &model.RuleAggregate{SensorID: sensor, RuleName: rule}
		m[k] = a
	}
	if passed {
		a.Passed++
	} else {
		a.Failed++
	}
	a.AverageScore += float64(score)
}
func avgAgg(a model.RuleAggregate) float64 {
	n := a.Passed + a.Failed
	if n == 0 {
		return 0
	}
	return a.AverageScore / float64(n)
}

func New(r *repository.Repository, a *audit.Service) *Service { return &Service{Repo: r, Audit: a} }
func (s *Service) RuleProfiles() []RuleProfile {
	return []RuleProfile{
		{ProfileID: "profile-v1", Version: 1, Active: true, SamplingRateMinimum: 8000, Weights: map[string]int{"audio_sampling_rate": 40, "digest_integrity": 30, "calibration_present": 30}},
		{ProfileID: "profile-v2", Version: 2, Active: true, SamplingRateMinimum: 16000, Weights: map[string]int{"audio_sampling_rate": 45, "digest_integrity": 30, "calibration_present": 25}},
		{ProfileID: "profile-legacy", Version: 0, Active: false, SamplingRateMinimum: 8000, Weights: map[string]int{"audio_sampling_rate": 40, "digest_integrity": 30, "calibration_present": 30}},
	}
}
func (s *Service) profile(id string) (RuleProfile, error) {
	if id == "" {
		id = "profile-v1"
	}
	for _, p := range s.RuleProfiles() {
		if p.ProfileID == id {
			if !p.Active {
				return p, errors.New("规则配置已停用")
			}
			return p, nil
		}
	}
	return RuleProfile{}, errors.New("未知规则配置版本")
}
func (s *Service) Screen(ctx context.Context, id, actor, requestID string) (QualityReview, error) {
	c, _ := s.Repo.GetCase(ctx, id)
	return s.screen(ctx, id, actor, requestID, c.Revision, "")
}
func (s *Service) ScreenExpected(ctx context.Context, id, actor, requestID string, expected int) (QualityReview, error) {
	return s.screen(ctx, id, actor, requestID, expected, "")
}
func (s *Service) ScreenExpectedProfile(ctx context.Context, id, actor, requestID string, expected int, profileID string) (QualityReview, error) {
	return s.screen(ctx, id, actor, requestID, expected, profileID)
}
func (s *Service) screen(ctx context.Context, id, actor, requestID string, expected int, profileID string) (QualityReview, error) {
	profile, e := s.profile(profileID)
	if e != nil {
		return QualityReview{}, e
	}
	fp := repository.Fingerprint(struct {
		ID, Actor, Profile string
		Expected           int
	}{id, actor, profile.ProfileID, expected})
	if requestID != "" {
		if old, e := s.Repo.Idempotent(ctx, requestID, fp); e != nil {
			return QualityReview{}, e
		} else if old != "" {
			return s.Repo.GetReview(ctx, id)
		}
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return QualityReview{}, e
	}
	ev, e := s.Repo.ListEvidence(ctx, id)
	if e != nil {
		return QualityReview{}, e
	}
	if c.Revision != expected {
		return QualityReview{}, repository.ErrRevision
	}
	if c.Status != observation.StatusEvidence && c.Status != observation.StatusResample {
		return QualityReview{}, errors.New("仅证据状态允许初筛或重跑")
	}
	if len(ev) == 0 {
		return QualityReview{}, errors.New("缺少校准证据")
	}
	active := ev[:0]
	for _, x := range ev {
		if !x.Withdrawn {
			active = append(active, x)
		}
	}
	ev = active
	sort.Slice(ev, func(i, j int) bool { return ev[i].EvidenceID < ev[j].EvidenceID })
	results := []RuleResult{}
	aggMap := map[string]*model.RuleAggregate{}
	total := 0
	for _, x := range ev {
		p := x.SamplingRate >= profile.SamplingRateMinimum
		score := 0
		if p {
			score = profile.Weights["audio_sampling_rate"]
			total += score
		}
		results = append(results, RuleResult{Name: "audio_sampling_rate", EvidenceID: x.EvidenceID, SensorID: x.SensorID, Passed: p, Score: score, Explanation: fmt.Sprintf("采样率 %dHz，要求至少 %dHz", x.SamplingRate, profile.SamplingRateMinimum)})
		addAgg(aggMap, x.SensorID, "audio_sampling_rate", p, score)
		dp := len(x.AudioDigest) == 64
		if dp {
			_, err := hex.DecodeString(x.AudioDigest)
			dp = err == nil
		}
		ds := 0
		if dp {
			ds = profile.Weights["digest_integrity"]
			total += ds
		}
		results = append(results, RuleResult{Name: "digest_integrity", EvidenceID: x.EvidenceID, SensorID: x.SensorID, Passed: dp, Score: ds, Explanation: "音频摘要为64位十六进制字符串"})
		addAgg(aggMap, x.SensorID, "digest_integrity", dp, ds)
		cp := strings.TrimSpace(x.CalibrationRef) != "" && !x.CalibratedAt.Before(c.StartedAt) && !x.CalibratedAt.After(c.EndedAt)
		cs := 0
		if cp {
			cs = profile.Weights["calibration_present"]
			total += cs
		}
		results = append(results, RuleResult{Name: "calibration_present", EvidenceID: x.EvidenceID, SensorID: x.SensorID, Passed: cp, Score: cs, Explanation: "校准记录存在且时间位于观测时段内"})
		addAgg(aggMap, x.SensorID, "calibration_present", cp, cs)
	}
	if len(ev) > 1 {
		total /= len(ev)
	}
	anomalies := []string{}
	for _, rr := range results {
		if !rr.Passed {
			anomalies = append(anomalies, rr.Name)
		}
	}
	risk := "low"
	if total < 90 {
		risk = "medium"
	}
	if total < 60 {
		risk = "high"
	}
	aggs := make([]model.RuleAggregate, 0, len(aggMap))
	for _, a := range aggMap {
		a.AverageScore = avgAgg(*a)
		aggs = append(aggs, *a)
	}
	sort.Slice(aggs, func(i, j int) bool {
		if aggs[i].SensorID == aggs[j].SensorID {
			return aggs[i].RuleName < aggs[j].RuleName
		}
		return aggs[i].SensorID < aggs[j].SensorID
	})
	params := map[string]interface{}{"sampling_rate_minimum": profile.SamplingRateMinimum, "weights": profile.Weights}
	q := QualityReview{ReviewID: fmt.Sprintf("review-%d", time.Now().UnixNano()), CaseID: id, RuleResults: results, Anomalies: anomalies, Decision: "pending", TotalScore: total, RiskLevel: risk, Aggregates: aggs, RunAt: time.Now().UTC(), CaseRevision: c.Revision, RuleProfileID: profile.ProfileID, RuleProfileVersion: profile.Version, RuleParameters: params, RuleFingerprint: audit.ManifestDigest(params)}
	q.ScreenedBy = actor
	if previous, er := s.Repo.GetReview(ctx, id); er == nil {
		q.PreviousReviewID = previous.ReviewID
		changes := []RuleChange{}
		pm := map[string]RuleResult{}
		for _, rr := range previous.RuleResults {
			pm[rr.SensorID+"\x00"+rr.Name] = rr
		}
		for _, rr := range results {
			if p, ok := pm[rr.SensorID+"\x00"+rr.Name]; ok && p.Score != rr.Score {
				ch := "changed"
				if !p.Passed && rr.Passed {
					ch = "recovered"
				}
				if p.Passed && !rr.Passed {
					ch = "new_failure"
				}
				changes = append(changes, RuleChange{Rule: rr.Name, SensorID: rr.SensorID, PreviousScore: p.Score, CurrentScore: rr.Score, Change: ch})
			}
		}
		q.Comparison = ScreenComparison{PreviousRunAt: &previous.RunAt, PreviousScore: previous.TotalScore, CurrentScore: total, RiskBefore: previous.RiskLevel, RiskAfter: risk, Changes: changes, ProfileChanged: previous.RuleProfileID != "" && previous.RuleProfileID != profile.ProfileID, PreviousProfile: previous.RuleProfileID, CurrentProfile: profile.ProfileID}
	}
	from := c.Status
	c.Status = observation.StatusScreened
	c.Revision++
	if e = s.Repo.SaveReviewAndCase(ctx, q, c, expected, false); e != nil {
		return q, e
	}
	_ = s.Repo.SaveReviewSnapshot(ctx, q)
	_, e = s.Audit.Append(ctx, id, requestID, "quality_screen", actor, string(from), string(c.Status), map[string]interface{}{"profile_id": profile.ProfileID, "profile_version": profile.Version, "rule_fingerprint": q.RuleFingerprint, "results": results})
	if requestID != "" {
		_ = s.Repo.PutIdempotent(ctx, requestID, fp, id)
	}
	return q, e
}

func (s *Service) ClaimReview(ctx context.Context, id, actor, requestID string, expected int) (repository.ReviewClaim, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return repository.ReviewClaim{}, errors.New("质检员不能为空")
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return repository.ReviewClaim{}, e
	}
	if c.Status != observation.StatusScreened && c.Status != observation.StatusResample {
		return repository.ReviewClaim{}, errors.New("仅 screened 或 resample 个案可认领")
	}
	old, _ := s.Repo.PeekReviewClaim(ctx, id)
	claim, reassigned, e := s.Repo.ClaimReview(ctx, id, actor, expected, time.Now().UTC(), 15*time.Minute)
	if e != nil {
		return claim, e
	}
	action := "review_claim"
	if reassigned {
		action = "review_reassign"
	} else if old.Actor == actor {
		action = "review_claim_renew"
	}
	_, e = s.Audit.Append(ctx, id, requestID, action, actor, string(c.Status), string(c.Status), map[string]interface{}{"claim": claim, "previous_actor": old.Actor})
	return claim, e
}
func (s *Service) ReleaseReviewClaim(ctx context.Context, id, actor, requestID string) (repository.ReviewClaim, error) {
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return repository.ReviewClaim{}, e
	}
	claim, e := s.Repo.ReleaseReviewClaim(ctx, id, strings.TrimSpace(actor))
	if e != nil {
		return claim, e
	}
	_, e = s.Audit.Append(ctx, id, requestID, "review_claim_release", actor, string(c.Status), string(c.Status), claim)
	return claim, e
}

func (s *Service) Review(ctx context.Context, id string, in ReviewInput, expected int, requestID string) (QualityReview, error) {
	fp := repository.Fingerprint(struct {
		ID       string
		In       ReviewInput
		Expected int
	}{id, in, expected})
	if requestID != "" {
		if old, e := s.Repo.Idempotent(ctx, requestID, fp); e != nil {
			return QualityReview{}, e
		} else if old != "" {
			return s.Repo.GetReview(ctx, id)
		}
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return QualityReview{}, e
	}
	if c.Revision != expected {
		return QualityReview{}, repository.ErrRevision
	}
	if c.Status != observation.StatusScreened && c.Status != observation.StatusResample {
		return QualityReview{}, errors.New("个案未处于待复核状态")
	}
	if strings.TrimSpace(in.Reviewer) == "" {
		return QualityReview{}, errors.New("审核人不能为空")
	}
	if claim, er := s.Repo.GetReviewClaim(ctx, id, time.Now().UTC()); er == nil {
		if claim.Actor != in.Reviewer || claim.ClaimedRevision != expected {
			return QualityReview{}, &repository.ClaimConflictError{Claim: claim}
		}
	}
	q, e := s.Repo.GetReview(ctx, id)
	if e != nil {
		return q, e
	}
	if in.Decision != "resample" && in.Decision != "approve" {
		return q, errors.New("复核决定无效")
	}
	if len(in.Anomalies) == 0 && in.Decision != "approve" {
		return q, errors.New("必须提供异常代码或明确无异常说明")
	}
	known := map[string]bool{}
	for _, rr := range q.RuleResults {
		if !rr.Passed {
			known[rr.Name] = true
		}
	}
	seen := map[string]bool{}
	for _, a := range in.Anomalies {
		if !known[a] {
			return q, fmt.Errorf("异常代码无效: %s", a)
		}
		if seen[a] {
			return q, errors.New("异常代码重复")
		}
		seen[a] = true
	}
	if in.Decision == "approve" && len(in.Anomalies) > 0 && len(in.SupplementRefs) == 0 && strings.TrimSpace(in.Disposition) == "" {
		return q, errors.New("所有异常必须有补充证据引用或处置说明")
	}
	if in.Decision == "approve" && len(known) > 0 && len(in.Anomalies) != len(known) {
		return q, errors.New("必须逐项处置所有异常")
	}
	if in.Decision == "approve" && len(in.Anomalies) > 0 && strings.TrimSpace(in.Disposition) == "" && len(in.SupplementRefs) < len(in.Anomalies) && len(in.EvidenceByAnomaly) == 0 {
		return q, errors.New("每项异常必须有补充证据引用")
	}
	if in.Decision == "resample" && strings.TrimSpace(in.ResampleReason) == "" {
		return q, errors.New("退回重采必须填写原因")
	}
	ev, _ := s.Repo.ListEvidence(ctx, id)
	valid := map[string]bool{}
	for _, x := range ev {
		valid[x.EvidenceID] = true
	}
	for _, ref := range in.SupplementRefs {
		if !valid[ref] {
			return q, errors.New("补充证据引用不存在")
		}
	}
	for anomaly, refs := range in.EvidenceByAnomaly {
		if !known[anomaly] {
			return q, errors.New("异常代码无效")
		}
		for _, ref := range refs {
			if !valid[ref] {
				return q, errors.New("补充证据引用不存在")
			}
		}
	}
	if in.Decision == "approve" {
		for _, anomaly := range in.Anomalies {
			if len(in.EvidenceByAnomaly[anomaly]) == 0 && len(in.SupplementRefs) == 0 && strings.TrimSpace(in.Disposition) == "" {
				return q, errors.New("每项异常必须有补充证据或处置说明")
			}
		}
	}
	q.Anomalies = in.Anomalies
	q.SupplementRefs = in.SupplementRefs
	q.EvidenceByAnomaly = in.EvidenceByAnomaly
	q.Reviewer = in.Reviewer
	q.SubmittedBy = in.Reviewer
	q.Decision = in.Decision
	q.ResampleReason = in.ResampleReason
	q.Disposition = in.Disposition
	if q.Decision == "resample" {
		c.Status = observation.StatusResample
	} else {
		c.Status = observation.StatusReviewing
	}
	c.Revision++
	if e = s.Repo.SaveReviewAndCase(ctx, q, c, expected, true); e != nil {
		return q, e
	}
	_, e = s.Audit.Append(ctx, id, requestID, "review_anomaly", in.Reviewer, string(observation.StatusScreened), string(c.Status), in)
	if requestID != "" {
		_ = s.Repo.PutIdempotent(ctx, requestID, fp, id)
	}
	return q, e
}
func (s *Service) Sign(ctx context.Context, id string, in SignInput, expected int, requestID string) (QualityReview, error) {
	if in.Token == "" {
		in.Token = in.DeclarationToken
	}
	fp := repository.Fingerprint(struct {
		ID       string
		In       SignInput
		Expected int
	}{id, in, expected})
	if requestID != "" {
		if old, er := s.Repo.Idempotent(ctx, requestID, fp); er != nil {
			return QualityReview{}, er
		} else if old != "" {
			return s.Repo.GetReview(ctx, id)
		}
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return QualityReview{}, e
	}
	if c.Revision != expected {
		return QualityReview{}, repository.ErrRevision
	}
	q, e := s.Repo.GetReview(ctx, id)
	if e != nil {
		return q, e
	}
	if c.Status != observation.StatusReviewing {
		return q, errors.New("个案未完成复核")
	}
	if strings.TrimSpace(in.Reviewer) == "" {
		return q, errors.New("审核人标识无效")
	}
	if q.SignedAt != nil || q.Decision != "approve" {
		return q, errors.New("审核结论已锁定或复核未完成")
	}
	// Sign 是不可逆迁移，不能只依赖早先的 GET 预检；在同一写入路径
	// 再次计算证据完整性和异常闭环，避免预检后数据变化导致误签。
	readiness, re := s.SignReadiness(ctx, id, strings.TrimSpace(in.Reviewer))
	if re != nil {
		return q, re
	}
	if !readiness.Ready {
		return q, fmt.Errorf("签署门槛未通过: %s", strings.Join(readiness.Failures, ";"))
	}
	if in.Token == "" || in.Token != readiness.DeclarationToken || !strings.Contains(in.DeclarationText, in.Token) {
		return q, ErrDeclarationConflict
	}
	if n := len([]rune(strings.TrimSpace(in.DeclarationText))); n < 20 || n > 1000 {
		return q, errors.New("签署声明长度必须为 20 至 1000 字符")
	}
	submitter := q.SubmittedBy
	if submitter == "" {
		submitter = q.Reviewer
	}
	if submitter == in.Reviewer || q.ScreenedBy == in.Reviewer {
		return q, errors.New("审核人必须与质检员不同")
	}
	if strings.TrimSpace(in.Reviewer) == "" || len([]rune(in.Reviewer)) > 64 {
		return q, errors.New("审核人标识无效")
	}
	if len(in.Grade) != 1 || !strings.Contains("ABC", in.Grade) {
		return q, errors.New("质量等级必须为 A、B 或 C")
	}
	if q.RiskLevel == "high" && in.Grade == "A" {
		return q, errors.New("high 风险不得签署为 A")
	}
	if q.RiskLevel == "medium" && in.Grade == "A" && q.TotalScore < 80 {
		return q, errors.New("总分不足以签署为 A")
	}
	now := time.Now().UTC()
	q.Grade = in.Grade
	q.Reviewer = in.Reviewer
	q.SignedAt = &now
	q.DeclarationDigest = audit.ManifestDigest(strings.TrimSpace(in.DeclarationText))
	q.DeclarationConfirmedAt = &now
	q.Decision = "approved"
	from := c.Status
	c.Status = observation.StatusSigned
	c.Revision++
	if e = s.Repo.SaveReviewAndCase(ctx, q, c, expected, true); e != nil {
		return q, e
	}
	_, e = s.Audit.Append(ctx, id, requestID, "sign_quality", in.Reviewer, string(from), string(c.Status), map[string]interface{}{"grade": in.Grade, "declaration_token_digest": audit.ManifestDigest(in.Token), "declaration_digest": q.DeclarationDigest})
	if requestID != "" {
		_ = s.Repo.PutIdempotent(ctx, requestID, fp, id)
	}
	return q, e
}
func (s *Service) Freeze(ctx context.Context, id, actor, requestID string, expected int) (repository.Bundle, error) {
	return s.FreezeWithPreview(ctx, id, actor, requestID, expected, "")
}

type ManifestMismatchError struct{ Differences []string }

func (e *ManifestMismatchError) Error() string {
	return "preview_stale: 清单分片摘要已变化: " + strings.Join(e.Differences, ",")
}

func (s *Service) buildManifest(ctx context.Context, id string, c model.ObservationCase, q model.QualityReview) (map[string]interface{}, map[string]string, string, error) {
	metadata := map[string]interface{}{"case_id": id, "buoy_id": c.BuoyID, "region": c.Region, "species_scope": c.SpeciesScope, "started_at": c.StartedAt, "ended_at": c.EndedAt, "revision": c.Revision}
	type summary struct {
		EvidenceID  string `json:"evidence_id"`
		SensorID    string `json:"sensor_id"`
		AudioDigest string `json:"audio_digest"`
		Supersedes  string `json:"supersedes,omitempty"`
	}
	evidence := []summary{}
	evs, e := s.Repo.ListEvidenceSorted(ctx, id)
	if e != nil {
		return nil, nil, "", e
	}
	for _, ev := range evs {
		if !ev.Withdrawn {
			evidence = append(evidence, summary{ev.EvidenceID, ev.SensorID, ev.AudioDigest, ev.Supersedes})
		}
	}
	qualityPart := map[string]interface{}{"review_id": q.ReviewID, "grade": q.Grade, "total_score": q.TotalScore, "risk_level": q.RiskLevel, "profile_id": q.RuleProfileID, "profile_version": q.RuleProfileVersion, "rule_fingerprint": q.RuleFingerprint, "declaration_digest": q.DeclarationDigest, "declaration_confirmed_at": q.DeclarationConfirmedAt}
	parts := map[string]interface{}{"metadata": metadata, "evidence": evidence, "quality": qualityPart}
	order := []string{"metadata", "evidence", "quality"}
	digests := map[string]string{}
	for _, name := range order {
		digests[name] = audit.ManifestDigest(parts[name])
	}
	root := audit.ManifestDigest(map[string]interface{}{"part_order": order, "part_digests": digests})
	manifest := map[string]interface{}{"manifest_schema_version": "2.0", "case_id": id, "revision": c.Revision, "part_order": order, "part_digests": digests, "root_digest": root, "parts": parts, "declaration_digest": q.DeclarationDigest}
	return manifest, digests, root, nil
}
func (s *Service) FreezeWithPreview(ctx context.Context, id, actor, requestID string, expected int, previewDigest string) (repository.Bundle, error) {
	return s.FreezeWithPartPreview(ctx, id, actor, requestID, expected, previewDigest, nil)
}
func (s *Service) FreezeWithPartPreview(ctx context.Context, id, actor, requestID string, expected int, previewDigest string, previewParts map[string]string) (repository.Bundle, error) {
	fp := repository.Fingerprint(struct {
		ID       string
		Expected int
		Actor    string
		Preview  string
		Parts    map[string]string
	}{id, expected, actor, previewDigest, previewParts})
	if requestID != "" {
		if old, e := s.Repo.Idempotent(ctx, requestID, fp); e != nil {
			return repository.Bundle{}, e
		} else if old != "" {
			return s.Repo.GetBundle(ctx, id)
		}
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return repository.Bundle{}, e
	}
	if c.Revision != expected {
		return repository.Bundle{}, repository.ErrRevision
	}
	q, e := s.Repo.GetReview(ctx, id)
	if c.Status != observation.StatusSigned || e != nil || q.SignedAt == nil {
		return repository.Bundle{}, errors.New("尚未完成独立签署")
	}
	manifest, partDigests, digest, e := s.buildManifest(ctx, id, c, q)
	if e != nil {
		return repository.Bundle{}, e
	}
	mb, _ := json.Marshal(manifest)
	if previewDigest != "" && previewDigest != digest {
		diffs := []string{}
		for _, name := range []string{"metadata", "evidence", "quality"} {
			if previewParts == nil || previewParts[name] != partDigests[name] {
				diffs = append(diffs, name)
			}
		}
		if len(diffs) == 0 {
			diffs = []string{"root"}
		}
		_, _ = s.Audit.Append(ctx, id, requestID, "freeze_compare_failed", actor, string(c.Status), string(c.Status), map[string]interface{}{"differences": diffs, "preview_part_digests": previewParts, "current_part_digests": partDigests})
		return repository.Bundle{}, &ManifestMismatchError{Differences: diffs}
	}
	b := repository.Bundle{BundleID: fmt.Sprintf("bundle-%d", time.Now().UnixNano()), CaseID: id, ManifestJSON: string(mb), FrozenRevision: c.Revision, FrozenBy: actor, FrozenAt: time.Now().UTC(), ManifestSchemaVersion: "2.0", VersionLabel: fmt.Sprintf("v%d", c.Revision), PreviewDigest: digest, PartDigests: partDigests, PartOrder: []string{"metadata", "evidence", "quality"}, RootDigest: digest}
	b.ContentDigest = digest
	from := c.Status
	c.Status = observation.StatusFrozen
	c.Revision++
	if e = s.Repo.SaveBundleAndCase(ctx, b, c, expected); e != nil {
		return b, e
	}
	if requestID != "" {
		_ = s.Repo.PutIdempotent(ctx, requestID, fp, b.BundleID)
	}
	_, e = s.Audit.Append(ctx, id, requestID, "freeze_release", actor, string(from), string(c.Status), manifest)
	return b, e
}
func (s *Service) GetReview(ctx context.Context, id string) (QualityReview, error) {
	return s.Repo.GetReview(ctx, id)
}
func (s *Service) GetBundle(ctx context.Context, id string) (repository.Bundle, error) {
	return s.Repo.GetBundle(ctx, id)
}
func (s *Service) ReviewHistory(ctx context.Context, id string) ([]QualityReview, error) {
	return s.Repo.ListReviewHistory(ctx, id)
}
func (s *Service) SignReadiness(ctx context.Context, id, candidate string) (SignReadiness, error) {
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return SignReadiness{}, e
	}
	q, e := s.Repo.GetReview(ctx, id)
	if e != nil {
		return SignReadiness{}, e
	}
	r := SignReadiness{Revision: c.Revision, RiskLevel: q.RiskLevel, GradeOptions: []string{"A", "B", "C"}, Failures: []string{}}
	evs, _ := s.Repo.ListEvidenceSorted(ctx, id)
	evidenceSummary := []map[string]string{}
	for _, ev := range evs {
		if !ev.Withdrawn {
			evidenceSummary = append(evidenceSummary, map[string]string{"evidence_id": ev.EvidenceID, "audio_digest": ev.AudioDigest})
		}
	}
	tokenDigest := audit.ManifestDigest(map[string]interface{}{"case_id": id, "revision": c.Revision, "review_id": q.ReviewID, "evidence": evidenceSummary})
	if len(tokenDigest) > 20 {
		r.DeclarationToken = tokenDigest[:20]
	} else {
		r.DeclarationToken = tokenDigest
	}
	if rep, _, er := s.evidenceReadiness(ctx, id); er == nil && rep.Blocking {
		r.Failures = append(r.Failures, "证据完整性存在阻断项")
	}
	if q.SignedAt != nil || c.Status == observation.StatusSigned || c.Status == observation.StatusFrozen {
		r.Failures = append(r.Failures, "个案已签署")
	}
	closed := len(q.Anomalies) == 0
	if !closed {
		closed = strings.TrimSpace(q.Disposition) != "" || len(q.SupplementRefs) >= len(q.Anomalies)
		if !closed && len(q.EvidenceByAnomaly) > 0 {
			closed = true
			for _, anomaly := range q.Anomalies {
				if len(q.EvidenceByAnomaly[anomaly]) == 0 {
					closed = false
					break
				}
			}
		}
	}
	if q.Decision != "approve" || !closed {
		r.Failures = append(r.Failures, "存在未闭环异常")
	}
	if strings.TrimSpace(candidate) == "" {
		r.Failures = append(r.Failures, "审核人标识不能为空")
	}
	if candidate == q.Reviewer || candidate == q.SubmittedBy || candidate == q.ScreenedBy {
		r.Failures = append(r.Failures, "审核人必须独立")
	}
	r.Ready = len(r.Failures) == 0
	return r, nil
}
func (s *Service) evidenceReadiness(ctx context.Context, id string) (observation.EvidenceReport, interface{}, error) {
	return observation.New(s.Repo, s.Audit).EvidenceReport(ctx, id, "", "")
}
func (s *Service) Preview(ctx context.Context, id string) (map[string]interface{}, string, error) {
	c, err := s.Repo.GetCase(ctx, id)
	if err != nil {
		return nil, "", err
	}
	q, err := s.Repo.GetReview(ctx, id)
	if err != nil || q.SignedAt == nil {
		return nil, "", errors.New("尚未完成独立签署")
	}
	manifest, _, root, e := s.buildManifest(ctx, id, c, q)
	return manifest, root, e
}

func (s *Service) ManifestPart(ctx context.Context, id, part, root string) (interface{}, string, repository.Bundle, error) {
	b, e := s.Repo.GetBundle(ctx, id)
	if e != nil {
		return nil, "", b, e
	}
	if part != "metadata" && part != "evidence" && part != "quality" {
		return nil, "", b, errors.New("未知清单分片")
	}
	if root != "" && root != b.RootDigest {
		return nil, "", b, errors.New("根摘要不匹配")
	}
	for _, cached := range s.manifestPartCache {
		if cached.Part == part {
			return cached.Content, cached.Digest, b, nil
		}
	}
	var manifest map[string]interface{}
	if e = json.Unmarshal([]byte(b.ManifestJSON), &manifest); e != nil {
		return nil, "", b, e
	}
	parts, _ := manifest["parts"].(map[string]interface{})
	content, digest := parts[part], b.PartDigests[part]
	s.manifestPartCache = append(s.manifestPartCache, manifestPartCacheEntry{Part: part, Content: content, Digest: digest})
	return content, digest, b, nil
}
