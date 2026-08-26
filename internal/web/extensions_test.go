package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/observation"
	"github.com/benzhi/chao-sheng/internal/quality"
	"github.com/benzhi/chao-sheng/internal/repository"
	"net/http"
	"net/http/httptest"
	"testing"
)

func extensionRequest(t *testing.T, h http.Handler, method, path string, value interface{}, headers map[string]string, want int) map[string]interface{} {
	t.Helper()
	var payload []byte
	if value != nil {
		payload, _ = json.Marshal(value)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s: status=%d body=%s", method, path, rec.Code, rec.Body.String())
	}
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

func TestExtendedObservationWorkflow(t *testing.T) {
	repo, _ := repository.Open(":memory:")
	au := audit.New(repo)
	h := New(observation.New(repo, au), quality.New(repo, au), au, repo).Handler()
	c := extensionRequest(t, h, http.MethodPost, "/api/v1/cases", map[string]interface{}{
		"buoy_id": "b-ext", "region": "东海", "species_scope": "WHALE", "started_at": "2026-01-01T00:00:00+08:00", "ended_at": "2026-01-01T04:00:00+08:00", "created_by": "observer",
	}, map[string]string{"X-Request-ID": "ext-create"}, http.StatusCreated)
	id := c["case_id"].(string)
	c = extensionRequest(t, h, http.MethodPatch, "/api/v1/cases/"+id, map[string]interface{}{"region": "黄海"}, map[string]string{"X-Expected-Revision": "1", "X-Request-ID": "ext-patch", "X-Actor": "observer"}, http.StatusOK)
	if c["region"] != "黄海" {
		t.Fatalf("metadata was not updated: %#v", c)
	}
	digest1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	c = extensionRequest(t, h, http.MethodPost, "/api/v1/cases/"+id+"/evidence", map[string]interface{}{"sensor_id": "S1", "calibration_ref": "CAL-1", "audio_digest": digest1, "operator": "operator", "calibrated_at": "2025-12-31T16:00:00Z", "sampling_rate": 48000}, map[string]string{"X-Expected-Revision": "2", "X-Request-ID": "ext-evidence"}, http.StatusOK)
	digest2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	c = extensionRequest(t, h, http.MethodPost, "/api/v1/cases/"+id+"/evidence/supersede", map[string]interface{}{"evidence_id": "missing", "reason": "误传", "replacement": map[string]interface{}{"sensor_id": "S1", "calibration_ref": "CAL-2", "audio_digest": digest2, "operator": "operator", "calibrated_at": "2025-12-31T19:30:00Z", "sampling_rate": 48000}}, map[string]string{"X-Expected-Revision": "3", "X-Request-ID": "ext-supersede-missing", "X-Actor": "operator"}, http.StatusNotFound)
	detail := extensionRequest(t, h, http.MethodGet, "/api/v1/cases/"+id, nil, nil, http.StatusOK)
	evidence := detail["evidence"].([]interface{})
	evidenceID := evidence[0].(map[string]interface{})["evidence_id"].(string)
	c = extensionRequest(t, h, http.MethodPost, "/api/v1/cases/"+id+"/evidence/supersede", map[string]interface{}{"evidence_id": evidenceID, "reason": "原摘要上传错误", "replacement": map[string]interface{}{"sensor_id": "S1", "calibration_ref": "CAL-2", "audio_digest": digest2, "operator": "operator", "calibrated_at": "2025-12-31T19:30:00Z", "sampling_rate": 48000}}, map[string]string{"X-Expected-Revision": "3", "X-Request-ID": "ext-supersede", "X-Actor": "operator"}, http.StatusOK)
	report := extensionRequest(t, h, http.MethodGet, "/api/v1/cases/"+id+"/evidence/report?coverage=1", nil, nil, http.StatusOK)
	if report["blocking"] != true {
		t.Fatalf("single calibration should have low coverage: %#v", report)
	}
	review := extensionRequest(t, h, http.MethodPost, "/api/v1/cases/"+id+"/screen", map[string]string{"rule_profile": "profile-v2"}, map[string]string{"X-Expected-Revision": "4", "X-Request-ID": "ext-screen", "X-Actor": "qc-a"}, http.StatusOK)
	if review["profile_id"] != "profile-v2" {
		t.Fatalf("profile not persisted: %#v", review)
	}
	extensionRequest(t, h, http.MethodPost, "/api/v1/cases/"+id+"/review/claim", nil, map[string]string{"X-Expected-Revision": "5", "X-Request-ID": "ext-claim-a", "X-Actor": "qc-a"}, http.StatusOK)
	extensionRequest(t, h, http.MethodPost, "/api/v1/cases/"+id+"/review/claim", nil, map[string]string{"X-Expected-Revision": "5", "X-Request-ID": "ext-claim-b", "X-Actor": "qc-b"}, http.StatusConflict)
	extensionRequest(t, h, http.MethodPost, "/api/v1/cases/"+id+"/review", map[string]interface{}{"decision": "approve", "reviewer": "qc-a"}, map[string]string{"X-Expected-Revision": "5", "X-Request-ID": "ext-review"}, http.StatusOK)
	ready := extensionRequest(t, h, http.MethodGet, "/api/v1/cases/"+id+"/sign-readiness?reviewer=auditor", nil, nil, http.StatusOK)
	token := ready["declaration_token"].(string)
	extensionRequest(t, h, http.MethodPost, "/api/v1/cases/"+id+"/sign", map[string]interface{}{"grade": "A", "reviewer": "auditor", "token": token, "declaration_text": fmt.Sprintf("本人确认当前证据与清单，确认码 %s。", token)}, map[string]string{"X-Expected-Revision": "6", "X-Request-ID": "ext-sign"}, http.StatusOK)
	preview := extensionRequest(t, h, http.MethodGet, "/api/v1/cases/"+id+"/preview", nil, nil, http.StatusOK)
	extensionRequest(t, h, http.MethodPost, "/api/v1/cases/"+id+"/freeze", map[string]interface{}{"preview_digest": preview["root_digest"], "part_digests": preview["part_digests"]}, map[string]string{"X-Expected-Revision": "7", "X-Request-ID": "ext-freeze", "X-Actor": "publisher"}, http.StatusOK)
	extensionRequest(t, h, http.MethodGet, "/api/v1/cases/"+id+"/manifest?part=evidence&root_digest="+preview["root_digest"].(string), nil, nil, http.StatusOK)
	audits := extensionRequest(t, h, http.MethodGet, "/api/v1/cases/"+id+"/audit?request_id=ext-patch", nil, nil, http.StatusOK)
	if audits["count"].(float64) != 1 {
		t.Fatalf("request audit filter failed: %#v", audits)
	}
}
