package checkpoint

import (
	"encoding/json/v2"
	"testing"

	"github.com/timbrinded/girdle/internal/jev"
)

// The decision log is read by tools that expect a Jev call's fields at the
// top level of each decision, as they were before Call was shared.
func TestCallFieldsAreFlat(t *testing.T) {
	d := Decision{
		Checkpoint: "turn_end",
		Answers:    map[string]jev.Answer{"status": {Type: "choice", Choice: "done"}}, JevModel: "jev-1.13.0", LatencyMS: 7, InputTokens: 11,
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"checkpoint", "answers", "jev_model", "latency_ms", "input_tokens"} {
		if _, ok := m[k]; !ok {
			t.Errorf("%s missing from %s", k, data)
		}
	}
	if _, ok := m["Call"]; ok {
		t.Errorf("Call nested in %s", data)
	}
}

func TestAskWithoutClient(t *testing.T) {
	call, ok := ask(t.Context(), nil, nil, nil)
	if ok || call.Error == "" {
		t.Errorf("ask with no client = %+v, %v; want a recorded failure", call, ok)
	}
}
