package web

import (
	"encoding/json"
	"net/http"

	"github.com/benzhi/chao-sheng/internal/repository"
)

func write(w http.ResponseWriter, v interface{}, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func body(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// errorStatus maps a service/repository error to the HTTP status code that
// should be used when base would otherwise apply. Persistence errors are
// elevated to 503 because they reflect a server-side storage failure, not a
// malformed client request; all other errors keep the caller-provided base.
func errorStatus(err error, base int) int {
	if repository.IsPersistenceError(err) {
		return http.StatusServiceUnavailable
	}
	return base
}

