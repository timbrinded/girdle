package jev

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestLongRetryAfterGivesUpAtOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "35824")
		http.Error(w, "quota exhausted", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(srv.URL, "jev-test", "key")
	start := time.Now()
	_, err := c.Ask(t.Context(), map[string]string{"x": "y"}, map[string]Question{"q": Noul("Is `x` y?")})
	if err == nil {
		t.Fatal("no error for an exhausted quota")
	}
	if took := time.Since(start); took > 2*time.Second || calls.Load() != 1 {
		t.Fatalf("took %s over %d calls; it should give up at once", took, calls.Load())
	}
}

func TestShortRetryAfterRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"model":"jev-test","answers":{"q":{"type":"noul","noul":0.9}}}`))
	}))
	defer srv.Close()
	res, err := New(srv.URL, "jev-test", "key").Ask(t.Context(), map[string]string{"x": "y"}, map[string]Question{"q": Noul("Is `x` y?")})
	if err != nil || res.Answers["q"].Noul != 0.9 || calls.Load() != 2 {
		t.Fatalf("res %v, err %v, calls %d", res, err, calls.Load())
	}
}
