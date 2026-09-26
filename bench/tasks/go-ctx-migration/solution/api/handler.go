// Package api serves the store over HTTP.
package api

import (
	"errors"
	"io"
	"net/http"

	"example.com/kv/store"
)

// Handler returns an HTTP handler for GET and PUT /kv/{key}.
func Handler(s *store.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /kv/{key}", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Get(r.Context(), r.PathValue("key"))
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		io.WriteString(w, v)
	})
	mux.HandleFunc("PUT /kv/{key}", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Put(r.Context(), r.PathValue("key"), string(body)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}
