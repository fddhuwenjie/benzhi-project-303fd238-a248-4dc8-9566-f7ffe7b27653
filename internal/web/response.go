package web

import (
	"encoding/json"
	"net/http"
)

func write(w http.ResponseWriter, v interface{}, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func body(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
