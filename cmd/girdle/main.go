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
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openrouter"

	"github.com/timbrinded/girdle/internal/checkpoint"
	"github.com/timbrinded/girdle/internal/jev"
	"github.com/timbrinded/girdle/internal/kernel"
	"github.com/timbrinded/girdle/internal/tui"
)

const defaultModel = "meta/muse-spark-1.3-contributor"

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
		model       = flag.String("model", cmp.Or(os.Getenv("GIRDLE_MODEL"), defaultModel), "OpenRouter model ID")
		reasoning   = flag.String("reasoning", "medium", "reasoning effort: none, minimal, low, medium, high, xhigh")
		logPath     = flag.String("log", "", "event log path (default: a new file under ~/.local/state/girdle/sessions)")
		jsonOut     = flag.Bool("json", false, "headless: print events as JSON lines")
		seedPath    = flag.String("seed", "", "headless: start from a seeded conversation (JSON)")
		noJev       = flag.Bool("no-checkpoints", false, "disable Jev checkpoints: stop at every turn end")
		noRoute     = flag.Bool("no-route", false, "don't let Jev choose the reasoning effort; always use -reasoning")
		maxNudges   = flag.Int("max-nudges", checkpoint.DefaultPolicy.MaxNudges, "most nudges per run before asking the user")
		maxSteps    = flag.Int("max-steps", 60, "most LLM steps per turn")
		timeoutFlag = flag.Duration("timeout", 0, "headless: give up after this long (0 = no limit)")
		fast        = flag.Bool("fast", false, "fast flow: -snapshot -batch -early-stop -speculate -crosscheck -heartbeat, and -race 3 -hedge 3s unless set")
		snapshot    = flag.Bool("snapshot", false, "send the repository's files with each request")
		batch       = flag.Bool("batch", false, "ask the LLM to make all edits and run the checks in one step")
		earlyStop   = flag.Bool("early-stop", false, "end the run as soon as Jev reads the tool results as the task done")
		raceFlag    = flag.Int("race", 0, "send each LLM call this many times at once and keep the first complete answer (default 1, or 3 with -fast)")
		speculate   = flag.Bool("speculate", false, "start the first LLM call on low effort while Jev routes, instead of waiting")
		fastModel   = flag.String("fast-model", "", "OpenRouter model for every LLM call after a request's first, and the cross-check writer, for example openai/gpt-oss-120b")
		fastProv    = flag.String("fast-provider", "groq,cerebras", "comma-separated OpenRouter providers to try, in order, for -fast-model")
		fastAll     = flag.Bool("fast-all", false, "with -fast-model, use it for every LLM call")
		fastEffort  = flag.String("fast-effort", "", "with -fast-model, the reasoning effort for its calls (default: the routed effort)")
		heartbeat   = flag.Bool("heartbeat", false, "every 6 steps, ask Jev whether the work is looping or drifting, and nudge it if so")
		crossCheck  = flag.Bool("crosscheck", false, "write an independent test of each request in the background and run it when the agent's check passes (needs -batch and -early-stop)")
		hedge       = flag.Duration("hedge", -1, "with -race, wait this long for an answer before starting each extra copy of calls after a request's first (default 0, or 3s with -fast)")
	)
	flag.Parse()

	workDir, err := filepath.Abs(*dir)
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, err := buildConfig(ctx, workDir, *model, *reasoning, !*noJev)
	if err != nil {
		return fail(err)
	}
	cfg.Policy.MaxNudges = *maxNudges
	cfg.MaxStepsPerTurn = *maxSteps
	cfg.Route = !*noJev && !*noRoute
	cfg.Snapshot = *fast || *snapshot
	cfg.Batch = *fast || *batch
	cfg.EarlyStop = !*noJev && (*fast || *earlyStop)
	cfg.Race = *raceFlag
	if cfg.Race == 0 {
		cfg.Race = 1
		if *fast {
			cfg.Race = 3
		}
	}
	cfg.Speculate = cfg.Route && (*fast || *speculate)
	cfg.CrossCheck = cfg.Batch && cfg.EarlyStop && (*fast || *crossCheck)
	cfg.Heartbeat = !*noJev && (*fast || *heartbeat)
	if *fastModel != "" {
		if err := addFastModel(ctx, &cfg, *fastModel, *fastProv); err != nil {
			return fail(err)
		}
		cfg.FastAll = *fastAll
		cfg.FastEffort = checkpoint.Effort(*fastEffort)
	}
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

	if *prompt == "" && *seedPath == "" {
		if err := tui.Run(ctx, cfg, path); err != nil {
			return fail(err)
		}
		return exitDone
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

func buildConfig(ctx context.Context, dir, modelName, reasoning string, checkpoints bool) (kernel.Config, error) {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return kernel.Config{}, errors.New("OPENROUTER_API_KEY is not set")
	}
	provider, err := openrouter.New(openrouter.WithAPIKey(key))
	if err != nil {
		return kernel.Config{}, err
	}
	model, err := provider.LanguageModel(ctx, modelName)
	if err != nil {
		return kernel.Config{}, err
	}
	var jc *jev.Client
	if checkpoints {
		if jc, err = jev.NewFromEnv(); err != nil {
			return kernel.Config{}, err
		}
	}
	effort := func(e checkpoint.Effort) fantasy.ProviderOptions {
		return openrouter.NewProviderOptions(&openrouter.ProviderOptions{
			Reasoning: &openrouter.ReasoningOptions{Effort: new(openrouter.ReasoningEffort(e))},
		})
	}
	return kernel.Config{
		Model:           model,
		ModelName:       modelName,
		ProviderOptions: effort(checkpoint.Effort(reasoning)),
		EffortOptions:   effort,
		Jev:             jc,
		Dir:             dir,
		Policy:          checkpoint.DefaultPolicy,
		RoutePolicy:     checkpoint.DefaultRoutePolicy,
		StepPolicy:      checkpoint.DefaultStepPolicy,
		Checkpoints:     checkpoints,
	}, nil
}

// addFastModel adds a second model on OpenRouter, pinned to the given
// providers in order, with fallbacks allowed.
func addFastModel(ctx context.Context, cfg *kernel.Config, name, providers string) error {
	key := os.Getenv("OPENROUTER_API_KEY")
	provider, err := openrouter.New(openrouter.WithAPIKey(key))
	if err != nil {
		return err
	}
	model, err := provider.LanguageModel(ctx, name)
	if err != nil {
		return err
	}
	var order []string
	for p := range strings.SplitSeq(providers, ",") {
		if p = strings.TrimSpace(p); p != "" {
			order = append(order, p)
		}
	}
	cfg.FastModel, cfg.FastModelName = model, name
	cfg.FastOptions = func(e checkpoint.Effort) fantasy.ProviderOptions {
		return openrouter.NewProviderOptions(&openrouter.ProviderOptions{
			Reasoning: &openrouter.ReasoningOptions{Effort: new(openrouter.ReasoningEffort(e))},
			Provider:  &openrouter.Provider{Order: order, AllowFallbacks: new(true)},
		})
	}
	return nil
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
			fmt.Printf("\n→ %s %s\n", e.Tool, clip(e.Input, 200))
		case kernel.EventToolResult:
			fmt.Printf("  %s\n", clip(strings.ReplaceAll(e.Text, "\n", " ⏎ "), 200))
		case kernel.EventDecision:
			d := e.Decision
			fmt.Printf("◆ jev %s: %s (%s) %dms\n", d.Checkpoint, d.Action, d.Rule, d.LatencyMS)
		case kernel.EventRoute:
			r := e.Route
			fmt.Printf("◆ jev route: complexity %.2f → %s effort %dms\n", r.Score, r.Effort, r.LatencyMS)
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

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "girdle:", err)
	return exitError
}
