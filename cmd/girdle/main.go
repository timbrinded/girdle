// Command girdle is a terminal coding agent. Run it with no arguments for the
// TUI, or with -p for a headless run that exits when the task is done.
package main

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openai"
	"charm.land/fantasy/providers/openaicompat"
	"charm.land/fantasy/providers/openrouter"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/clip"
	"github.com/timbrinded/girdle/internal/jev"
	"github.com/timbrinded/girdle/internal/kernel"
	"github.com/timbrinded/girdle/internal/models"
	"github.com/timbrinded/girdle/internal/tui"
)

// defaultModel is free on OpenRouter for now, and it is what the fast flow
// has been measured and tuned on since decision 0018. It is an anonymous
// "stealth" model: its provider may log prompts, so it suits public code
// only, and it may be withdrawn without notice (decision 0021).
const defaultModel = "stealth/space-bunny-alpha"

// OpenCode Zen's free LLMs only work inside OpenCode, but its free Jev
// works anywhere (decision 0017). See https://opencode.ai/docs/zen.
const (
	zenBaseURL  = "https://opencode.ai/zen/v1"
	zenJevURL   = "https://opencode.ai/zen"
	zenJevModel = "jev-1.13-free"
)

// Exit codes for headless runs.
const (
	exitDone      = 0
	exitError     = 1
	exitNeedsUser = 2
	exitCancelled = 130
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		prompt      = flag.String("p", "", "run this prompt headless and exit when done")
		dir         = flag.String("C", ".", "working directory")
		provider    = flag.String("provider", cmp.Or(os.Getenv("GIRDLE_PROVIDER"), "openrouter"), "LLM provider: openrouter (OPENROUTER_API_KEY) or zen, OpenCode Zen (ZEN_API_KEY)")
		model       = flag.String("model", os.Getenv("GIRDLE_MODEL"), "model ID at the provider (default: the default saved in the TUI's model picker, or "+defaultModel+" on OpenRouter; required with -provider zen)")
		jevVia      = flag.String("jev", cmp.Or(os.Getenv("GIRDLE_JEV"), "typesafe"), "where to call Jev: typesafe (TYPESAFE_API_KEY, pinned "+jev.DefaultModel+") or zen (ZEN_API_KEY, "+zenJevModel+")")
		reasoning   = flag.String("reasoning", "medium", "reasoning effort when Jev can't choose one: none, minimal, low, medium, high, xhigh or max, fitted to what the model accepts")
		logPath     = flag.String("log", "", "event log path (default: a new file under ~/.local/state/girdle/sessions)")
		jsonOut     = flag.Bool("json", false, "headless: print events as JSON lines")
		seedPath    = flag.String("seed", "", "headless: start from a seeded conversation (JSON)")
		noJev       = flag.Bool("no-checkpoints", false, "disable Jev checkpoints: stop at every turn end")
		noRoute     = flag.Bool("no-route", false, "don't let Jev choose the reasoning effort; always use -reasoning (without either flag, the effort saved in the TUI's model picker applies)")
		maxNudges   = flag.Int("max-nudges", checkpoint.DefaultPolicy.MaxNudges, "most nudges per run before asking the user")
		maxSteps    = flag.Int("max-steps", 60, "most LLM steps per turn")
		timeoutFlag = flag.Duration("timeout", 0, "headless: give up after this long (0 = no limit)")
		fast        = flag.Bool("fast", false, "fast flow: -snapshot -prefetch -batch -early-stop -stepfan -leftovers -speculate -crosscheck -reproduce -heartbeat -grepctx, and -race 3 -hedge 3s; setting any of them explicitly overrides it, so -fast -reproduce=false leaves reproduce out")
		snapshot    = flag.Bool("snapshot", false, "send the repository's files with each request")
		batch       = flag.Bool("batch", false, "ask the LLM to make all edits and run the checks in one step")
		earlyStop   = flag.Bool("early-stop", false, "end the run as soon as Jev reads the tool results as the task done")
		raceFlag    = flag.Int("race", 0, "send each LLM call this many times at once and keep the first complete answer (default 1, or 3 with -fast)")
		speculate   = flag.Bool("speculate", false, "start the first LLM call on low effort while Jev routes, instead of waiting")
		compact     = flag.Bool("compact", false, "every 8 steps, prune older tool output that Jev judges no longer needed (shelved: not part of -fast)")
		stepfan     = flag.Bool("stepfan", false, "at each step end, ask Jev a broad set of questions about the changes and the check's output, and stop once it reads the work as done and verified")
		prefetch    = flag.Bool("prefetch", false, "for a repository too large to snapshot whole, ask Jev which other files the request needs and add them to the snapshot")
		leftovers   = flag.Bool("leftovers", false, "ask Jev which names and files the request wants gone, and before stopping check that none remain")
		grepCtx     = flag.Bool("grepctx", false, "add to search results the definitions the matches are in, and show them when there are few")
		denyRead    []*regexp.Regexp
		offline     = flag.Bool("offline-tools", false, "run shell commands without outside network access (macOS), as the benchmark does so an agent can't fetch the fix it is tested on")
		tripwire    = flag.Bool("tripwire", true, "check every shell command first and block ones that delete outside the project, force-push a shared branch, or send secrets off the machine")
		reproduce   = flag.Bool("reproduce", false, "let apply check that a bug fix's regression test fails without the fix")
		heartbeat   = flag.Bool("heartbeat", false, "every 6 steps, ask Jev whether the work is looping or drifting, and nudge it if so")
		crossCheck  = flag.Bool("crosscheck", false, "write an independent test of each request in the background and run it when the agent's check passes (needs -batch and -early-stop)")
		hedge       = flag.Duration("hedge", -1, "with -race, wait this long for an answer before starting each extra copy of calls after a request's first (default 0, or 3s with -fast)")
	)
	flag.Func("deny-read", "a regular expression for absolute paths that tools may not read outside the working directory, such as other copies of a benchmark's code under test (repeatable; shell commands need macOS's sandbox-exec)", func(v string) error {
		re, err := regexp.Compile(v)
		if err != nil {
			return err
		}
		denyRead = append(denyRead, re)
		return nil
	})
	flag.Parse()
	// -fast turns a set of flags on. A flag set explicitly wins, so an
	// ablation is -fast with one part set to false.
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	withFast := func(name string, v bool) bool { return v || *fast && !explicit[name] }

	workDir, err := filepath.Abs(*dir)
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *provider == "zen" && *model == "" {
		return fail(errors.New("-provider zen needs -model: Zen's free models only work inside OpenCode, so pick a paid one"))
	}
	effort := checkpoint.Effort(*reasoning)
	if !slices.Contains(checkpoint.Efforts, effort) {
		return fail(fmt.Errorf("unknown -reasoning %q: use %s", effort, checkpoint.JoinEfforts(checkpoint.Efforts, ", ")))
	}
	cfg, open, err := buildConfig(*provider, *jevVia, !*noJev)
	if err != nil {
		return fail(err)
	}
	interactive := *prompt == "" && *seedPath == ""
	store := models.DefaultStore()
	st := start{model: *model, effort: effort, auto: !*noRoute}
	st.choose(store, *provider == "openrouter", interactive, explicit["reasoning"] || explicit["no-route"])
	lm, err := open(ctx, st.model)
	if err != nil {
		return fail(err)
	}
	cfg.Settings = kernel.Settings{
		Model: lm, ModelName: st.model, Efforts: st.catalog.Efforts(st.model),
		AutoEffort: st.auto, Effort: st.effort,
	}
	cfg.Dir = workDir
	cfg.Policy.MaxNudges = *maxNudges
	cfg.MaxStepsPerTurn = *maxSteps
	// Features whose prerequisites are off, such as Jev, are turned off by
	// the kernel. Jev routes even a pinned effort: whether a request asks
	// for tests matters to early stop.
	cfg.Route = true
	cfg.Snapshot = withFast("snapshot", *snapshot)
	cfg.Batch = withFast("batch", *batch)
	cfg.EarlyStop = withFast("early-stop", *earlyStop)
	cfg.Race = *raceFlag
	if cfg.Race == 0 {
		cfg.Race = 1
		if *fast {
			cfg.Race = 3
		}
	}
	cfg.Speculate = withFast("speculate", *speculate)
	cfg.CrossCheck = withFast("crosscheck", *crossCheck)
	cfg.Reproduce = withFast("reproduce", *reproduce)
	cfg.Heartbeat = withFast("heartbeat", *heartbeat)
	cfg.Prefetch = withFast("prefetch", *prefetch)
	if withFast("stepfan", *stepfan) {
		cfg.StepPolicy = checkpoint.FanoutStepPolicy
	}
	cfg.Compact = *compact
	cfg.CompactPolicy = checkpoint.DefaultCompactPolicy
	cfg.Tripwire = *tripwire
	cfg.OfflineTools = *offline
	cfg.DenyRead = denyRead
	cfg.GrepContext = withFast("grepctx", *grepCtx)
	cfg.Leftovers = withFast("leftovers", *leftovers)
	cfg.TripwirePolicy = checkpoint.DefaultTripwirePolicy
	cfg.HeartbeatPolicy = checkpoint.DefaultHeartbeatPolicy
	cfg.Hedge = *hedge
	if cfg.Hedge < 0 {
		cfg.Hedge = 0
		if *fast {
			cfg.Hedge = 3 * time.Second
		}
	}

	path := cmp.Or(*logPath, defaultLogPath())
	log, err := kernel.OpenLog(path)
	if err != nil {
		return fail(fmt.Errorf("open event log: %w", err))
	}
	defer log.Close()
	cfg.Log = log

	if interactive {
		var picker *tui.Models
		if *provider == "openrouter" {
			picker = &tui.Models{
				Store: store, List: st.saved, Catalog: st.catalog, Note: st.note, Open: open,
				Fetch: func(ctx context.Context) (models.Catalog, error) {
					return models.FetchForKey(ctx, http.DefaultClient, os.Getenv("OPENROUTER_API_KEY"))
				},
			}
		}
		if err := tui.Run(ctx, cfg, path, picker); err != nil {
			return fail(err)
		}
		return exitDone
	}
	if st.note != "" {
		fmt.Fprintln(os.Stderr, "girdle:", st.note)
	}

	if *timeoutFlag > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeoutFlag)
		defer cancel()
	}
	cfg.Emit = headlessPrinter(*jsonOut)
	sess := kernel.NewSession(cfg)

	var outcome kernel.Outcome
	var reason string
	if *seedPath != "" {
		task, msgs, err := loadSeed(*seedPath)
		if err != nil {
			return fail(err)
		}
		sess.Seed(msgs)
		if *prompt != "" {
			outcome, reason = sess.Run(ctx, *prompt)
		} else {
			outcome, reason = sess.Resume(ctx, task)
		}
	} else {
		outcome, reason = sess.Run(ctx, *prompt)
	}
	if !*jsonOut {
		fmt.Fprintf(os.Stderr, "\n[%s: %s] event log: %s\n", outcome, reason, path)
	}
	switch outcome {
	case kernel.OutcomeDone:
		return exitDone
	case kernel.OutcomeNeedsUser:
		return exitNeedsUser
	case kernel.OutcomeCancelled:
		return exitCancelled
	default:
		return exitError
	}
}

// start is what a session starts on, and the saved list and catalogue it
// was chosen from.
type start struct {
	saved   models.List
	catalog models.Catalog
	model   string
	note    string // why the default model wasn't used, if it wasn't
	effort  checkpoint.Effort
	auto    bool
}

// choose settles the model and effort a session starts on, from the flag
// values already in st: flags first, then what the user saved, then the
// defaults.
// The saved list and the cached catalogue are OpenRouter's. Only the TUI
// reads the catalogue: a headless run sends efforts as asked, so what it
// sends never depends on when the TUI last cached the list on this machine.
func (st *start) choose(store models.Store, openRouter, interactive, effortFlagged bool) {
	if openRouter {
		var err error
		if st.saved, err = store.Load(); err != nil {
			fmt.Fprintln(os.Stderr, "girdle: ignoring the saved models:", err)
		}
		if interactive {
			st.catalog, _ = store.Catalog()
		}
	}
	if st.model == "" {
		st.model, st.note = st.saved.Start(st.catalog, defaultModel)
	}
	if d := st.saved.Default.Effort; d != "" && !effortFlagged {
		e, auto, err := models.ParseEffort(d)
		if err != nil {
			fmt.Fprintln(os.Stderr, "girdle: ignoring the saved effort:", err)
			return
		}
		st.effort, st.auto = cmp.Or(e, st.effort), auto
	}
}

// buildConfig sets up the provider and Jev. open builds one of the
// provider's models by its ID.
func buildConfig(providerName, jevVia string, checkpoints bool) (cfg kernel.Config, open func(context.Context, string) (fantasy.LanguageModel, error), err error) {
	var provider fantasy.Provider
	var effort func(checkpoint.Effort) fantasy.ProviderOptions
	switch providerName {
	case "openrouter":
		key := os.Getenv("OPENROUTER_API_KEY")
		if key == "" {
			return kernel.Config{}, nil, errors.New("OPENROUTER_API_KEY is not set")
		}
		provider, err = openrouter.New(openrouter.WithAPIKey(key))
		effort = func(e checkpoint.Effort) fantasy.ProviderOptions {
			return openrouter.NewProviderOptions(&openrouter.ProviderOptions{
				Reasoning: &openrouter.ReasoningOptions{Effort: new(openrouter.ReasoningEffort(e))},
			})
		}
	case "zen":
		key := zenKey()
		if key == "" {
			return kernel.Config{}, nil, errors.New("ZEN_API_KEY is not set: get a key at https://opencode.ai/auth")
		}
		provider, err = openaicompat.New(openaicompat.WithBaseURL(zenBaseURL), openaicompat.WithAPIKey(key),
			openaicompat.WithResponsesAPIFunc(zenResponsesModel))
		effort = zenEffort
	default:
		return kernel.Config{}, nil, fmt.Errorf("unknown provider %q: use openrouter or zen", providerName)
	}
	if err != nil {
		return kernel.Config{}, nil, err
	}
	var jc *jev.Client
	if checkpoints {
		switch jevVia {
		case "typesafe":
			jc, err = jev.NewFromEnv()
		case "zen":
			key := zenKey()
			if key == "" {
				return kernel.Config{}, nil, errors.New("-jev zen needs ZEN_API_KEY")
			}
			jc = jev.New(zenJevURL, zenJevModel, key)
		default:
			err = fmt.Errorf("unknown -jev %q: use typesafe or zen", jevVia)
		}
		if err != nil {
			return kernel.Config{}, nil, err
		}
	}
	return kernel.Config{
		EffortOptions: effort,
		Jev:           jc,
		Policy:        checkpoint.DefaultPolicy,
		RoutePolicy:   checkpoint.DefaultRoutePolicy,
		StepPolicy:    checkpoint.DefaultStepPolicy,
		Checkpoints:   checkpoints,
	}, provider.LanguageModel, nil
}

// zenKey is the OpenCode Zen API key, from ZEN_API_KEY or OPENCODE_API_KEY.
func zenKey() string { return cmp.Or(os.Getenv("ZEN_API_KEY"), os.Getenv("OPENCODE_API_KEY")) }

// zenResponsesModel reports whether Zen serves a model through the
// Responses API rather than chat completions: its Muse Spark models.
func zenResponsesModel(id string) bool { return strings.HasPrefix(id, "muse-") }

// zenEffort sets the reasoning effort for either API: each model reads the
// options for its own.
func zenEffort(e checkpoint.Effort) fantasy.ProviderOptions {
	re := openai.ReasoningEffort(e)
	opts := openaicompat.NewProviderOptions(&openaicompat.ProviderOptions{ReasoningEffort: &re})
	maps.Copy(opts, openai.NewResponsesProviderOptions(&openai.ResponsesProviderOptions{ReasoningEffort: &re}))
	return opts
}

func defaultLogPath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	name := time.Now().Format("2006-01-02T15-04-05") + fmt.Sprintf("-%d.jsonl", os.Getpid())
	return filepath.Join(base, "girdle", "sessions", name)
}

// headlessPrinter prints a readable transcript, or JSON lines with -json.
func headlessPrinter(asJSON bool) func(kernel.Event) {
	return func(e kernel.Event) {
		if asJSON {
			if e.Type == kernel.EventTextDelta {
				return
			}
			b, _ := json.Marshal(e)
			fmt.Println(string(b))
			return
		}
		switch e.Type {
		case kernel.EventTextDelta:
			fmt.Print(e.Text)
		case kernel.EventAssistantText:
			fmt.Println()
		case kernel.EventToolCall:
			fmt.Printf("\n→ %s %s\n", e.Tool, clip.Head(e.Input, 200))
		case kernel.EventToolResult:
			fmt.Printf("  %s\n", clip.Head(strings.ReplaceAll(e.Text, "\n", " ⏎ "), 200))
		case kernel.EventDecision:
			d := e.Decision
			fmt.Printf("◆ jev %s: %s (%s) %dms\n", d.Checkpoint, d.Action, d.Rule, d.LatencyMS)
		case kernel.EventRoute:
			r := e.Route
			handling := ""
			if h := r.Handling(); h != "" {
				handling = ", " + h
			}
			fmt.Printf("◆ jev route: complexity %.2f → %s effort%s %dms\n", r.Score, cmp.Or(string(e.Effort), "no"), handling, r.LatencyMS)
		case kernel.EventNudge:
			fmt.Printf("↻ %s\n", e.Text)
		case kernel.EventError:
			fmt.Printf("✗ %s\n", e.Text)
		}
	}
}

type seedFile struct {
	Task     string `json:"task"`
	Messages []struct {
		Role string `json:"role"`
		Text string `json:"text"`
	} `json:"messages"`
}

func loadSeed(path string) (string, []fantasy.Message, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	var sf seedFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		return "", nil, fmt.Errorf("seed %s: %w", path, err)
	}
	msgs := make([]fantasy.Message, 0, len(sf.Messages))
	for _, m := range sf.Messages {
		switch m.Role {
		case "user":
			msgs = append(msgs, fantasy.NewUserMessage(m.Text))
		case "assistant":
			msgs = append(msgs, fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: m.Text}}})
		default:
			return "", nil, fmt.Errorf("seed %s: unknown role %q", path, m.Role)
		}
	}
	return sf.Task, msgs, nil
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "girdle:", err)
	return exitError
}
