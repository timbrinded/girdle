package tui

import (
	"cmp"
	"slices"
	"strings"
)

// A candidate is something the picker can find: a model's ID and name.
type candidate struct {
	id, name string
	// yours puts the user's own models ahead of equal matches, then created
	// puts newer models ahead of older.
	yours   bool
	created int64
}

// search ranks the candidates that match every word of query, best first.
// A word matches the ID or name as a substring, best at the start of a
// part such as "claude" in "anthropic/claude-opus", or as a scattered
// subsequence, so "musespk" finds "muse-spark". Ties go to the user's own
// models, then to newer models, then to shorter IDs, so a base model comes
// before its variants.
func search(query string, cands []candidate) []candidate {
	words := strings.Fields(strings.ToLower(query))
	type scored struct {
		candidate
		score int
	}
	var found []scored
	for _, c := range cands {
		id, text := strings.ToLower(c.id), strings.ToLower(c.id+" "+c.name)
		total := 0
		for _, w := range words {
			// A match in the ID counts for more than one only in the name.
			s := max(2*wordScore(w, id), wordScore(w, text))
			if s == 0 {
				total = 0
				break
			}
			total += s
		}
		if total > 0 {
			found = append(found, scored{c, total})
		}
	}
	slices.SortStableFunc(found, func(a, b scored) int {
		if a.score != b.score {
			return b.score - a.score
		}
		if a.yours != b.yours {
			if a.yours {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(b.created, a.created), len(a.id)-len(b.id), strings.Compare(a.id, b.id))
	})
	out := make([]candidate, len(found))
	for i, f := range found {
		out[i] = f.candidate
	}
	return out
}

// wordScore scores one query word against text: 4 for a substring at the
// start of a part, 3 for one elsewhere, 1 for a subsequence spread over no
// more than twice the word's length, 0 for no match.
func wordScore(w, text string) int {
	best := 0
	for i := 0; ; {
		j := strings.Index(text[i:], w)
		if j < 0 {
			break
		}
		at := i + j
		if at == 0 || strings.ContainsRune("/-_.: ", rune(text[at-1])) {
			return 4
		}
		best = 3
		i = at + 1
	}
	if best > 0 || len(w) < 3 {
		return best
	}
	// Each subsequence starts at an occurrence of the word's first letter;
	// the tightest wins.
	for start := range len(text) {
		if text[start] != w[0] {
			continue
		}
		k, end := 1, start+1
		for ; end < len(text) && k < len(w); end++ {
			if text[end] == w[k] {
				k++
			}
		}
		if k == len(w) && end-start <= 2*len(w) {
			return 1
		}
	}
	return 0
}
