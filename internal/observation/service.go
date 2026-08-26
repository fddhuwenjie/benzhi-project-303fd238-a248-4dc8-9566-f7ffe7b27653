package observation

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/repository"
	"sort"
	"strings"
	"sync"
	"time"
)

type Service struct {
	Repo              *repository.Repository
	Audit             *audit.Service
	CoverageThreshold float64
	reportCacheMu     sync.RWMutex
	reportCache       map[string]EvidenceReport
}

func New(r *repository.Repository, a *audit.Service) *Service {
	return &Service{Repo: r, Audit: a, CoverageThreshold: 0.5, reportCache: map[string]EvidenceReport{}}
}

func cloneEvidenceReport(r EvidenceReport) EvidenceReport {
	out := r
	out.Sensors = append([]SensorEvidenceSummary(nil), r.Sensors...)
	for i := range out.Sensors {
		out.Sensors[i].SamplingRates = append([]int(nil), r.Sensors[i].SamplingRates...)
	}
	out.Differences = append([]EvidenceDifference(nil), r.Differences...)
	out.Gaps = append([]EvidenceGap(nil), r.Gaps...)
	out.Coverage = append([]SensorCoverage(nil), r.Coverage...)
	for i := range out.Coverage {
		out.Coverage[i].Warnings = append([]string(nil), r.Coverage[i].Warnings...)
	}
	return out
}
func (s *Service) Precheck(ctx context.Context, in CreateInput) PrecheckResult {
	r := PrecheckResult{Valid: true, Errors: map[string]string{}, Conflicts: []repository.Overlap{}}
	r.BuoyID = strings.ToUpper(strings.TrimSpace(in.BuoyID))
	r.Region = normalizeRegion(in.Region)
	if r.BuoyID == "" || len([]rune(r.BuoyID)) > 64 {
		r.Errors["buoy_id"] = "浮标编号不能为空或过长"
	}
	st, e1 := time.Parse(time.RFC3339, in.StartedAt)
	et, e2 := time.Parse(time.RFC3339, in.EndedAt)
	if e1 == nil {
		st = st.UTC()
	}
	if e2 == nil {
		et = et.UTC()
	}
	r.StartedAt, r.EndedAt = st, et
	if e1 != nil {
		r.Errors["started_at"] = "时间格式错误或缺少时区"
	}
	if e2 != nil {
		r.Errors["ended_at"] = "时间格式错误或缺少时区"
	}
	if e1 == nil && e2 == nil {
		if !et.After(st) {
			r.Errors["ended_at"] = "必须晚于 started_at"
		} else if et.Sub(st) > 24*time.Hour {
			r.Errors["ended_at"] = "采集时长超过业务上限"
		}
	}
	if !supportedRegion(r.Region) {
		r.Errors["region"] = "不支持的海域"
	}
	if len(r.Errors) == 0 {
		r.Conflicts, _ = s.Repo.FindOverlaps(ctx, r.BuoyID, st, et)
	}
	r.Valid = len(r.Errors) == 0 && len(r.Conflicts) == 0
	return r
}
func (s *Service) Create(ctx context.Context, in CreateInput, requestID string) (ObservationCase, error) {
	// 幂等指纹基于与预检/写入相同的规范化输入，避免仅空格或大小写差异被误判为冲突。
	in.BuoyID = strings.ToUpper(strings.TrimSpace(in.BuoyID))
	in.Region = normalizeRegion(in.Region)
	in.SpeciesScope = strings.ToLower(strings.Join(strings.Fields(in.SpeciesScope), " "))
	fp := repository.Fingerprint(in)
	if requestID != "" {
		if old, e := s.Repo.Idempotent(ctx, requestID, fp); e != nil {
			return ObservationCase{}, e
		} else if old != "" {
			return s.Repo.GetCase(ctx, old)
		}
	}
	var fields []string
	if n := strings.TrimSpace(in.BuoyID); n == "" || len([]rune(n)) > 64 {
		fields = append(fields, "buoy_id")
	}
	if n := in.Region; n == "" || len([]rune(n)) > 128 {
		fields = append(fields, "region")
	}
	if n := strings.TrimSpace(in.SpeciesScope); n != "" && len([]rune(n)) > 128 {
		fields = append(fields, "species_scope")
	}
	if len(fields) > 0 {
		return ObservationCase{}, fmt.Errorf("字段校验失败: %s", strings.Join(fields, ","))
	}
	st, e := time.Parse(time.RFC3339, in.StartedAt)
	if e != nil {
		return ObservationCase{}, errors.New("started_at: 时间格式错误或缺少时区")
	}
	et, e := time.Parse(time.RFC3339, in.EndedAt)
	if e != nil {
		return ObservationCase{}, errors.New("ended_at: 时间格式错误或缺少时区")
	}
	st = st.UTC()
	et = et.UTC()
	if !et.After(st) {
		return ObservationCase{}, errors.New("ended_at: 必须晚于 started_at")
	}
	if et.Sub(st) > 24*time.Hour {
		return ObservationCase{}, errors.New("ended_at: 采集时长超过业务上限")
	}
	if !supportedRegion(in.Region) {
		return ObservationCase{}, errors.New("region: 不支持的海域")
	}
	now := time.Now().UTC()
	c := ObservationCase{CaseID: fmt.Sprintf("case-%d", now.UnixNano()), BuoyID: in.BuoyID, Region: in.Region, SpeciesScope: in.SpeciesScope, StartedAt: st, EndedAt: et, Status: StatusRegistered, Revision: 1, CreatedBy: strings.TrimSpace(in.CreatedBy), CreatedAt: now}
	if e = s.Repo.SaveCaseNoOverlap(ctx, c, 0); e != nil {
		return c, e
	}
	if requestID != "" {
		_ = s.Repo.PutIdempotent(ctx, requestID, fp, c.CaseID)
	}
	_, e = s.Audit.Append(ctx, c.CaseID, requestID, "create_case", in.CreatedBy, "", string(c.Status), in)
	return c, e
}

func metadataSummary(c ObservationCase) map[string]interface{} {
	return map[string]interface{}{"buoy_id": c.BuoyID, "region": c.Region, "species_scope": c.SpeciesScope, "started_at": c.StartedAt, "ended_at": c.EndedAt, "revision": c.Revision}
}

func (s *Service) UpdateMetadata(ctx context.Context, id string, in MetadataPatchInput, expected int, requestID, actor string) (ObservationCase, error) {
	if in.BuoyID == nil && in.Region == nil && in.SpeciesScope == nil && in.StartedAt == nil && in.EndedAt == nil {
		return ObservationCase{}, errors.New("至少提供一个待更正字段")
	}
	fp := repository.Fingerprint(struct {
		ID       string
		Input    MetadataPatchInput
		Expected int
	}{id, in, expected})
	if requestID != "" {
		if old, e := s.Repo.Idempotent(ctx, requestID, fp); e != nil {
			return ObservationCase{}, e
		} else if old != "" {
			return s.Repo.GetCase(ctx, id)
		}
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return c, e
	}
	if c.Revision != expected {
		return c, repository.ErrRevision
	}
	if c.Status != StatusRegistered && c.Status != StatusResample {
		return c, errors.New("仅 registered 或 resample 状态允许更正元数据")
	}
	old := c
	ci := CreateInput{BuoyID: c.BuoyID, Region: c.Region, SpeciesScope: c.SpeciesScope, StartedAt: c.StartedAt.Format(time.RFC3339), EndedAt: c.EndedAt.Format(time.RFC3339)}
	if in.BuoyID != nil {
		ci.BuoyID = *in.BuoyID
	}
	if in.Region != nil {
		ci.Region = *in.Region
	}
	if in.SpeciesScope != nil {
		ci.SpeciesScope = *in.SpeciesScope
	}
	if in.StartedAt != nil {
		ci.StartedAt = *in.StartedAt
	}
	if in.EndedAt != nil {
		ci.EndedAt = *in.EndedAt
	}
	pre := s.Precheck(ctx, ci)
	conflicts := []repository.Overlap{}
	for _, x := range pre.Conflicts {
		if x.CaseID != id {
			conflicts = append(conflicts, x)
		}
	}
	if len(pre.Errors) > 0 {
		return c, fmt.Errorf("元数据校验失败: %v", pre.Errors)
	}
	if len(conflicts) > 0 {
		return c, errors.New("浮标与既有个案时段重叠")
	}
	species := strings.ToLower(strings.Join(strings.Fields(ci.SpeciesScope), " "))
	if len([]rune(species)) > 128 {
		return c, errors.New("species_scope: 字段过长")
	}
	c.BuoyID, c.Region, c.SpeciesScope, c.StartedAt, c.EndedAt = pre.BuoyID, pre.Region, species, pre.StartedAt, pre.EndedAt
	c.Revision++
	if e = s.Repo.SaveMetadataNoOverlap(ctx, c, expected); e != nil {
		return old, e
	}
	_, e = s.Audit.Append(ctx, id, requestID, "metadata_update", actor, string(c.Status), string(c.Status), map[string]interface{}{"before": metadataSummary(old), "after": metadataSummary(c)})
	if requestID != "" {
		_ = s.Repo.PutIdempotent(ctx, requestID, fp, id)
	}
	return c, e
}

func (s *Service) SupersedeEvidence(ctx context.Context, id string, in SupersedeInput, expected int, requestID, actor string) (ObservationCase, []string, error) {
	fp := repository.Fingerprint(struct {
		ID       string
		Input    SupersedeInput
		Expected int
	}{id, in, expected})
	if requestID != "" {
		if old, e := s.Repo.Idempotent(ctx, requestID, fp); e != nil {
			return ObservationCase{}, nil, e
		} else if old != "" {
			c, e := s.Repo.GetCase(ctx, id)
			return c, nil, e
		}
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return c, nil, e
	}
	if c.Revision != expected {
		return c, nil, repository.ErrRevision
	}
	if strings.TrimSpace(in.EvidenceID) == "" || strings.TrimSpace(in.Reason) == "" || len([]rune(in.Reason)) > 500 {
		return c, nil, errors.New("evidence_id 和有效撤回原因不能为空")
	}
	refs := s.Repo.EvidenceReferences(ctx, id, in.EvidenceID)
	if len(refs) > 0 {
		return c, refs, fmt.Errorf("证据仍被引用: %s", strings.Join(refs, ","))
	}
	if c.Status == StatusSigned || c.Status == StatusFrozen {
		return c, nil, errors.New("已签署或冻结个案不能撤回证据")
	}
	oldList, _ := s.Repo.ListEvidence(ctx, id)
	var old *CalibrationEvidence
	for i := range oldList {
		if oldList[i].EvidenceID == in.EvidenceID {
			old = &oldList[i]
			break
		}
	}
	if old == nil {
		return c, nil, repository.ErrNotFound
	}
	if old.Withdrawn {
		return c, nil, errors.New("证据已撤回")
	}
	x := in.Replacement
	if x.SensorID == "" {
		x = in.ReplacementEvidence
	}
	if x.SensorID == "" {
		x = in.Evidence
	}
	x.SensorID = strings.TrimSpace(x.SensorID)
	x.AudioDigest = strings.ToLower(strings.TrimSpace(x.AudioDigest))
	if x.SensorID == "" || x.SamplingRate < 8000 {
		return c, nil, errors.New("替代证据 sensor_id/sampling_rate 字段不完整")
	}
	if len(x.AudioDigest) != 64 {
		return c, nil, errors.New("替代证据 audio_digest 必须为64位十六进制字符串")
	}
	if _, er := hex.DecodeString(x.AudioDigest); er != nil {
		return c, nil, errors.New("替代证据 audio_digest 必须为十六进制字符串")
	}
	cal, er := time.Parse(time.RFC3339, x.CalibratedAt)
	if er != nil {
		return c, nil, errors.New("替代证据 calibrated_at 时间格式错误")
	}
	cal = cal.UTC()
	if cal.Before(c.StartedAt) || cal.After(c.EndedAt) {
		return c, nil, errors.New("替代证据 calibrated_at 必须位于观测时段内")
	}
	for _, v := range oldList {
		if !v.Withdrawn && v.EvidenceID != in.EvidenceID && v.SamplingRate != x.SamplingRate {
			return c, nil, errors.New("替代证据 sampling_rate 必须与个案一致")
		}
		if !v.Withdrawn && v.EvidenceID != in.EvidenceID && v.SensorID == x.SensorID && v.AudioDigest == x.AudioDigest {
			return c, nil, errors.New("替代证据重复")
		}
	}
	now := time.Now().UTC()
	replacement := CalibrationEvidence{EvidenceID: fmt.Sprintf("evi-%d", now.UnixNano()), CaseID: id, SensorID: x.SensorID, CalibrationRef: strings.TrimSpace(x.CalibrationRef), AudioDigest: x.AudioDigest, Operator: strings.TrimSpace(x.Operator), CalibratedAt: cal, SubmittedAt: now, SamplingRate: x.SamplingRate, SubmittedRevision: c.Revision + 1, WithdrawnReason: strings.TrimSpace(in.Reason), WithdrawnBy: actor, WithdrawnAt: &now}
	from := c.Status
	c.Status = StatusEvidence
	c.Revision++
	if e = s.Repo.SupersedeEvidence(ctx, id, in.EvidenceID, replacement, c, expected); e != nil {
		return c, nil, e
	}
	_, e = s.Audit.Append(ctx, id, requestID, "evidence_supersede", actor, string(from), string(c.Status), map[string]interface{}{"old": map[string]interface{}{"evidence_id": old.EvidenceID, "sensor_id": old.SensorID, "audio_digest": old.AudioDigest}, "new": map[string]interface{}{"evidence_id": replacement.EvidenceID, "sensor_id": replacement.SensorID, "audio_digest": replacement.AudioDigest}, "reason": in.Reason})
	if requestID != "" {
		_ = s.Repo.PutIdempotent(ctx, requestID, fp, id)
	}
	return c, nil, e
}
func (s *Service) AddEvidence(ctx context.Context, id string, in EvidenceInput, expected int, requestID string) (ObservationCase, error) {
	return s.AddEvidenceBatch(ctx, id, EvidenceBatchInput{Items: []EvidenceInput{in}}, expected, requestID)
}
func (s *Service) AddEvidenceBatch(ctx context.Context, id string, batch EvidenceBatchInput, expected int, requestID string) (ObservationCase, error) {
	fp := repository.Fingerprint(struct {
		ID       string
		Batch    EvidenceBatchInput
		Expected int
	}{id, batch, expected})
	if requestID != "" {
		if old, e := s.Repo.Idempotent(ctx, requestID, fp); e != nil {
			return ObservationCase{}, e
		} else if old != "" {
			return s.Repo.GetCase(ctx, old)
		}
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return c, e
	}
	if c.Revision != expected {
		return c, repository.ErrRevision
	}
	if c.Status != StatusRegistered && c.Status != StatusEvidence && c.Status != StatusResample {
		return c, errors.New("当前状态不允许提交证据")
	}
	if len(batch.Items) == 0 {
		return c, errors.New("证据批次不能为空")
	}
	old, _ := s.Repo.ListEvidence(ctx, id)
	rate := 0
	if len(old) > 0 {
		rate = old[0].SamplingRate
	}
	evs := make([]CalibrationEvidence, 0, len(batch.Items))
	seen := map[string]bool{}
	for i, in := range batch.Items {
		in.SensorID = strings.TrimSpace(in.SensorID)
		in.AudioDigest = strings.ToLower(strings.TrimSpace(in.AudioDigest))
		if in.SensorID == "" || in.SamplingRate < 8000 {
			return c, fmt.Errorf("items[%d].sensor_id/sampling_rate: 字段不完整", i)
		}
		if len(in.AudioDigest) != 64 {
			return c, fmt.Errorf("items[%d].audio_digest: 必须为64位十六进制字符串", i)
		}
		if _, err := hex.DecodeString(in.AudioDigest); err != nil {
			return c, fmt.Errorf("items[%d].audio_digest: 必须为十六进制字符串", i)
		}
		cal, err := time.Parse(time.RFC3339, in.CalibratedAt)
		if err != nil {
			return c, fmt.Errorf("items[%d].calibrated_at: 时间格式错误", i)
		}
		if cal.Before(c.StartedAt) || cal.After(c.EndedAt) {
			return c, fmt.Errorf("items[%d].calibrated_at: 必须位于观测时段内", i)
		}
		if rate != 0 && rate != in.SamplingRate {
			return c, fmt.Errorf("items[%d].sampling_rate: 同一个案采样率必须一致", i)
		}
		rate = in.SamplingRate
		key := in.SensorID + "\x00" + in.AudioDigest
		if seen[key] {
			return c, fmt.Errorf("items[%d]: 重复证据", i)
		}
		seen[key] = true
		for _, x := range old {
			if x.SensorID == in.SensorID && x.AudioDigest == in.AudioDigest {
				return c, fmt.Errorf("items[%d]: 重复证据", i)
			}
		}
		evs = append(evs, CalibrationEvidence{EvidenceID: fmt.Sprintf("evi-%d-%d", time.Now().UnixNano(), i), CaseID: id, SensorID: in.SensorID, CalibrationRef: strings.TrimSpace(in.CalibrationRef), AudioDigest: in.AudioDigest, Operator: strings.TrimSpace(in.Operator), SamplingRate: in.SamplingRate, CalibratedAt: cal, SubmittedAt: time.Now().UTC(), SubmittedRevision: c.Revision + 1})
	}
	from := c.Status
	c.Status = StatusEvidence
	c.Revision++
	if e = s.Repo.SaveEvidenceAndCase(ctx, id, evs, c, expected); e != nil {
		return c, e
	}
	actor := ""
	if len(batch.Items) > 0 {
		actor = batch.Items[0].Operator
	}
	_, e = s.Audit.Append(ctx, id, requestID, "submit_evidence", actor, string(from), string(c.Status), batch)
	if requestID != "" {
		_ = s.Repo.PutIdempotent(ctx, requestID, fp, id)
	}
	return c, e
}
func (s *Service) Get(ctx context.Context, id string) (ObservationCase, error) {
	return s.Repo.GetCase(ctx, id)
}
func (s *Service) List(ctx context.Context) ([]ObservationCase, error) { return s.Repo.ListCases(ctx) }
func (s *Service) Query(ctx context.Context, f repository.CaseFilter) (repository.CasePage, error) {
	return s.Repo.QueryCases(ctx, f)
}
func (s *Service) Evidence(ctx context.Context, id string) ([]CalibrationEvidence, error) {
	return s.Repo.ListEvidence(ctx, id)
}
func (s *Service) AddVerificationNote(ctx context.Context, id, note, actor, requestID string, expected int) (ObservationCase, error) {
	fp := repository.Fingerprint(struct {
		ID, Note, Actor string
		Expected        int
	}{id, note, actor, expected})
	if requestID != "" {
		if old, e := s.Repo.Idempotent(ctx, requestID, fp); e != nil {
			return ObservationCase{}, e
		} else if old != "" {
			return s.Repo.GetCase(ctx, id)
		}
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return c, e
	}
	if c.Revision != expected {
		return c, repository.ErrRevision
	}
	if c.Status != StatusRegistered && c.Status != StatusEvidence && c.Status != StatusResample {
		return c, errors.New("当前状态不允许补充核对备注")
	}
	if strings.TrimSpace(note) == "" || len([]rune(note)) > 1000 {
		return c, errors.New("核对备注不能为空或过长")
	}
	c.VerificationNotes = append(c.VerificationNotes, strings.TrimSpace(note))
	c.Revision++
	if e = s.Repo.SaveCase(ctx, c, expected); e != nil {
		return c, e
	}
	_, e = s.Audit.Append(ctx, id, requestID, "evidence_note", actor, string(c.Status), string(c.Status), map[string]string{"note": note})
	if requestID != "" {
		_ = s.Repo.PutIdempotent(ctx, requestID, fp, id)
	}
	return c, e
}
func (s *Service) EvidenceReport(ctx context.Context, id string, sensorID string, evidenceID string) (EvidenceReport, interface{}, error) {
	return s.EvidenceReportCoverage(ctx, id, sensorID, evidenceID, false)
}
func (s *Service) EvidenceReportCoverage(ctx context.Context, id string, sensorID string, evidenceID string, coverage bool) (EvidenceReport, interface{}, error) {
	if sensorID == "" && evidenceID == "" {
		cacheKey := fmt.Sprintf("%s:%t", id, coverage)
		s.reportCacheMu.RLock()
		cached, ok := s.reportCache[cacheKey]
		s.reportCacheMu.RUnlock()
		if ok {
			return cloneEvidenceReport(cached), nil, nil
		}
	}
	c, e := s.Repo.GetCase(ctx, id)
	if e != nil {
		return EvidenceReport{}, nil, e
	}
	evs, _ := s.Repo.ListEvidence(ctx, id)
	if evidenceID != "" {
		for _, ev := range evs {
			if ev.EvidenceID == evidenceID {
				return EvidenceReport{CaseID: id, Revision: c.Revision}, ev, nil
			}
		}
		return EvidenceReport{}, nil, repository.ErrNotFound
	}
	if sensorID != "" {
		for _, ev := range evs {
			if ev.SensorID == sensorID {
				return EvidenceReport{CaseID: id, Revision: c.Revision}, ev, nil
			}
		}
		return EvidenceReport{}, nil, repository.ErrNotFound
	}
	active := evs[:0]
	for _, ev := range evs {
		if !ev.Withdrawn {
			active = append(active, ev)
		}
	}
	evs = active
	r := EvidenceReport{CaseID: id, Revision: c.Revision, Sensors: []SensorEvidenceSummary{}, Differences: []EvidenceDifference{}, Gaps: []EvidenceGap{}, Score: 100}
	groups := map[string]*SensorEvidenceSummary{}
	resolved := map[string]map[string]bool{}
	rates := map[int]bool{}
	revisions := map[int]bool{}
	for _, ev := range evs {
		if resolved[ev.SensorID] == nil {
			resolved[ev.SensorID] = map[string]bool{}
		}
		if ev.SubmittedRevision > r.LatestRevision {
			r.LatestRevision = ev.SubmittedRevision
		}
		revisions[ev.SubmittedRevision] = true
		g := groups[ev.SensorID]
		if g == nil {
			g = &SensorEvidenceSummary{SensorID: ev.SensorID, Earliest: ev.CalibratedAt, Latest: ev.CalibratedAt}
			groups[ev.SensorID] = g
		}
		g.Count++
		if ev.CalibratedAt.Before(g.Earliest) {
			g.Earliest = ev.CalibratedAt
		}
		if ev.CalibratedAt.After(g.Latest) {
			g.Latest = ev.CalibratedAt
		}
		rates[ev.SamplingRate] = true
		found := false
		for _, x := range g.SamplingRates {
			if x == ev.SamplingRate {
				found = true
			}
		}
		if !found {
			g.SamplingRates = append(g.SamplingRates, ev.SamplingRate)
		}
		if strings.TrimSpace(ev.CalibrationRef) == "" {
			r.Differences = append(r.Differences, EvidenceDifference{Code: "missing_calibration_ref", EvidenceID: ev.EvidenceID, SensorID: ev.SensorID, Detail: "缺少校准引用"})
			r.Gaps = append(r.Gaps, EvidenceGap{Code: "missing_calibration_ref", EvidenceID: ev.EvidenceID, SensorID: ev.SensorID, Blocking: true, Detail: "缺少校准引用"})
			r.Score -= 20
			r.Blocking = true
		} else {
			resolved[ev.SensorID]["missing_calibration_ref"] = true
		}
		if len(ev.AudioDigest) != 64 {
			r.Differences = append(r.Differences, EvidenceDifference{Code: "invalid_digest", EvidenceID: ev.EvidenceID, SensorID: ev.SensorID, Detail: "摘要格式异常"})
			r.Gaps = append(r.Gaps, EvidenceGap{Code: "invalid_digest", EvidenceID: ev.EvidenceID, SensorID: ev.SensorID, Blocking: true, Detail: "摘要格式异常"})
			r.Score -= 20
			r.Blocking = true
		} else if _, err := hex.DecodeString(ev.AudioDigest); err != nil {
			r.Gaps = append(r.Gaps, EvidenceGap{Code: "invalid_digest", EvidenceID: ev.EvidenceID, SensorID: ev.SensorID, Blocking: true, Detail: "摘要非十六进制"})
			r.Score -= 20
			r.Blocking = true
		} else {
			resolved[ev.SensorID]["invalid_digest"] = true
		}
		if strings.TrimSpace(ev.Operator) == "" {
			r.Gaps = append(r.Gaps, EvidenceGap{Code: "missing_operator", EvidenceID: ev.EvidenceID, SensorID: ev.SensorID, Blocking: true, Detail: "缺少操作员"})
			r.Score -= 10
			r.Blocking = true
		} else {
			resolved[ev.SensorID]["missing_operator"] = true
		}
	}
	// 补交记录不会覆盖历史证据；对完整性判定则以同一传感器最近一次
	// 已补齐的字段为准，使补证据后缺口可以真正关闭。
	if len(resolved) > 0 {
		filteredGaps := r.Gaps[:0]
		for _, g := range r.Gaps {
			if g.SensorID != "" && resolved[g.SensorID][g.Code] {
				continue
			}
			filteredGaps = append(filteredGaps, g)
		}
		r.Gaps = filteredGaps
		filteredDiffs := r.Differences[:0]
		for _, d := range r.Differences {
			if d.SensorID != "" && resolved[d.SensorID][d.Code] {
				continue
			}
			filteredDiffs = append(filteredDiffs, d)
		}
		r.Differences = filteredDiffs
		r.Blocking = false
		for _, g := range r.Gaps {
			if g.Blocking {
				r.Blocking = true
				break
			}
		}
		r.Score = 100
		for _, g := range r.Gaps {
			switch g.Code {
			case "missing_calibration_ref", "invalid_digest":
				r.Score -= 20
			case "missing_operator":
				r.Score -= 10
			}
		}
	}
	for _, g := range groups {
		sort.Ints(g.SamplingRates)
		r.Sensors = append(r.Sensors, *g)
	}
	sort.Slice(r.Sensors, func(i, j int) bool { return r.Sensors[i].SensorID < r.Sensors[j].SensorID })
	if len(rates) > 1 {
		r.Differences = append(r.Differences, EvidenceDifference{Code: "sampling_rate_mismatch", Detail: "个案存在多个采样率"})
		r.Gaps = append(r.Gaps, EvidenceGap{Code: "sampling_rate_mismatch", Blocking: true, Detail: "采样率不一致"})
		r.Score -= 20
		r.Blocking = true
	}
	if len(evs) == 0 {
		r.Score = 0
		r.Gaps = append(r.Gaps, EvidenceGap{Code: "missing_evidence", Blocking: true, Detail: "缺少证据"})
		r.Blocking = true
	}
	if r.Score < 0 {
		r.Score = 0
	}
	r.IntegrityScore = r.Score
	for rev := range revisions {
		if rev < r.Revision && rev > r.PreviousRevision {
			r.PreviousRevision = rev
		}
	}
	if coverage {
		sorted, _ := s.Repo.ListEvidenceSorted(ctx, id)
		bySensor := map[string][]CalibrationEvidence{}
		lastSubmitted := map[string]time.Time{}
		for _, ev := range sorted {
			if ev.Withdrawn {
				continue
			}
			bySensor[ev.SensorID] = append(bySensor[ev.SensorID], ev)
		}
		// 逆序按原提交顺序检测，排序后的快照用于稳定区间与重复时间判断。
		for _, ev := range evs {
			if p, ok := lastSubmitted[ev.SensorID]; ok && ev.CalibratedAt.Before(p) {
				r.Gaps = append(r.Gaps, EvidenceGap{Code: "calibration_submission_out_of_order", EvidenceID: ev.EvidenceID, SensorID: ev.SensorID, Blocking: true, Detail: "校准时间提交顺序逆序"})
				r.Blocking = true
			}
			lastSubmitted[ev.SensorID] = ev.CalibratedAt
		}
		total := c.EndedAt.Sub(c.StartedAt)
		ratioSum := float64(0)
		ids := make([]string, 0, len(bySensor))
		for sensor := range bySensor {
			ids = append(ids, sensor)
		}
		sort.Strings(ids)
		for _, sensor := range ids {
			xs := bySensor[sensor]
			warnings := []string{}
			for i := 1; i < len(xs); i++ {
				if xs[i].CalibratedAt.Equal(xs[i-1].CalibratedAt) {
					warnings = append(warnings, "duplicate_calibration_time")
					r.Gaps = append(r.Gaps, EvidenceGap{Code: "duplicate_calibration_time", EvidenceID: xs[i].EvidenceID, SensorID: sensor, Blocking: true, Detail: "同一传感器存在重复校准时间"})
					r.Blocking = true
				}
			}
			start, end := xs[0].CalibratedAt, xs[len(xs)-1].CalibratedAt
			ratio := float64(0)
			if total > 0 {
				ratio = float64(end.Sub(start)) / float64(total)
			}
			threshold := s.CoverageThreshold
			if threshold <= 0 || threshold > 1 {
				threshold = 0.5
			}
			if ratio < threshold {
				warnings = append(warnings, "low_calibration_coverage")
				r.Gaps = append(r.Gaps, EvidenceGap{Code: "low_calibration_coverage", SensorID: sensor, Blocking: true, Detail: fmt.Sprintf("校准覆盖比例低于 %.0f%%", threshold*100)})
				r.Blocking = true
			}
			ratioSum += ratio
			r.Coverage = append(r.Coverage, SensorCoverage{SensorID: sensor, StartedAt: start, EndedAt: end, DurationSeconds: int64(end.Sub(start).Seconds()), Ratio: ratio, Warnings: warnings})
		}
		if len(ids) > 0 {
			r.CoverageScore = int(ratioSum / float64(len(ids)) * 100)
		}
	}
	cacheKey := fmt.Sprintf("%s:%t", id, coverage)
	s.reportCacheMu.Lock()
	s.reportCache[cacheKey] = cloneEvidenceReport(r)
	s.reportCacheMu.Unlock()
	return r, nil, nil
}
func normalizeRegion(v string) string {
	v = strings.TrimSpace(v)
	for _, r := range []string{"黄海", "渤海", "东海", "南海"} {
		if strings.EqualFold(v, r) {
			return r
		}
	}
	return v
}
func supportedRegion(v string) bool {
	return v == "黄海" || v == "渤海" || v == "东海" || v == "南海"
}
