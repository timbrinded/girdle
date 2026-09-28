package tui

import (
	"slices"
	"testing"
)

func ids(cs []candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.id
	}
	return out
}

func TestSearchRanksWholeWordsFirst(t *testing.T) {
	cands := []candidate{
		{id: "mistralai/mistral-small-2603", name: "Mistral: Mistral Small 4"},
		{id: "anthropic/claude-opus-4.5", name: "Anthropic: Claude Opus 4.5"},
		{id: "anthropic/claude-opus-5.5", name: "Anthropic: Claude Opus 5.5"},
		{id: "meta/muse-spark-1.3-contributor", name: "Meta: Muse Spark 1.3 (Contributor)"},
		{id: "meta/muse-spark-1.3", name: "Meta: Muse Spark 1.3"},
		{id: "stealth/space-bunny-alpha", name: "Space Bunny Alpha", yours: true},
	}
	cases := []struct {
		query string
		want  []string
	}{
		// Every word must match; "4" alone would match Mistral's name.
		{"opus 4", []string{"anthropic/claude-opus-4.5"}},
		{"opus", []string{"anthropic/claude-opus-4.5", "anthropic/claude-opus-5.5"}},
		// Scattered letters still find a model.
		{"musespk", []string{"meta/muse-spark-1.3", "meta/muse-spark-1.3-contributor"}},
		{"bunny", []string{"stealth/space-bunny-alpha"}},
		{"MUSE Contributor", []string{"meta/muse-spark-1.3-contributor"}},
		{"zzz", nil},
	}
	for _, c := range cases {
		if got := ids(search(c.query, cands)); !slices.Equal(got, c.want) {
			t.Errorf("search(%q) = %v, want %v", c.query, got, c.want)
		}
	}
}

func TestSearchBreaksTies(t *testing.T) {
	cands := []candidate{
		{id: "b/model-x", created: 3},
		{id: "a/model-x", yours: true, created: 1},
		{id: "c/model-x:batch", created: 2},
		{id: "c/model-x", created: 2},
	}
	// Yours first, then newest, then the base model before its variant.
	want := []string{"a/model-x", "b/model-x", "c/model-x", "c/model-x:batch"}
	if got := ids(search("model", cands)); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWordScore(t *testing.T) {
	for _, c := range []struct {
		w, text string
		want    int
	}{
		{"claude", "anthropic/claude-opus", 4},
		{"laude", "anthropic/claude-opus", 3},
		{"cop", "anthropic/claude-opus", 0}, // too spread out
		{"clop", "anthropic/claude-opus", 0},
		{"clo", "anthropic/clo-x", 4},
		{"ms", "meta/muse", 0}, // short words must be substrings
	} {
		if got := wordScore(c.w, c.text); got != c.want {
			t.Errorf("wordScore(%q, %q) = %d, want %d", c.w, c.text, got, c.want)
		}
	}
}
