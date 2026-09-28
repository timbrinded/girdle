package models

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/timbrinded/girdle/internal/checkpoint"
)

// listing is a trimmed copy of OpenRouter's /api/v1/models response.
const listing = `{"data": [
  {"id": "stealth/space-bunny-alpha", "name": "Space Bunny Alpha", "context_length": 1000000,
   "pricing": {"prompt": "0", "completion": "0"},
   "supported_parameters": ["include_reasoning", "reasoning", "reasoning_effort", "tool_choice", "tools"],
   "reasoning": {"mandatory": true, "supported_efforts": ["max", "xhigh", "high", "medium", "low"], "default_effort": "max"},
   "expiration_date": null},
  {"id": "meta/muse-spark-1.3-contributor", "name": "Meta: Muse Spark 1.3", "context_length": 262144,
   "pricing": {"prompt": "0.0000001", "completion": "0.0000002"},
   "supported_parameters": ["reasoning", "tools"],
   "reasoning": {"mandatory": true, "supported_efforts": ["ultra", "high", "none"]}},
  {"id": "cohere/command-a-plus", "name": "Cohere: Command A+", "context_length": 256000,
   "pricing": {"prompt": "0.0000025", "completion": "0.00001"},
   "supported_parameters": ["tools"], "reasoning": {"mandatory": false}},
  {"id": "openrouter/auto", "name": "Auto Router", "pricing": {"prompt": "-1", "completion": "-1"},
   "supported_parameters": ["reasoning", "reasoning_effort", "tools"]},
  {"id": "qwen/qwen3-max-thinking", "name": "Qwen3 Max Thinking", "pricing": {"prompt": "0.000001", "completion": "0.000005"},
   "supported_parameters": ["max_tokens"], "expiration_date": "2026-10-09"}
]}`

func TestFetchReadsTheCatalogue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "girdle" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(listing))
	}))
	defer srv.Close()
	c, err := Fetch(t.Context(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Models) != 5 || c.Fetched.IsZero() {
		t.Fatalf("catalogue %+v", c)
	}
	bunny, _ := c.Lookup("stealth/space-bunny-alpha")
	if !bunny.Tools || bunny.Input != 0 || bunny.Context != 1000000 || bunny.Expires != "" {
		t.Errorf("bunny %+v", bunny)
	}
	if want := []checkpoint.Effort{"low", "medium", "high", "xhigh", "max"}; !slices.Equal(bunny.Efforts, want) {
		t.Errorf("bunny efforts %v, want %v lowest first", bunny.Efforts, want)
	}
	// Efforts Girdle doesn't know are dropped.
	muse, _ := c.Lookup("meta/muse-spark-1.3-contributor")
	if !slices.Equal(muse.Efforts, []checkpoint.Effort{"none", "high"}) || muse.Input != 0.1 || muse.Output != 0.2 {
		t.Errorf("muse %+v", muse)
	}
	if cohere, _ := c.Lookup("cohere/command-a-plus"); len(cohere.Efforts) != 0 {
		t.Errorf("a model without effort control has efforts %v", cohere.Efforts)
	}
	if auto, _ := c.Lookup("openrouter/auto"); !slices.Equal(auto.Efforts, checkpoint.Efforts) || auto.Input != -1 {
		t.Errorf("router %+v", auto)
	}
	if q, _ := c.Lookup("qwen/qwen3-max-thinking"); q.Tools || q.Expires != "2026-10-09" {
		t.Errorf("qwen %+v", q)
	}
	if !c.Gone("nobody/nothing") || c.Gone("openrouter/auto") {
		t.Error("Gone disagrees with the listing")
	}
	if !slices.Equal(c.Efforts("nobody/nothing"), checkpoint.Efforts) {
		t.Error("an unknown model should accept every effort")
	}
}

func TestFetchReportsAFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer srv.Close()
	if _, err := Fetch(t.Context(), srv.Client(), srv.URL); err == nil {
		t.Fatal("a 502 was accepted")
	}
}
