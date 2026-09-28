package checkpoint

import "slices"

// Effort is the reasoning effort the LLM is asked to use.
type Effort string

const (
	EffortNone    Effort = "none" // reasoning off
	EffortMinimal Effort = "minimal"
	EffortLow     Effort = "low"
	EffortMedium  Effort = "medium"
	EffortHigh    Effort = "high"
	EffortXHigh   Effort = "xhigh"
	EffortMax     Effort = "max"
)

// Efforts lists every reasoning effort OpenRouter names, lowest first. A
// model whose efforts are unknown is assumed to accept all of them.
var Efforts = []Effort{EffortNone, EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax}

// Fit returns the effort to ask of a model that accepts only supported: e
// itself if supported has it, otherwise the lowest supported effort above
// e, otherwise the highest below. Rounding up keeps a request from getting
// less reasoning than it was routed to. None, which turns reasoning off, is
// only chosen when asked for. It returns "" when supported is empty: the
// model has no effort setting.
func (e Effort) Fit(supported []Effort) Effort {
	if slices.Contains(supported, e) {
		return e
	}
	want := slices.Index(Efforts, e)
	var below Effort
	for _, s := range Efforts {
		if s == EffortNone || !slices.Contains(supported, s) {
			continue
		}
		if slices.Index(Efforts, s) > want {
			return s
		}
		below = s
	}
	return below
}
