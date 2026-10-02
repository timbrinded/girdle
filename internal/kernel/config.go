package kernel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"charm.land/fantasy"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/jev"
)

// Settings are the choices a user can change between requests: the model
// and its reasoning effort.
type Settings struct {
	Model     fantasy.LanguageModel
	ModelName string
	// Efforts are the reasoning efforts the model accepts, lowest first.
	// Each request asks for the nearest of them (checkpoint.Effort.Fit).
	// Empty means the model has no effort setting, and none is sent.
	Efforts []checkpoint.Effort
	// AutoEffort lets Jev's route choose each request's reasoning effort.
	// Without it, or when Jev can't route, requests use Effort.
	AutoEffort bool
	Effort     checkpoint.Effort
}

// Config wires a session together.
type Config struct {
	Settings
	Jev    *jev.Client
	Dir    string
	Policy checkpoint.Policy
	// Checkpoints turns the Jev turn-end checkpoint on. When false, every
	// turn end stops the run, as in a plain agent loop.
	Checkpoints bool
	// Route asks Jev how hard each request is, and whether it asks for
	// tests. With AutoEffort, the answer sets the request's reasoning effort.
	Route       bool
	RoutePolicy checkpoint.RoutePolicy
	// EffortOptions builds provider options for a reasoning effort. Without
	// it, no effort is sent.
	EffortOptions   func(checkpoint.Effort) fantasy.ProviderOptions
	MaxStepsPerTurn int
	Emit            func(Event)
	Log             *Log

	// Snapshot sends the repository's files with each request, so the LLM
	// doesn't spend steps listing and reading them.
	Snapshot       bool
	SnapshotBudget int
	// Prefetch asks Jev, for a repository too large to snapshot whole,
	// which other files the request needs, and adds them. It needs Snapshot.
	Prefetch bool
	// Batch asks the LLM to make all its edits and run the checks in one
	// step, since every extra step costs seconds of latency.
	Batch bool
	// EarlyStop asks Jev, after any step whose last command succeeded,
	// whether the task is already done, and ends the run there if so.
	// It needs Checkpoints.
	EarlyStop  bool
	StepPolicy checkpoint.StepPolicy
	// Race sends each LLM call this many times and keeps the first complete
	// answer. Below 2, calls are not raced. The first call of a request
	// starts every copy at once; later calls start an extra copy only after
	// waiting Hedge for an answer, so quick steps cost one call. A zero
	// Hedge starts every copy at once for every call.
	Race  int
	Hedge time.Duration
	// Speculate starts a request's first LLM call on the usual effort while
	// Jev routes it, instead of waiting for the route. It needs Route, and
	// acts only while Jev chooses the effort (AutoEffort), which settings
	// can change between requests.
	Speculate bool
	// CrossCheck writes an independent test of each request in the
	// background and runs it once the agent's own check passes. It needs
	// Batch and EarlyStop.
	CrossCheck bool
	// Heartbeat asks Jev every few steps whether the turn is looping or
	// drifting, and nudges it if so. It needs Checkpoints.
	Heartbeat       bool
	HeartbeatPolicy checkpoint.HeartbeatPolicy
	// Compact prunes older tool output that Jev judges no longer needed,
	// every CompactPolicy.Every steps. It needs Checkpoints.
	Compact       bool
	CompactPolicy checkpoint.CompactPolicy
	// Reproduce lets apply check that a bug fix's regression test fails
	// without the fix. It needs Batch.
	Reproduce bool
	// Leftovers asks Jev which names and files the request wants gone, and
	// before the run stops, checks that none of them remain.
	Leftovers bool
	// Tripwire checks every shell command before it runs and blocks the
	// catastrophic ones, handing the run back to the user.
	Tripwire       bool
	TripwirePolicy checkpoint.TripwirePolicy
	// OfflineTools runs shell commands without outside network access
	// (macOS only), so a benchmark agent can't fetch the fix it's tested on.
	OfflineTools bool
	// DenyRead lists path prefixes the tools may not read outside the
	// working directory, such as other copies of a benchmark's code under
	// test.
	DenyRead []*regexp.Regexp
	// GrepContext adds to search results the definitions the matches are
	// in, and shows them when there are few.
	GrepContext bool

	// OpenModel opens model id, with the efforts it accepts, for the girdle
	// tool's model switch. Nil means the model can't be switched that way.
	OpenModel func(ctx context.Context, id string) (fantasy.LanguageModel, []checkpoint.Effort, error)
	// SaveDefaults saves a model and effort setting ("auto" or an effort)
	// as what new sessions start with. Nil means they can't be saved.
	SaveDefaults func(model, effort string) error
}

// resolved turns off every feature whose prerequisites are off, so the rest
// of the kernel reads one field per feature. This is the only place the
// features' dependencies on each other are written down.
func (c Config) resolved() Config {
	judged := c.Checkpoints && c.Jev != nil
	c.Route = c.Route && c.Jev != nil
	c.AutoEffort = c.AutoEffort && c.canAutoEffort()
	c.Speculate = c.Speculate && c.Route
	c.EarlyStop = c.EarlyStop && judged
	c.CrossCheck = c.CrossCheck && c.Batch && c.EarlyStop
	c.Heartbeat = c.Heartbeat && judged
	c.Compact = c.Compact && judged
	c.Reproduce = c.Reproduce && c.Batch
	c.Leftovers = c.Leftovers && c.Jev != nil
	c.Snapshot = c.Snapshot && unindexed(c.Dir) == ""
	c.Prefetch = c.Prefetch && c.Snapshot && c.Jev != nil
	if c.MaxStepsPerTurn == 0 {
		c.MaxStepsPerTurn = 60
	}
	return c
}

// features records which features are on, by name, for the log and the
// girdle tool.
func (c Config) features() map[string]string {
	return map[string]string{
		"checkpoints": fmt.Sprint(c.Checkpoints),
		"route":       fmt.Sprint(c.Route),
		"snapshot":    fmt.Sprint(c.Snapshot),
		"batch":       fmt.Sprint(c.Batch),
		"early_stop":  fmt.Sprint(c.EarlyStop),
		"speculate":   fmt.Sprint(c.Speculate),
		"crosscheck":  fmt.Sprint(c.CrossCheck),
		"heartbeat":   fmt.Sprint(c.Heartbeat),
		"compact":     fmt.Sprint(c.Compact),
		"prefetch":    fmt.Sprint(c.Prefetch),
		"stepfan":     fmt.Sprint(c.StepPolicy.Fanout),
		"reproduce":   fmt.Sprint(c.Reproduce),
		"tripwire":    fmt.Sprint(c.Tripwire),
		"leftovers":   fmt.Sprint(c.Leftovers),
		"grepctx":     fmt.Sprint(c.GrepContext),
	}
}

// unindexed says why Girdle won't read dir's files up front, or returns "".
// Like fff, it leaves the filesystem root and the home directory alone:
// either holds far more than any project, and walking one held up every
// request in v0.2.0. Their subdirectories are read as usual.
func unindexed(dir string) string {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	if filepath.Dir(dir) == dir {
		return "the filesystem root"
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == dir {
		return "your home directory"
	}
	return ""
}

// canAutoEffort reports whether Jev can choose request efforts.
func (c Config) canAutoEffort() bool {
	return c.Route && c.Jev != nil && c.EffortOptions != nil
}

// effort fits e to the model and builds the provider options that ask for
// it. A model with no effort setting is asked for none.
func (c Config) effort(e checkpoint.Effort) (checkpoint.Effort, fantasy.ProviderOptions) {
	e = e.Fit(c.Efforts)
	if e == "" || c.EffortOptions == nil {
		return e, nil
	}
	return e, c.EffortOptions(e)
}
