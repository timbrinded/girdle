package checkpoint

import (
	"context"
	"fmt"
	"time"

	"github.com/timbrinded/girdle/internal/jev"
)

// Leftovers catch a rename or a removal that isn't finished. In the
// benchmark logs every failed rename left the old name behind, in a comment
// or a string message, and every passing one left none (decision 0014).
// Jev judges what the request means: which names should be gone, which
// files should lose their unused code, and which remaining mentions really
// must change. Code finds the mentions.

// Intent is what the request asks to disappear.
type Intent struct {
	// Gone are the names the request wants renamed, replaced or removed.
	Gone []string
	// Prune are the files whose unused code the request wants removed.
	Prune     []string
	Answers   map[string]jev.Answer
	LatencyMS int64
	Tokens    int64
}

// AskIntent asks, for each name and file the request mentions, whether it
// should be gone by the end. One request, one question per item.
func AskIntent(ctx context.Context, c *jev.Client, task string, names, files []string) Intent {
	var in Intent
	if c == nil || len(names)+len(files) == 0 {
		return in
	}
	qs := map[string]jev.Question{}
	for i := range names {
		qs[fmt.Sprintf("gone_%d", i)] = jev.Noul(fmt.Sprintf("Does `task` ask for `names[%d]` to be renamed, replaced or removed, so that it no longer appears in the repository?", i))
	}
	for i := range files {
		qs[fmt.Sprintf("prune_%d", i)] = jev.Noul(fmt.Sprintf("Does `task` ask to remove the code in `files[%d]` that nothing else uses?", i))
	}
	start := time.Now()
	res, err := c.Ask(ctx, map[string]any{"task": task, "names": names, "files": files}, qs)
	in.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		return in
	}
	in.Answers, in.Tokens = res.Answers, res.Usage.InputTokens
	for i, n := range names {
		if res.Answers[fmt.Sprintf("gone_%d", i)].Noul >= 0.5 {
			in.Gone = append(in.Gone, n)
		}
	}
	for i, f := range files {
		if res.Answers[fmt.Sprintf("prune_%d", i)].Noul >= 0.5 {
			in.Prune = append(in.Prune, f)
		}
	}
	return in
}

// MustChange asks, for each remaining mention of name, whether the request
// needs it changed: a changelog line recording the rename, say, should stay.
// It returns the mentions that must change.
func MustChange(ctx context.Context, c *jev.Client, task, name string, mentions []string) ([]string, int64) {
	if c == nil || len(mentions) == 0 {
		return mentions, 0
	}
	qs := map[string]jev.Question{}
	for i := range mentions {
		qs[fmt.Sprintf("m%d", i)] = jev.Noul(fmt.Sprintf("For `task` to be done, must `mentions[%d]` change so that it no longer says `name`?", i))
	}
	res, err := c.Ask(ctx, map[string]any{"task": task, "name": name, "mentions": mentions}, qs)
	if err != nil {
		return mentions, 0
	}
	var out []string
	for i, m := range mentions {
		if res.Answers[fmt.Sprintf("m%d", i)].Noul >= 0.5 {
			out = append(out, m)
		}
	}
	return out, res.Usage.InputTokens
}
