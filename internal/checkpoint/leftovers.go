package checkpoint

import (
	"context"
	"fmt"

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
	Gone []string `json:"gone"`
	// Prune are the files whose unused code the request wants removed.
	Prune []string `json:"prune"`
	Call
}

// AskIntent asks, for each name and file the request mentions, whether it
// should be gone by the end. One request, one question per item.
func AskIntent(ctx context.Context, c *jev.Client, task string, names, files []string) Intent {
	var in Intent
	if len(names)+len(files) == 0 {
		return in
	}
	qs := map[string]jev.Question{}
	for i := range names {
		qs[fmt.Sprintf("gone_%d", i)] = jev.Noul(fmt.Sprintf("Does `task` ask for `names[%d]` to be renamed, replaced or removed, so that it no longer appears in the repository?", i))
	}
	for i := range files {
		qs[fmt.Sprintf("prune_%d", i)] = jev.Noul(fmt.Sprintf("Does `task` ask to remove the code in `files[%d]` that nothing else uses?", i))
	}
	var ok bool
	if in.Call, ok = ask(ctx, c, map[string]any{"task": task, "names": names, "files": files}, qs); !ok {
		return in
	}
	for i, n := range names {
		if in.Answers[fmt.Sprintf("gone_%d", i)].Noul >= 0.5 {
			in.Gone = append(in.Gone, n)
		}
	}
	for i, f := range files {
		if in.Answers[fmt.Sprintf("prune_%d", i)].Noul >= 0.5 {
			in.Prune = append(in.Prune, f)
		}
	}
	return in
}

// MustChange asks, for each remaining mention of name, whether the request
// needs it changed: a changelog line recording the rename, say, should stay.
// If Jev can't be asked, every mention must change.
func MustChange(ctx context.Context, c *jev.Client, task, name string, mentions []string) MustChangeDecision {
	d := MustChangeDecision{Checkpoint: "must_change", Name: name, Mentions: mentions, Must: mentions}
	if len(mentions) == 0 {
		return d
	}
	qs := map[string]jev.Question{}
	for i := range mentions {
		qs[fmt.Sprintf("m%d", i)] = jev.Noul(fmt.Sprintf("For `task` to be done, must `mentions[%d]` change so that it no longer says `name`?", i))
	}
	var ok bool
	if d.Call, ok = ask(ctx, c, map[string]any{"task": task, "name": name, "mentions": mentions}, qs); !ok {
		return d
	}
	d.Must = nil
	for i, m := range mentions {
		if d.Answers[fmt.Sprintf("m%d", i)].Noul >= 0.5 {
			d.Must = append(d.Must, m)
		}
	}
	return d
}

// MustChangeDecision records which mentions of a name must change.
type MustChangeDecision struct {
	Checkpoint string   `json:"checkpoint"`
	Name       string   `json:"name"`
	Mentions   []string `json:"mentions"`
	Must       []string `json:"must"`
	Call
}
