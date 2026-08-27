package staleevidencereportcache_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/observation"
	"github.com/benzhi/chao-sheng/internal/quality"
	"github.com/benzhi/chao-sheng/internal/repository"
	"github.com/benzhi/chao-sheng/internal/web"
)

func request(t *testing.T, handler http.Handler, method, path string, value interface{}, headers map[string]string, wantStatus int) map[string]interface{} {
	t.Helper()
	var payload []byte
	if value != nil {
		var err error
		payload, err = json.Marshal(value)
		if err != nil {
			t.Fatalf("编码请求失败: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s 状态码=%d，响应=%s", method, path, rec.Code, rec.Body.String())
	}
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	return result
}

func TestEvidenceReportRefreshesAfterEvidenceWrite(t *testing.T) {
	repo, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("打开仓储失败: %v", err)
	}
	auditService := audit.New(repo)
	handler := web.New(observation.New(repo, auditService), quality.New(repo, auditService), auditService, repo).Handler()

	created := request(t, handler, http.MethodPost, "/api/v1/cases", map[string]interface{}{
		"buoy_id":       "cache-edge-buoy",
		"region":        "东海",
		"species_scope": "鲸类",
		"started_at":    "2026-08-20T00:00:00Z",
		"ended_at":      "2026-08-20T02:00:00Z",
		"created_by":    "observer-a",
	}, map[string]string{"X-Request-ID": "cache-create"}, http.StatusCreated)
	caseID := created["case_id"].(string)

	before := request(t, handler, http.MethodGet, "/api/v1/cases/"+caseID+"/evidence/report", nil, nil, http.StatusOK)
	if before["revision"] != float64(1) || before["score"] != float64(0) {
		t.Fatalf("初始空报告不符合前置条件: %#v", before)
	}

	request(t, handler, http.MethodPost, "/api/v1/cases/"+caseID+"/evidence", map[string]interface{}{
		"sensor_id":       "sensor-cache-1",
		"calibration_ref": "CAL-CACHE-1",
		"audio_digest":    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"sampling_rate":   48000,
		"operator":        "operator-a",
		"calibrated_at":   "2026-08-20T01:00:00Z",
	}, map[string]string{"X-Expected-Revision": "1", "X-Request-ID": "cache-evidence"}, http.StatusOK)

	after := request(t, handler, http.MethodGet, "/api/v1/cases/"+caseID+"/evidence/report", nil, nil, http.StatusOK)
	sensors, _ := after["sensors"].([]interface{})
	if after["revision"] != float64(2) || after["score"] != float64(100) || len(sensors) != 1 {
		t.Fatalf("写入证据后仍返回旧缓存报告: revision=%v score=%v sensors=%d", after["revision"], after["score"], len(sensors))
	}
}
