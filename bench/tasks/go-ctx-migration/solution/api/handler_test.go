package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/kv/store"
)

func TestPutThenGet(t *testing.T) {
	h := Handler(store.New())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/kv/a", strings.NewReader("hello")))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kv/a", nil))
	if rec.Body.String() != "hello" {
		t.Fatalf("GET body %q", rec.Body.String())
	}
}
