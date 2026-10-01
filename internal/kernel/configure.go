package kernel

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/models"
)

// switchable are the features the girdle tool can turn on and off, by the
// names the log and the facts use. The tripwire, offline tools and denied
// reads are left out on purpose: text the model reads can steer it, so it
// must not be able to lower Girdle's floor. Checkpoints and routing are left
// out too: the route is what lets the tool change anything.
var switchable = map[string]func(*Config) *bool{
	"snapshot":   func(c *Config) *bool { return &c.Snapshot },
	"prefetch":   func(c *Config) *bool { return &c.Prefetch },
	"batch":      func(c *Config) *bool { return &c.Batch },
	"early_stop": func(c *Config) *bool { return &c.EarlyStop },
	"stepfan":    func(c *Config) *bool { return &c.StepPolicy.Fanout },
	"speculate":  func(c *Config) *bool { return &c.Speculate },
	"crosscheck": func(c *Config) *bool { return &c.CrossCheck },
	"heartbeat":  func(c *Config) *bool { return &c.Heartbeat },
	"compact":    func(c *Config) *bool { return &c.Compact },
	"reproduce":  func(c *Config) *bool { return &c.Reproduce },
	"leftovers":  func(c *Config) *bool { return &c.Leftovers },
	"grepctx":    func(c *Config) *bool { return &c.GrepContext },
}

// toolFeatures are the switchable features the tools are built with.
var toolFeatures = []string{"batch", "reproduce", "grepctx"}

// applyFeatures takes the features the girdle tool turned on or off, as a
// request starts.
func (s *Session) applyFeatures() {
	want := s.want.resolved()
	var changed []string
	for name, field := range switchable {
		if *field(&s.cfg) != *field(&want) {
			*field(&s.cfg) = *field(&want)
			changed = append(changed, name)
		}
	}
	if len(changed) == 0 {
		return
	}
	// stepfan's thresholds come with it.
	s.cfg.StepPolicy = want.StepPolicy
	if slices.ContainsFunc(changed, func(n string) bool { return slices.Contains(toolFeatures, n) }) {
		s.buildTools()
	}
	s.useModel()
	s.emit(Event{Type: EventSettings, Meta: s.cfg.features()})
}

// configure makes the girdle tool's settings changes, from the next request,
// and describes them. It changes nothing unless the route read the user's
// request as being about Girdle: text the model reads, such as a file in the
// repository, can ask it to call the tool, but can't change what the user
// asked.
func (s *Session) configure(ctx context.Context, in GirdleInput) (string, error) {
	// A pinned effort's route may still be on its way.
	s.takeRoute(ctx, true)
	if !s.girdle {
		return "", errors.New("Girdle changes its own settings only when the user's request asks for that, and Jev doesn't read this request that way. The user can change the model and effort in the terminal UI (/model, /effort) and features with flags")
	}
	if in.SaveDefault && s.cfg.SaveDefaults == nil {
		return "", errors.New("this session can't save defaults: they belong to the OpenRouter model picker")
	}

	// Check every change before making any.
	set := s.cfg.Settings
	if next := s.next.Load(); next != nil {
		set = *next
	}
	var done []string
	if in.Model != "" {
		if s.cfg.OpenModel == nil {
			return "", errors.New("this session can't switch models")
		}
		lm, efforts, err := s.cfg.OpenModel(ctx, in.Model)
		if err != nil {
			return "", fmt.Errorf("model %s: %w", in.Model, err)
		}
		set.Model, set.ModelName, set.Efforts = lm, in.Model, efforts
		done = append(done, "model "+in.Model)
	}
	if in.Effort != "" {
		e := checkpoint.Effort(in.Effort)
		switch {
		case in.Effort == models.EffortAuto && !s.canAuto:
			return "", errors.New("auto effort needs Jev to choose efforts, which it can't in this session")
		case in.Effort == models.EffortAuto:
			set.AutoEffort = true
		case !slices.Contains(checkpoint.Efforts, e):
			return "", fmt.Errorf("unknown effort %q: use auto or %s", in.Effort, checkpoint.JoinEfforts(checkpoint.Efforts, ", "))
		case len(set.Efforts) == 0:
			return "", fmt.Errorf("%s has no reasoning effort setting", set.ModelName)
		default:
			set.AutoEffort, set.Effort = false, e
		}
	}
	if in.Effort != "" || in.Model != "" {
		done = append(done, "effort "+effortSetting(set))
	}
	want := s.want
	for _, turn := range []struct {
		on    bool
		names []string
	}{{true, in.TurnOn}, {false, in.TurnOff}} {
		on := turn.on
		for _, name := range turn.names {
			field, ok := switchable[name]
			if !ok {
				return "", fmt.Errorf("can't switch %q: the features Girdle can switch are %s", name, strings.Join(slices.Sorted(maps.Keys(switchable)), ", "))
			}
			*field(&want) = on
			if name == "stepfan" {
				want.StepPolicy = checkpoint.DefaultStepPolicy
				if on {
					want.StepPolicy = checkpoint.FanoutStepPolicy
				}
			}
		}
	}

	// Make them.
	if in.Model != "" || in.Effort != "" {
		s.Configure(set)
	}
	resolved := want.resolved()
	for _, name := range slices.Sorted(maps.Keys(switchable)) {
		field := switchable[name]
		switch {
		case *field(&want) == *field(&s.want):
		case *field(&resolved) != *field(&want):
			done = append(done, name+" on, but it stays off while a feature it needs is off (see the configuration guide)")
		default:
			done = append(done, name+map[bool]string{true: " on", false: " off"}[*field(&want)])
		}
	}
	s.want = want
	summary := "Changed from the user's next request: " + cmp.Or(strings.Join(done, "; "), "nothing") + "."
	if in.SaveDefault {
		if err := s.cfg.SaveDefaults(set.ModelName, effortSetting(set)); err != nil {
			summary += " Saving the defaults failed: " + err.Error()
		} else {
			summary += fmt.Sprintf(" New sessions now start on %s at effort %s.", set.ModelName, effortSetting(set))
		}
	}
	s.emit(Event{Type: EventConfigure, Text: summary, Settings: &set, Meta: map[string]string{"changes": strings.Join(done, "; "), "saved": fmt.Sprint(in.SaveDefault)}})
	return summary, nil
}

// effortSetting is set's effort setting as saved: auto or an effort.
func effortSetting(set Settings) string {
	if set.AutoEffort {
		return models.EffortAuto
	}
	return string(set.Effort.Fit(set.Efforts))
}
