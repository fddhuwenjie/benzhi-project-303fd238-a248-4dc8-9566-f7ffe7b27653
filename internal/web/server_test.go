package web

import (
	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/observation"
	"github.com/benzhi/chao-sheng/internal/quality"
	"github.com/benzhi/chao-sheng/internal/repository"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateCase(t *testing.T) {
	r, _ := repository.Open(":memory:")
	a := audit.New(r)
	h := New(observation.New(r, a), quality.New(r, a), a, r).Handler()
	req := httptest.NewRequest("POST", "/api/v1/cases", strings.NewReader(`{"buoy_id":"B1","region":"渤海","started_at":"2025-01-01T00:00:00Z","ended_at":"2025-01-01T01:00:00Z","created_by":"u"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("status %d", rec.Code)
	}
}
