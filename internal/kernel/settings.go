package kernel

import (
	"fmt"
	"slices"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/race"
)

// Configure changes the model and reasoning effort from the next request
// on. It is safe to call while a request runs.
func (s *Session) Configure(set Settings) { s.next.Store(&set) }

// CanAutoEffort reports whether Jev can choose request efforts, so that
// Settings.AutoEffort takes effect.
func (s *Session) CanAutoEffort() bool { return s.canAuto }

// applySettings switches to the settings Configure left, if any, as a
// request starts. Only cfg.Settings changes: goroutines still finishing the
// last request may be reading the rest.
func (s *Session) applySettings() {
	set := s.next.Swap(nil)
	if set == nil {
		return
	}
	if set.ModelName != s.cfg.ModelName {
		s.history = portable(s.history)
	}
	set.AutoEffort = set.AutoEffort && s.canAuto
	s.cfg.Settings = *set
	s.useModel()
	s.emit(Event{Type: EventSettings, Meta: s.settingsMeta()})
}

// useModel builds the agent around cfg.Model.
func (s *Session) useModel() {
	s.model = sessionModel{LanguageModel: race.New(s.cfg.Model, s.cfg.Race, s.stagger, s.noteRace), s: s}
	s.agent = fantasy.NewAgent(
		s.model,
		fantasy.WithSystemPrompt(systemPrompt(s.cfg)),
		fantasy.WithTools(s.tools...),
		fantasy.WithStopConditions(fantasy.StepCountIs(s.cfg.MaxStepsPerTurn)),
	)
}

// settingsMeta records the settings for the log.
func (s *Session) settingsMeta() map[string]string {
	return map[string]string{
		"model":       s.cfg.ModelName,
		"auto_effort": fmt.Sprint(s.cfg.AutoEffort),
		"effort":      string(s.cfg.Effort),
		"efforts":     checkpoint.JoinEfforts(s.cfg.Efforts, ","),
	}
}

// portable rewrites a conversation, for a model that didn't write it, into
// the form every chat API accepts:
//   - Reasoning is dropped, with the messages that were only reasoning.
//     Reasoning belongs to the model that wrote it, which may have signed or
//     encrypted it.
//   - An assistant message's tool calls are made one at a time, each
//     followed by its result. Space Bunny's provider fails on any history
//     with parallel calls, which Muse Spark makes.
func portable(msgs []fantasy.Message) []fantasy.Message {
	out := make([]fantasy.Message, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		if m.Role != fantasy.MessageRoleAssistant {
			out = append(out, m)
			continue
		}
		m.Content = slices.DeleteFunc(slices.Clone(m.Content), func(p fantasy.MessagePart) bool {
			return p.GetType() == fantasy.ContentTypeReasoning
		})
		var calls []fantasy.ToolCallPart
		var rest []fantasy.MessagePart
		for _, p := range m.Content {
			if c, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](p); ok {
				calls = append(calls, c)
			} else {
				rest = append(rest, p)
			}
		}
		if len(calls) < 2 || i+1 == len(msgs) || msgs[i+1].Role != fantasy.MessageRoleTool {
			if len(m.Content) > 0 {
				out = append(out, m)
			}
			continue
		}
		i++
		results := msgs[i].Content
		// The assistant's text goes with the first call.
		for k, call := range calls {
			content := []fantasy.MessagePart{call}
			if k == 0 {
				content = append(rest, call)
			}
			out = append(out, fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: content, ProviderOptions: m.ProviderOptions})
			for _, r := range results {
				if r, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](r); ok && r.ToolCallID == call.ToolCallID {
					out = append(out, fantasy.Message{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{r}})
				}
			}
		}
	}
	return out
}
