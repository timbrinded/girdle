package tui

import (
	"fmt"
	"slices"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/kernel"
	"github.com/timbrinded/girdle/internal/models"
)

// The effort setting is auto, where Jev chooses each request's effort, or
// one pinned effort. m.settings.AutoEffort is only ever set when Jev can
// choose (canRoute), so the rest of the TUI reads it directly.

// effortChoices are the effort settings for the model in use: auto, when
// Jev can route, then each effort the model accepts, lowest first.
func (m *model) effortChoices() []string {
	efforts := m.settings.Efforts
	var choices []string
	if m.canRoute && len(efforts) > 0 {
		choices = append(choices, models.EffortAuto)
	}
	for _, e := range efforts {
		choices = append(choices, string(e))
	}
	return choices
}

// cycleEffort steps the effort setting on to the next choice.
func (m *model) cycleEffort() {
	choices := m.effortChoices()
	if len(choices) == 0 {
		m.notify(toneInfo, m.settings.ModelName+" has no reasoning effort setting")
		return
	}
	// A pinned effort the model doesn't accept steps on from the one it
	// is fitted to.
	current := models.EffortAuto
	if !m.settings.AutoEffort {
		current = string(m.settings.Effort.Fit(m.settings.Efforts))
	}
	// Every choice parses: they are auto and checkpoint efforts.
	e, auto, _ := models.ParseEffort(choices[(slices.Index(choices, current)+1)%len(choices)])
	m.applyEffort(e, auto)
	if m.running {
		m.notify(toneInfo, "effort "+m.effortLabel()+m.fromNext())
	}
}

// setEffort applies an effort setting typed or picked by the user, and
// reports whether it was valid.
func (m *model) setEffort(setting string) bool {
	e, auto, err := models.ParseEffort(setting)
	switch {
	case err != nil:
		m.notify(toneError, err.Error())
		return false
	case len(m.settings.Efforts) == 0:
		m.notify(toneInfo, m.settings.ModelName+" has no reasoning effort setting")
		return false
	case auto && !m.canRoute:
		m.notify(toneInfo, "auto needs Jev, which is off: pick an effort instead")
		return false
	}
	m.applyEffort(e, auto)
	if !auto && e.Fit(m.settings.Efforts) != e {
		m.notify(toneInfo, fmt.Sprintf("%s doesn't take %s, so it gets %s", m.settings.ModelName, e, m.effortLabel()))
		return true
	}
	m.notify(toneInfo, "effort "+m.effortLabel()+m.fromNext())
	return true
}

// applyEffort switches to auto, or to effort e.
func (m *model) applyEffort(e checkpoint.Effort, auto bool) {
	m.settings.AutoEffort = auto
	if !auto {
		m.settings.Effort = e
	}
	m.configure()
}

// configure hands the settings to the session for its next request. Jev's
// latest route belongs to the old settings, unless a request is still
// running on them.
func (m *model) configure() {
	if !m.running {
		m.routed = ""
	}
	m.sess.Configure(m.settings)
}

// changed reports that the next request's settings differ from those the
// running request started with.
func (m *model) changed() bool {
	a, s := m.active, m.settings
	return a.ModelName != s.ModelName || a.AutoEffort != s.AutoEffort || a.Effort != s.Effort
}

// fromNext notes that a change waits for the running request to end.
func (m *model) fromNext() string {
	if m.running {
		return " (from the next request)"
	}
	return ""
}

// effortSetting is the setting as saved: auto or an effort.
func (m *model) effortSetting() string {
	if m.settings.AutoEffort {
		return models.EffortAuto
	}
	return string(m.settings.Effort)
}

// effortLabel describes the effort the next request uses.
func (m *model) effortLabel() string {
	if m.running && m.changed() {
		return effortLabel(m.settings, "")
	}
	return effortLabel(m.settings, m.routed)
}

// effortLabel describes the effort requests on s use: auto, with the effort
// Jev routed the latest one to, or the pinned effort, and what it is fitted
// to when the model doesn't accept it.
func effortLabel(s kernel.Settings, routed checkpoint.Effort) string {
	switch fit := s.Effort.Fit(s.Efforts); {
	case len(s.Efforts) == 0:
		return "n/a"
	case s.AutoEffort && routed != "":
		return "auto → " + string(routed)
	case s.AutoEffort:
		return "auto"
	case fit != s.Effort:
		return fmt.Sprintf("%s (asked %s)", fit, s.Effort)
	default:
		return string(s.Effort)
	}
}

// effortAbout says what each effort setting means.
var effortAbout = map[string]string{
	models.EffortAuto: "Jev chooses for each request",
	"none":            "no reasoning",
	"minimal":         "the least reasoning, for the fastest answers",
	"low":             "quick reasoning, for simple steps",
	"medium":          "balanced",
	"high":            "thorough reasoning",
	"xhigh":           "extra thorough",
	"max":             "as much reasoning as the model offers",
}
