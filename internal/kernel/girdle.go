package kernel

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"strings"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/docs"
	"github.com/timbrinded/girdle/internal/checkpoint"
)

// girdleTopics are the guides the girdle tool serves, each a file in
// docs.Guides, with what it covers.
var girdleTopics = map[string]string{
	"getting-started": "installing Girdle, its keys and a first run",
	"usage":           "the terminal UI, its keys and commands, headless runs and the event log",
	"configuration":   "flags, providers, models, reasoning effort and features",
	"architecture":    "how the kernel, the checkpoints and Jev decide what happens",
}

// routedTopics are the guides sent with a request the route reads as a
// question about Girdle: the ones most such questions are about.
var routedTopics = []string{"usage", "configuration"}

var girdleDescription = func() string {
	var b strings.Builder
	b.WriteString("Answer questions about Girdle, the coding agent you are running in, from this build rather than from memory or the repository, and change its settings when the user asks. Returns any changes made, this session's facts (version, model, reasoning effort, Jev, features on), then the guides you ask for:")
	for _, t := range slices.Sorted(maps.Keys(girdleTopics)) {
		fmt.Fprintf(&b, "\n- %s: %s", t, girdleTopics[t])
	}
	b.WriteString("\nSettings changes take effect from the user's next request.")
	return b.String()
}()

// GirdleInput is the girdle tool's input. Every field is optional.
type GirdleInput struct {
	Topics      []string `json:"topics,omitempty" description:"Guides to include, by topic. Without any, only this session's facts are returned."`
	Model       string   `json:"model,omitempty" description:"Switch to this model ID at the session's provider, as the model picker and the facts show IDs."`
	Effort      string   `json:"effort,omitempty" description:"Set the reasoning effort: auto, where Jev chooses for each request, or none, minimal, low, medium, high, xhigh or max."`
	TurnOn      []string `json:"turn_on,omitempty" description:"Features to turn on: snapshot, prefetch, batch, early_stop, stepfan, speculate, crosscheck, heartbeat, compact, reproduce, leftovers or grepctx. The configuration guide says what each does."`
	TurnOff     []string `json:"turn_off,omitempty" description:"Features to turn off, from the same list."`
	SaveDefault bool     `json:"save_default,omitempty" description:"Save the model and effort, after any change here, as what new sessions start with."`
}

// Nudge texts that carry the girdle tool's answer to the LLM.
const (
	girdleRouted = "[Girdle] This request asks about Girdle itself, so here is the girdle tool's answer for this session. Answer from it, and call girdle for another guide only if it doesn't cover the question."
	girdleLate   = "[Girdle] The request asks about Girdle itself, and your answer didn't come from the girdle tool. Here is its answer for this session: answer again from it, correcting anything it contradicts."
)

func (s *Session) girdleTool() fantasy.AgentTool {
	return fantasy.NewAgentTool("girdle", girdleDescription, func(ctx context.Context, in GirdleInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		for _, t := range in.Topics {
			if _, ok := girdleTopics[t]; !ok {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("unknown topic %q: choose from %s", t, strings.Join(slices.Sorted(maps.Keys(girdleTopics)), ", "))), nil
			}
		}
		var changed string
		if in.Model != "" || in.Effort != "" || len(in.TurnOn)+len(in.TurnOff) > 0 || in.SaveDefault {
			var err error
			if changed, err = s.configure(ctx, in); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			changed = "=== changes\n" + changed + "\n\n"
		}
		return fantasy.NewTextResponse(changed + s.aboutGirdle(in.Topics)), nil
	})
}

// girdleAnswer is lead followed by the girdle tool's answer for a routed
// request. The LLM has now seen it, so the turn end won't send it again.
func (s *Session) girdleAnswer(lead string) string {
	s.girdleSeen = true
	return lead + "\n\n" + s.aboutGirdle(routedTopics)
}

// aboutGirdle is the girdle tool's answer: the session's facts, then each
// topic's guide. Every topic must be in girdleTopics.
func (s *Session) aboutGirdle(topics []string) string {
	var b strings.Builder
	b.WriteString("=== this session\n")
	b.WriteString(s.facts())
	for _, t := range topics {
		// go:embed fails the build if a topic's guide is missing.
		guide, _ := docs.Guides.ReadFile(t + ".md")
		fmt.Fprintf(&b, "\n=== guide %s\n%s", t, guide)
	}
	return b.String()
}

// facts describes the session as it is configured now. Settings can change
// between requests, so they are read at each call.
func (s *Session) facts() string {
	c := s.cfg
	var b strings.Builder
	fmt.Fprintf(&b, "Girdle version: %s\n", buildVersion())
	fmt.Fprintf(&b, "Working directory: %s\n", c.Dir)
	fmt.Fprintf(&b, "Model: %s\n", c.ModelName)
	switch {
	case len(c.Efforts) == 0:
		b.WriteString("Reasoning effort: the model has no effort setting\n")
	case c.AutoEffort:
		fmt.Fprintf(&b, "Reasoning effort: auto, chosen by Jev for each request; the model accepts %s\n", checkpoint.JoinEfforts(c.Efforts, ", "))
	default:
		fmt.Fprintf(&b, "Reasoning effort: fixed at %s; the model accepts %s\n", c.Effort.Fit(c.Efforts), checkpoint.JoinEfforts(c.Efforts, ", "))
	}
	if c.Jev == nil {
		b.WriteString("Jev: off, so no checkpoints, routing or tripwire judgements\n")
	} else {
		fmt.Fprintf(&b, "Jev: %s\n", c.Jev.Model)
	}
	var on []string
	for name, v := range c.features() {
		if v == "true" {
			on = append(on, name)
		}
	}
	slices.Sort(on)
	fmt.Fprintf(&b, "Features on: %s\n", cmp.Or(strings.Join(on, ", "), "none"))
	if c.Race > 1 {
		fmt.Fprintf(&b, "Each LLM call is raced %d times\n", c.Race)
	}
	return b.String()
}

// buildVersion is the module version and commit Girdle was built from.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	v := info.Main.Version
	for _, st := range info.Settings {
		switch {
		case st.Key == "vcs.revision":
			v += ", commit " + st.Value[:min(len(st.Value), 12)]
		case st.Key == "vcs.modified" && st.Value == "true":
			v += " with uncommitted changes"
		}
	}
	return v
}
