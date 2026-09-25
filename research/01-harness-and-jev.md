# Girdle: what a coding-agent harness does, and how much a decision engine can run

Research notes, 25 Sep 2026. No production code yet. The experiments are in `research/experiments/`.

---

## 0. Summary

1. **Jev can't be the LLM, but it can be the scheduler.** Jev (TypeSafe AI, released 15 Sep 2026) is a *System One* model. It takes a `state` and a map of typed questions and returns calibrated probabilities in roughly 50–300 ms. There are three question types:
   - `choice`: sorts the state into one of up to 255 categories, with the full probability distribution
   - `score`: places it on an ordered scale, with a distribution over levels
   - `noul`: a yes/no probability

   The Choice distribution is the most useful output for a harness. It lets you rank options, shortlist them, sum probability over groups of categories, and weight categories by cost. See §1a. It doesn't generate text, so it can't write code, plans, summaries or replies. What it can do is make almost every *control-flow* decision a harness makes. Today those decisions come from the frontier LLM (expensive and slow), from brittle heuristics (hashes, regexes, token counters) or from the human (permission prompts).
2. **Architecture: an event-driven kernel where code owns control flow and Jev answers questions at each transition.** LLMs become workers the kernel calls. They are no longer the loop. TypeSafe's own docs say this: System One is "for building AI-powered software, not agents… it does not choose its own next action". The *harness* is the part that turns it into an agent, and that part is the new thing you'd be building.
3. **The inner coding loop stays LLM-driven.** Choosing *which* edit to make next is System Two work. Jev wraps that loop at checkpoints: before a tool call, after a tool result, at turn end, on every incoming event, and on every background-agent report.
4. **Build the kernel yourself and borrow the commodity layers. Don't fork a big harness.** Grok Build (~1.1M lines, source dump, no PRs) and Codex (~885k lines) are references, not starting points. OpenCode (~446k) is a large, fast-moving platform. Crush uses the **FSL-1.1-MIT** licence, which bars a competing commercial product for two years. Pi's loop file is about 790 lines (Pi overall is about 173k; the loop is under 3k lines in every harness) and already exposes the exact hook points a decision engine needs, and its provider layer (`pi-ai`) is MIT and reusable as a library.
5. **Local Jev-compatible models exist and are usable for development now.** Kev (Jared Palmer, Apache-2.0) serves the same `/v1/systemone` API, so the TypeSafe SDK works against it unchanged. On this M3 Pro, Kev-0.8B answered 43 hand-labelled harness decisions at a **47 ms median** with **79%** accuracy. Scoring only the top category hid how much the model knows. Using the full distribution, one risk question separated "needs approval" commands at AUC 0.94 as a Choice and 0.97 as a damage Score, where argmax had been 50% correct. Complexity routing as a Score had rank correlation 0.84. Kev-4B didn't fit in 18 GB alongside a normal desktop workload (§6).

---

## 1. What Jev is and isn't (facts from primary sources)

| | |
|---|---|
| Endpoint | `POST https://api.typesafe.ai/v1/systemone` with body `{state, model, questions}` |
| State | string, JSON object or array. Text only (no images) |
| Questions | a named map. `choice` (≤255 options, returns choice, probabilities and confidence), `score` (2–10 ordered levels, returns an expected level, probabilities and confidence), `noul` (returns P(yes)) |
| Isolation | Questions can't see each other. The state is encoded once and each question branches off it, so "speculative fan-out" (asking everything in one call) is cheap: about 5.7× faster and 6× fewer tokens than 10 separate calls |
| Context | 64k tokens per request; 32k for state plus the longest question. Accuracy degrades well before that ("context rot") |
| Latency | "most queries ~100 ms". About 90–150 ms for a single question. A pi-jev fixture run measured 126 ms median |
| Price | **$0.042 per million input tokens**, output free |
| Limits | 250k tok/s, 1,200 req/min, "adjusting dynamically" while they scale. Waitlist |
| Model | `jev-1.13.0` (alias `jev-latest`). Pin the version if you tune thresholds against it. No per-customer fine-tuning |
| Training | "RLCD", reinforcement learning for calibrated decisions (proper-scoring-rule rewards) |

**Known weaknesses** (TypeSafe's own "jaggedness" page for 1.13), each of which shapes the harness design:
- **Literal reading.** It answers the question as written. Put the intent in the wording.
- **No maths, counting or date comparison.** Keep those in code.
- **Indirection hurts.** Point questions at named paths in the state (for example ``Is `tool_call.command` …``).
- **Large, irrelevant state hurts.** Building the state becomes the real engineering work.
- **Adversarial content can move the answer.** It's not a prompt-injection-proof judge.
- **No structural invariants.** P(A) + P(¬A) ≠ 1 across separate questions, and a Noul and an equivalent Choice give different numbers. Thresholds don't transfer between question forms.
- **Not generative.** Extraction works by generating candidates in code or with an LLM, then asking Jev to *pick* one.

### 1a. Use the whole distribution, not just the top answer

A Choice answer is a probability distribution over *your* categories, not a single label. Most of its value for a harness comes from what code does with that distribution:

| Pattern | What code computes | Harness use |
|---|---|---|
| **Argmax + confidence** | top category; `confidence` = how peaked the distribution is | routing when the categories are cleanly separable |
| **Group mass** | Σ P(categories in a group), e.g. P(destructive) + P(external) | "needs approval?" gates. Categories can be fine-grained while the policy stays coarse |
| **Expected cost** | Σ P(c) × cost(c) | risk budgets and auto-approve thresholds that encode *your* cost of mistakes |
| **Rank / shortlist** | sort by probability, take top-k or cumulative mass ≥ 0.9 | tool, skill, agent, model or file selection. The shortlist goes to a rerank question or to the LLM as a hint |
| **Margin / entropy** | p1 − p2, or entropy | "ambiguous between X and Y" → ask a clarifying question naming exactly X and Y |
| **Hierarchical** | Choice at the top level, then a Choice within the winning branch | more than 255 options, or large tool/skill catalogues (TypeSafe's hierarchical-classification cookbook) |
| **"None of these" option** | an explicit catch-all category | stops a relative Choice from forcing a match. Otherwise pair it with an absolute Noul ("does anything fit?") as the skill-suggestion cookbook does |

When to use which type: a **Choice** is relative and answers *which*. Its probabilities always sum to 1, so it can't say "none of them". A **Noul** is absolute and answers *whether*. A **Score** is for ordered magnitude (urgency, complexity, damage). Thresholds don't transfer between types (see "structural invariants" above).

A **Score** answer has three parts. `score` is the probability-weighted expected level, and it can land between levels, e.g. 1.4 on a 0–3 scale. `probabilities` gives one probability per level. `confidence` measures how peaked that is. What code does with it:

| Pattern | What code computes | Harness use |
|---|---|---|
| **Threshold the expected level** | `score ≥ t`, or cut points for tiers | model tier from complexity, approval from damage, notify-now from urgency. Jev says to use Score for thresholds, not to reconstruct exact numbers between levels |
| **Tail mass** | Σ P(level ≥ k) | "any real chance this is severe?" Catches split distributions that a middling expected level would hide (e.g. `git push --force` → [0.19, 0.24, 0.29, 0.27]) |
| **Sort by score** | rank items by expected level | prioritise background reports, events and review queues |
| **Composite** | weighted sum of several atomic Scores and Nouls in code | effort or budget estimates (TypeSafe's composite-scoring pattern) |

Use a **Score** instead of a Choice whenever the categories are *ordered*. The model then puts probability on neighbouring levels rather than spreading it over unrelated categories, and code gets a single sortable number.

**Local, open-source Jev-compatible options:**

| Project | Base / size | Licence | API-compatible | Notes |
|---|---|---|---|---|
| **Kev** (jaredpalmer/kev, ★6.9k) | Qwen3.5 LoRA + pointer head: 0.8B / 4B / 9B / 27B | Apache-2.0 | **yes**, TypeSafe SDK works as-is | Best documented. Calibrated temperature per checkpoint. Fine-tuning on your own labels costs about $1 on Modal. Trained on states ≤384 tokens. Kev-27B is within about 1 point of Jev on unseen sources; 4B/9B within about 4 |
| **Laya** (★24k) | ModernBERT-large 421M; mmBERT 322M multilingual | Apache-2.0 | own API, same primitives | Very fast (33 ms on a T4). 512–1k context (8k for multilingual). Weak zero-shot (0.36 vs 0.77 fine-tuned on their benchmark) |
| SemIf-OpenJev (★4.3k) | Qwen3.5-4B / MiniCPM 2B | MIT | — | Logit scoring, WebGPU |
| openjev (★414) | DiffusionGemma 26B-A4B | Apache-2.0 | yes | Image input |
| localjev (githubnext) | configurable upstream LLM | MIT | yes | TypeScript/Bun shim that emulates the API over an ordinary LLM |
| jeff | GliFormer 400M | MIT | yes | ONNX, CPU |

Kev's API compatibility makes "hosted or local" a **config switch** (`base_url`), which is what you want for development and for users who won't use a hosted provider.

---

## 2. Anatomy of a coding-agent harness

These are the responsibilities in production harnesses (Claude Code, Codex, OpenCode, Crush, Grok Build, Pi). **Phase** is when you'd need it (v0 = first usable headless build). **Jev** says what a decision engine can do there: **R** = replaces an LLM, heuristic or human decision; **A** = assists (filters or ranks for code or an LLM); **–** = no role (plumbing or generation).

### A. Surfaces
| Responsibility | Phase | Jev |
|---|---|---|
| Headless / print mode (`-p`), JSON event stream on stdout | v0 | – |
| Interactive TUI: streaming render, diff view, approval prompts, input queue, interrupts | v1 | – |
| Server / RPC protocol so other clients can drive the agent (Codex `app-server`, OpenCode client/server, Pi RPC, ACP for editors) | v1 | – |
| IDE, desktop, web clients | v3 | – |

### B. Sessions and state
| Responsibility | Phase | Jev |
|---|---|---|
| Append-only event log per session (JSONL or SQLite). Everything else is a projection of it | v0 | – |
| Resume, fork, branch, rewind | v1 | – |
| File checkpoints / undo (shadow git snapshots, as in OpenCode's `snapshot` and Codex's ghost commits) | v1 | – |
| Export and share transcripts | v2 | – |

### C. Model layer
| Responsibility | Phase | Jev |
|---|---|---|
| Provider abstraction: streaming, tool-call deltas, reasoning traces, images, stop reasons. Each provider leaks differently (Zechner's main lesson from building `pi-ai`) | v0 | – |
| Prompt caching (stable prefix, variable suffix), token and cost accounting | v0 | – |
| Retries, backoff, rate limits, overload fallback | v0 | – |
| Auth: API keys, OAuth subscriptions, credential storage | v1 | – |
| **Model routing**: which model or effort level for this turn | v1 | **R**: complexity Choice, plus fallback on low confidence |

### D. Context assembly
| Responsibility | Phase | Jev |
|---|---|---|
| System prompt, environment block (cwd, OS, date, git status) | v0 | – |
| Hierarchical memory files (`AGENTS.md` / `CLAUDE.md`) | v0 | – |
| Tool schema exposure. Fewer tools means better behaviour, and schemas cost tokens every turn | v0 | **A**: pick which tool *groups* to expose this turn |
| Skills / progressive disclosure: index in the prompt, body loaded on demand | v2 | **R/A**: TypeSafe's skill-suggestion cookbook ranks 182 skills, then reranks the top 3. Wrong loads fell from 16.8% to 7.3% |
| @-mentions, attachments, images | v1 | – |
| Mid-conversation system reminders (todo state, file changed on disk, and so on) | v1 | **A**: "is this reminder relevant now?" |

### E. The loop
| Responsibility | Phase | Jev |
|---|---|---|
| Turn loop: stream → tool calls → execute → append results → repeat | v0 | – (the kernel) |
| **Ingress triage**: what kind of message is this (question, change, command, plan, chit-chat, slash command), and does it need the LLM at all? | v0 | **R** |
| **Steering vs follow-up**: the user typed mid-turn. Interrupt now, inject at the next boundary, or queue? | v1 | **R** |
| **Turn-end decision**: done, asking the user, stalled, claimed work it didn't do, needs a nudge? | v0 | **R** (today it's "no tool calls ⇒ stop") |
| **Stuck/loop detection** | v1 | **R** (Crush uses a SHA-256 over the last 10 tool calls, so it only catches *identical* loops) |
| Budgets: max turns, cost, wall time | v0 | – (code) |
| Malformed or truncated tool-call recovery | v0 | – |
| Cancellation that propagates into running tools | v0 | – |

### F. Tools
| Responsibility | Phase | Jev |
|---|---|---|
| `read`, `write`, `edit` (str-replace or `apply_patch`), `bash`. Pi ships only these four, with a prompt under 1k tokens | v0 | – |
| `grep`, `glob`, `ls` (read-only variants) | v0 | – |
| Output truncation and head/tail windows; timeouts | v0 | **A**: relevance-filter long output line by line (the `semantic_find` cookbook) |
| PTY and background processes (dev servers, watchers) | v2 | **A**: "does this log line need attention?" |
| Web fetch/search, LSP diagnostics, todo, ask-user, image view | v2 | – |
| **Deterministic fast paths**: "run the tests", "git status" dispatched straight to code with closed-set arguments (the function-calling cookbook) | v2 | **R** (skips a whole LLM round trip) |

### G. Safety
| Responsibility | Phase | Jev |
|---|---|---|
| Permission modes (ask, auto-edit, YOLO) and allow/deny rules per shell segment | v0 | – (code first; rules are exact) |
| **Per-call approval judgement**: in scope? authorised by the user's words? hazardous? | v1 | **R**. This is where Codex's `guardian` uses an LLM reviewer. See the lessons in §5 |
| OS sandbox (Seatbelt, Landlock/bubblewrap, containers), network policy | v1 | – |
| Prompt-injection screening of tool output (READMEs, web pages, issue text) | v1 | **R/A**, with the caveat that Jev itself isn't adversarially robust. Taint the session after a hit (jev-sentinel's approach) |
| Secret scrubbing before anything leaves the machine (including to Jev) | v1 | – (code; regex plus entropy) |
| Project trust (untrusted repo ⇒ restricted mode) | v1 | – |

### H. Context management
| Responsibility | Phase | Jev |
|---|---|---|
| Overflow detection by token count | v0 | – (code) |
| **What to drop or keep** when pruning tool outputs | v1 | **R**: relevance Noul per chunk against the pinned task |
| Summarising compaction | v1 | – (generative: LLM) |
| Long-term memory writes ("is this worth remembering?") | v3 | **R** gate plus LLM writer |

### I. Multi-agent and background work
| Responsibility | Phase | Jev |
|---|---|---|
| Subagents: isolated context, filtered tools, depth limit | v2 | **R**: "delegate this?" and "to which agent profile?" |
| Background tasks and a scheduler (cron, "check CI in 10 min") | v2 | **R**: "does this event need a human now, later, or never?" |
| Parallel worktrees, merge-back | v3 | **A**: conflict triage |
| External event ingress (CI failed, PR comment, Slack mention, file watcher) | v3 | **R**: route to the right session or agent, set priority, decide whether to wake the user |
| Supervising background agents ("is agent 3 still making progress?") | v2 | **R**, the same stuck/done questions asked from outside |

### J. Extensibility
| Responsibility | Phase | Jev |
|---|---|---|
| Hooks (pre/post tool, prompt submit, stop). OpenCode exposes `tool.execute.before/after`, `permission.ask`, `chat.messages.transform`. Pi exposes `beforeToolCall`, `afterToolCall`, `transformContext`, `getSteeringMessages`, `getFollowUpMessages` | v1 | Jev decisions *are* hooks in this design |
| MCP client (stdio/HTTP, OAuth, tool-list changes) | v2 | **A**: pick which MCP servers' tools to expose |
| Slash commands, custom agents, plugins, skills | v2 | **R**: fuzzy command/skill matching |

### K. Observability and evaluation
| Responsibility | Phase | Jev |
|---|---|---|
| Structured traces of every model call, tool call **and decision** (state, answers, model version, latency) | v0 | the decision log is the training set |
| Cost dashboards, OpenTelemetry | v2 | – |
| Offline eval harness (terminal-bench / SWE-bench style, plus a **decision eval set**) | v1 | measures Jev versus local models versus the LLM |

### L. Distribution
Install/update, layered config (global → project → env → flags), telemetry opt-in, crash reports. All v1–v2, and none of it involves Jev.

**Where the size comes from.** The *loop* is small in every harness. Measured as non-blank, non-comment lines:

| Harness | Loop file(s) | Loop | Whole codebase | Biggest non-loop components |
|---|---|---|---|---|
| mini-swe-agent | `agents/default.py` | ~100 | ~4k | – |
| Pi | `agent/src/agent-loop.ts` | ~790 | ~173k | coding-agent app 77k, providers (`pi-ai`) 26k, TUI 16k |
| Crush | `internal/agent/{agent,coordinator}.go` | ~3.1k | ~76k | – |
| OpenCode | `session/{prompt,processor,llm}.ts` | ~2.5k | ~446k | web/desktop app 136k, hosted console 39k, SDK 27k |
| Codex | `core/src/session/turn.rs` | ~2.8k | ~885k | **TUI 232k**, app-server + protocol 72k, Windows sandbox 21k, network proxy 20k |
| Grok Build | `…/acp_session_impl/sampler_turn.rs` | ~1.9k | ~1.1M | **TUI/pager 314k**, tools 117k, workspace 79k (a monorepo dump) |

In every harness the loop is under 0.5% of the code. The rest is product scope: the TUI (the largest single item), extra interfaces (IDE and desktop protocols, web apps, SDKs), per-OS sandboxing, the tool implementations and their edge cases, persistence, config, auth, telemetry, and extension systems (MCP, plugins, hooks, subagents). Rust also tends to take more lines than TypeScript for the same behaviour (not measured). Pi is small because it deliberately leaves out MCP, subagents, permissions, sandboxing and plan mode, and ships 4 tools.

---

## 3. Decision inventory: everything the kernel asks Jev

This inventory is the product. Each row is a question set that fires on a kernel event. Answers plus confidence go into a **policy table in code** that picks the action.

| Event | Questions (type) | Policy in code (what uses the distribution) |
|---|---|---|
| **User message arrives** | intent (Choice: question / change / command / plan / review / chit-chat / other), complexity (Score), which skill or slash command (Choice over a shortlist, plus a "none" option), background-able? (Noul) | argmax intent → worker; complexity → model tier; **margin** between the top 2 intents < 0.15 → ask a clarifying question naming both; skill **top-3** → rerank question |
| **User types mid-turn** | kind (Choice: stop / redirect / add constraint / unrelated / answer to a pending question) | stop → cancel now; redirect → inject at the next boundary; unrelated → queue |
| **Before a tool call** | effect (Choice: read-only / local write / destructive / external / credentials), authorised by the user's words? (Noul), in scope of the pinned task? (Noul) | **group mass** P(destructive + external + credentials) or **expected cost** vs. a threshold, scaled down when authorisation is high. Hard-deny lists and allow rules run *before* Jev |
| **After a tool result** | outcome (Choice: success / env or network / code error / test failure / permission / timeout), contains instructions aimed at the agent? (Noul), relevance per chunk (Noul) | env/network with high confidence → retry without the LLM; injection → taint the session; low relevance → prune |
| **LLM turn ends (no tool calls)** | status (Choice: done / needs user / in progress / stuck), claims unsupported by the tool results? (Noul) | argmax with confidence ≥ τ → act; **ambiguous done vs needs_user** → surface the message to the user rather than auto-continue; in_progress → nudge to continue |
| **Choosing tools / context** | which tool groups, MCP servers or files matter for this step (Choice **ranked**, hierarchical if large) | expose the top-k tool groups; attach the top-ranked files; the LLM still makes the actual call |
| **Every N steps (heartbeat)** | trajectory (Choice: progressing / looping / drifting / blocked), what to keep (Nouls per chunk) | looping → hint or escalate the model; drifting → re-pin the task; blocked → ask the user |
| **Background agent reports** | status (Choice: progress / blocked / finished / failed), urgency (Score) | urgency score × status → notify now, add to a digest, or stay silent |
| **External event** (CI, PR comment, watcher) | owner (Choice over live sessions/agents, plus "new session" and "ignore"), urgency (Score) | route to the argmax owner if confident; otherwise ask the user with the top 2 |

Fan-out makes this cheap. All the questions for one event go in **one** request (one state encode). At $0.042/Mtok, a 2k-token state costs about $0.0001 per event, so 1,000 decisions come to about $0.10.

---

## 4. Architecture sketch

```
            ┌───────────── events ─────────────┐
 user msg ──┤ tool result · turn end · timer   │
 CI/webhook ┤ background-agent report · steer  │
            └──────────────┬───────────────────┘
                           ▼
                 ┌──────────────────┐     append-only
                 │  Event log (JSONL│◀─── every event,
                 │  / SQLite)       │     decision, action
                 └────────┬─────────┘
                          ▼
             ┌─────────────────────────┐
             │ State builder (per event│  ← the hard part: small, relevant,
             │ type → small JSON state)│    named fields, secrets scrubbed
             └────────┬────────────────┘
                      ▼
             ┌─────────────────────────┐   backends behind one interface:
             │ Decider (System One)    │   TypeSafe Jev │ Kev local │ Laya │
             │ fan-out question sets   │   LLM-as-judge fallback │ rules
             └────────┬────────────────┘
                      ▼  answers + confidence
             ┌─────────────────────────┐
             │ Policy table (code)     │  act / verify / defer ladders,
             │ thresholds per action   │  thresholds pinned per model version
             └────────┬────────────────┘
      ┌───────────────┼─────────────────┬────────────────┐
      ▼               ▼                 ▼                ▼
 LLM worker      tool executor     ask / notify user   spawn / stop agent
 (System Two:    (sandboxed)                           compact context
 edits, plans,
 summaries)
```

Design principles:
- **Kernel = reducer over events.** `(state, event) → decisions → actions → new events`. Deterministic given the decision answers, so it can be replayed and tested with recorded answers.
- **Decider is an interface, not a vendor.** Use the same question schema against hosted Jev, local Kev, or a cheap LLM with structured output as the fallback. That makes the "users who won't use a hosted provider" case a config value.
- **Confidence ladders, not single thresholds.** High confidence → act. Medium → verify (a second question, more context, or an LLM judge). Low → the human. Risk sets the bar: reads are lenient, destructive calls strict.
- **Shadow mode first.** Log what Jev *would* have decided alongside what actually happened before letting it gate anything. This is how pi-jev ships by default.
- **Every decision is a training example.** Log `{state, questions, answers, model_version, outcome}`. Kev fine-tunes on exactly that JSONL format, so you get a local model tuned to your harness.
- **Code before model.** Anything a parser, regex, token counter or allowlist can decide exactly never goes to Jev (TypeSafe's first rule, and confirmed by our risk experiment).

---

## 5. Lessons from people who already put Jev in a harness

Several projects already bolt Jev onto existing harnesses as **sidecars**. Your idea (Jev *as* the loop) goes further, but their measurements carry over:

- **pi-jev-permit**: one "should this be allowed?" Noul refused **100 of 125** commands under a clearly authorising instruction, and 44 of those landed between 0.40 and 0.59 ("can't tell"). Splitting it into *hazard*, *critical* and *authorised by the user's words* and combining them in code fixed it. Layering: hard-deny list → user rules → read fast path (0 ms) → same-turn repeat → Jev. A missing answer counts as a block ("an unanswered question is never consent").
- **pi-jev**: phrasing matters enormously. "Cannot be recovered from version control" scored `rm -rf src && git push --force` at 0.77 because "it's in git" is a reasoning path the model takes. Asking plainly "is this destructive?" separated safe from unsafe at 0.03 vs 0.99. Over 53 fixtures: 126 ms median, about 490 tokens per request.
- **jev-sentinel**: re-asks with more context when Jev signals it's unsure. A delete scored 83% on-task with no conversation and 97% after adding earlier messages. It pins a task so "on task" is judged against the pin rather than the drifting chat. It taints the session after an injection hit.
- **LangChain's Jev harness post**: the two middleware patterns they ship are model routing (complexity → model) and auto mode (pre-screening tool calls).

---

## 6. Local experiments (this machine: M3 Pro, 18 GB, Kev on MLX)

`experiments/harness_decisions_eval.py` covers 43 hand-labelled cases across 9 decision families, one question per request, then a 6-question fan-out.
`experiments/risk_decomposed.py` compares shell-risk as one 4-way Choice against 3 atomic Nouls combined in code.
`experiments/choice_distributions.py` applies distribution-aware policies to the same Choice (group mass, expected cost), a 4-way turn-status Choice, and tool routing over 12 tools.
`experiments/score_scales.py` covers a damage Score for shell commands, a complexity Score for model routing, and an urgency Score for background reports.

### Kev-0.8B
| Family | Correct | Notes |
|---|---|---|
| intent | 7/8 | missed "add a --json flag" (called it chit-chat, conf 0.04) |
| complexity | 4/6 | under-rates difficulty |
| **risk (4-way Choice)** | **4/9** | called `ls -la` and `git log` "destructive" |
| done | 4/4 | but p(yes) sat at 0.36–0.53, so the margins are thin |
| asks_user / stuck / background / relevant | 12/12 | |
| injection | 3/4 | one false positive on a README |
| **overall** | **34/43 = 79%** | **47 ms median, 62 ms p90** per single-question request |
| confidence ≥ 0.6 | 9/9 correct | but only 21% of cases reached it |
| fan-out, 6 questions | 219 ms | |

**Shell-command risk, four ways** (12 commands; the gate is "ask before running?", true for destructive or external):

| Method | Result | Notes |
|---|---|---|
| 4-way Choice, **argmax** | 6/12 | the top category was nearly always "destructive" (~0.37–0.43) |
| 3 Nouls combined in code | 9/12 | |
| **Same Choice, group mass** P(destructive)+P(external) | **AUC 0.94**, 11/12 at t=0.64 | separable; only `git reset --hard` (0.47) was missed |
| Same Choice, expected cost (weights 0/1/3/3) | AUC 0.91, 11/12 | |
| **Damage Score (0–3)** | **AUC 0.97**, rank correlation with true level 0.90 | `rm -rf` and `docker prune` put most mass on "serious"; reads sat at ~0.5 |

Thresholds marked "at t=" were picked on the same 12 items, so they're optimistic. AUC is the threshold-free measure.

**Other distribution results:**
| Decision | Type | Result |
|---|---|---|
| Model routing (complexity, 9 requests) | Score 0–2 | rank correlation 0.84; 8/9 tiers with in-sample cut points (0.77 / 1.17). The earlier 3-way Choice got 4/6 |
| Turn-end status (8 turns) | Choice: done / needs user / in progress / stuck | 6/8. Misses were low-confidence (0.12, 0.31), so the confidence floor routes them to the user |
| Tool routing (8 steps, 12 tools) | Choice, ranked | top-1 6/8, **top-3 8/8**. The misses were near-neighbours (glob vs grep, git_status vs git_diff), which suits shortlist → LLM |
| Background-report urgency (6 reports) | Score 0–2 | rank correlation 0.48, weak at 0.8B. "Blocked on credentials" scored only 0.95 |

Caveat on Kev's Score `confidence`: it stayed at 0.7 for the near-flat `git push --force` distribution. Kev notes its confidence formula approximates TypeSafe's (which isn't public), so work from the level probabilities or tail mass rather than Kev's `confidence`.

### Kev-4B
Downloaded (about 8 GB of bf16 weights) and served via MLX. On this 18 GB machine, alongside a normal desktop workload, it **thrashed swap** (27 of 28 GB used) and took about 80 s per request. I stopped it after 7 requests, so there are no accuracy numbers. Kev's README quotes 721 ms for 5 questions on an Apple M5 with 32 GB. **Practical takeaway:** on a laptop with 16–18 GB, Kev-0.8B is the local dev model. Kev-4B needs a 32 GB Mac, or a GPU box / Modal endpoint (scales to zero, about 35 s cold start).

### What the experiments say
- Latency is a non-issue locally, even at 0.8B. A decision costs about 50 ms, which is invisible next to a multi-second LLM turn.
- **Read the distribution, not just the top answer.** The same risk call went from 50% correct (argmax) to AUC 0.94 (group mass) and 0.97 (damage Score). Pick the question type to match the decision: Choice for *which*, Score for *how much*, Noul for *whether*. Then write the policy over the probabilities.
- **Ordered questions should be Scores.** Damage and complexity both did best as Scores.
- **Ranking is enough for selection.** Tool routing was only 6/8 top-1 but 8/8 top-3, so shortlist with Jev and let the LLM or a rerank question pick.
- Code still goes first for exact rules (hard-deny lists, allowlists, parsers). Jev covers the long tail and the *authorisation* question ("did the user ask for this?"), which a parser can't answer.
- Semantic, conversational judgements (done / stuck / asks-user / background / relevance) are where even a tiny local model is already useful. They're also the decisions harnesses currently get wrong with heuristics.
- Treat the 0.8B model as a dev stand-in. For real gating use hosted Jev or Kev-4B or larger, and measure thresholds on your own decision log.

---

## 7. Build vs fork

Measured locally (non-blank, non-comment source lines, excluding tests and vendored code):

| Harness | Lang | Size | Licence | Core loop | Verdict |
|---|---|---|---|---|---|
| **Grok Build** (xai-org) | Rust | **~1.1M** | Apache-2.0 | deep in a Rust workspace | ✗ A source-transparency dump synced from a monorepo, with no external PRs. Forking means owning a million lines with no upstream collaboration |
| **Codex** (openai) | Rust | ~885k (TUI 227k, core 100k) | Apache-2.0 | `core/` | ✗ as a base. ✓ as a **reference** for sandboxing (Seatbelt/Landlock/Windows), `apply_patch`, the `guardian` auto-reviewer and the `app-server` protocol |
| **OpenCode** (anomalyco) | TS/Bun | ~446k (agent pkg 71k) | MIT | `packages/opencode/src/session` | ~ Great plugin hooks (`permission.ask`, `tool.execute.before`), so it's a good *test bed* for Jev decisions. Forking means tracking a very fast upstream and a large product surface |
| **Crush** (charmbracelet) | Go | ~76k | **FSL-1.1-MIT** | `internal/agent` | ✗ for a product. FSL forbids a "Competing Use" (a commercial substitute) until it converts to MIT two years after each release |
| **Pi** (earendil-works, ★109k) | TS | ~173k total. **agent loop ~790 lines**, `pi-ai` 26k, `pi-tui` 16k | MIT | `packages/agent/src/agent-loop.ts` | ✓ **as libraries**. The loop already has `getSteeringMessages`, `getFollowUpMessages`, `transformContext`, `beforeToolCall`, `afterToolCall`, the decision points in §3. Minimal by design (4 tools, <1k-token prompt, no MCP, no subagents, YOLO) |
| **mini-swe-agent** | Python | ~4k (loop ~100) | MIT | `agents/default.py` | ✓ for **evals**: scores >74% on SWE-bench Verified with a trivial loop. Useful for A/B testing Jev decisions on benchmarks |

**Recommendation: write the kernel from scratch and depend on commodity layers.**
- *The kernel is the novel part and it's small.* It's an event log, a state builder, a Decider interface, a policy table and a worker supervisor. Forking would mean fighting someone else's LLM-driven loop to put Jev in charge. In all five big harnesses the LLM *is* the control flow, so you'd be inverting their core assumption.
- *Don't rebuild provider plumbing.* `@earendil-works/pi-ai` (MIT) already handles the leaky provider abstractions. Optionally use `pi-tui` for the TUI and `pi-agent-core` as the *inner* LLM worker loop, driving its hooks from your kernel.
- *Validate before you build.* A few days prototyping the §3 decisions as a **Pi extension or OpenCode plugin**, in shadow mode on your own daily work, gives you a real decision log and real thresholds before the kernel exists.
- *Steal designs from Codex*: sandbox policies, `apply_patch`, and the approval-reviewer idea (the LLM there becomes Jev here).

---

## 8. Staged plan (sequencing only, no code)

| Stage | Scope | Jev decisions introduced |
|---|---|---|
| **v0: headless kernel** | event log, `pi-ai` worker, 4 tools, print mode, budgets, Decider interface with Jev and Kev backends, decision log | ingress intent and complexity routing; turn-end (done / asks-user / unsupported claims) — **shadow only** |
| **v1: usable daily** | TUI, sessions/resume, checkpoints/undo, permission rules, sandbox, compaction, decision-eval set | tool gating (hazard × authorisation × scope) enforced; stuck detection; output failure-kind; injection taint; relevance pruning |
| **v2: background** | subagents, background tasks, scheduler, supervisor, skills, MCP | delegate?/which profile; supervisor progress checks; notify-or-not; skill/tool-group selection; deterministic fast paths |
| **v3: ambient** | external event ingress (CI, PRs, chat), worktrees, memory | event routing and priority; memory-write gate; conflict triage |
| **continuous** | fine-tune Kev on your own decision log; A/B each decision on mini-swe-agent or terminal-bench | swap hosted and local per decision family |

---

## 9. Risks

- **Vendor maturity.** Jev is 10 days old, closed, waitlisted, with rate limits that change without notice. Model aliases move, and thresholds are tuned per version. Mitigations: the Decider interface, pinned versions, and Kev as the local fallback.
- **Privacy.** Every decision sends state to TypeSafe (ZDR is enterprise-only). Scrub secrets and truncate in code, as pi-jev does (400-character argument caps).
- **Adversarial state.** Jev can be steered by injected text in tool output. Never let a single Jev answer be the *only* barrier to an irreversible action. Hard-deny lists and the sandbox sit underneath.
- **State building is the real work.** Small, named, relevant states are what make Jev accurate, and designing a state per event type is where the engineering goes.
- **Latency stacking.** 4–6 gated checkpoints per step at about 100–300 ms hosted adds up. Fan out per event, run gates concurrently with LLM streaming where possible, and use code fast paths for obvious cases.
- **Local model quality varies a lot.** Kev-0.8B ranks risk and complexity well when you use its distributions, but its urgency judgements were weak and its confidence values are approximations. Kev was trained on ≤384-token states.

---

## 10. Open questions (these change the design)

1. **Who is it for first?** Your own daily driver (terminal, single user) or a product others install? That decides TUI-first vs headless/server-first, and how much safety surface v1 needs.
2. **How ambient?** Is "background agents" a user-launched parallel task, or a long-running daemon reacting to CI/PR/chat events? The daemon version makes the event kernel central from day one.
3. **Language.** TypeScript/Bun gets you `pi-ai`, the TypeSafe JS SDK and OpenCode/Pi plugin prototyping. Rust matches Codex/Grok Build for sandboxing and performance. Effect-TS would suit an event-sourced kernel with typed errors, if you want it.
4. **Hosted Jev access.** Are you on the TypeSafe waitlist or OpenRouter? None of the experiments here touched hosted Jev.

---

## Sources

- TypeSafe docs: [System One](https://docs.typesafe.ai/concepts/system-one), [API](https://docs.typesafe.ai/api), [Models & pricing](https://docs.typesafe.ai/models), [Jev 1.13 jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13), [Confidence](https://docs.typesafe.ai/confidence), [How to build](https://docs.typesafe.ai/concepts/how-to-build-with-system-one), [Jev with coding agents](https://docs.typesafe.ai/introduction/coding-agents), [Skill suggestion cookbook](https://docs.typesafe.ai/cookbooks/skill_suggestion), [Function calling cookbook](https://docs.typesafe.ai/cookbooks/function_calling), [llms.txt index](https://docs.typesafe.ai/llms.txt)
- [MarkTechPost: Jev release](https://www.marktechpost.com/2026/09/19/typesafe-ai-releases-jev/) and [coding guide](https://www.marktechpost.com/2026/09/23/a-coding-guide-to-typesafe-ai-jev/)
- [LangChain: building a harness with Jev](https://www.langchain.com/blog/building-a-harness-with-jev)
- [Archer Hume: Jev's architecture unmasked](https://archerhume.com/posts/jevs-architecture-unmasked) (speculative reverse-engineering)
- Local models: [Kev](https://github.com/jaredpalmer/kev), [Laya](https://github.com/NandhaKishorM/laya), [SemIf-OpenJev](https://github.com/TheoLeeCJ/SemIf-OpenJev), [openjev](https://github.com/razorback16/openjev), [localjev](https://github.com/githubnext/localjev), [jeff](https://github.com/logan-markewich/jeff), [ScriptByAI roundup](https://www.scriptbyai.com/jev-open-source-alternatives/)
- Jev in harnesses: [pi-jev](https://github.com/y0usaf/pi-jev), [pi-jev-permit](https://github.com/kurihada/pi-jev-permit), [jev-sentinel](https://github.com/harshwasan/jev-sentinel), [OpenRouter auto-approve cookbook](https://openrouter.ai/docs/cookbook/coding-agents/auto-approve-permission-prompts-with-jev)
- Harnesses: [Codex](https://github.com/openai/codex), [OpenCode](https://github.com/anomalyco/opencode), [Crush](https://github.com/charmbracelet/crush), [Pi](https://github.com/earendil-works/pi), [Grok Build](https://github.com/xai-org/grok-build) ([Simon Willison on its release](https://simonwillison.net/2026/Jul/15/grok-build/)), [mini-swe-agent](https://github.com/SWE-agent/mini-swe-agent)
- [Mario Zechner: what I learned building pi](https://mariozechner.at/posts/2025-11-30-pi-coding-agent/), [LangChain: anatomy of an agent harness](https://www.langchain.com/blog/the-anatomy-of-an-agent-harness), [Martin Fowler: harness engineering](https://martinfowler.com/articles/harness-engineering.html), [arXiv 2603.05344: building coding agents for the terminal](https://arxiv.org/html/2603.05344v1)

---

## 11. Decisions (25 Sep 2026) and follow-ups

Made in the proposal artifact (https://claude.ai/artifact/RiFnQgX1UYHARpjPgqG7qw):

| Decision | Choice |
|---|---|
| Audience | Your daily driver first |
| Prove it in Pi first | **No**, start the kernel now |
| Language | **Go** |
| First interface | **Terminal UI first** |
| Decision backend | Hosted Jev for safety gates, local Kev for the rest |
| Background agents | Not yet |
| Default before risky actions | Ask only when risky |
| Extensions first | Skills and hooks |
| Licence | Apache-2.0 |

**What Go means.** Pi's TypeScript libraries are out. The Go replacements are Charm's [`fantasy`](https://github.com/charmbracelet/fantasy) (Apache-2.0, ~19k lines, the LLM layer under Crush, *not* under Crush's FSL). It supports Anthropic, OpenAI, Google, Bedrock, Azure, OpenRouter, Vercel and OpenAI-compatible providers, and has the hooks `PrepareStep`, `StopWhen`, `RepairToolCall`, `OnStepStart/Finish`, `OnToolCall` and `OnToolResult`. Also [`catwalk`](https://github.com/charmbracelet/catwalk) (MIT, model catalogue) and Bubble Tea / Lip Gloss / Bubbles (MIT, TUI). TypeSafe ships only Python and JS SDKs, and the API is one endpoint, so we write a small Go client. Kev is Python, so it runs as a managed local sidecar over HTTP.

### Cheaper alternatives to Jev?
Jev's $0.042/Mtok input (output free) is effectively the floor for hosted System One models:
- **OpenRouter** `typesafe/jev-1.13`: same price.
- **Together AI Tev1-4B-experimental** (23 Sep): same price. A Qwen3.5-4B fine-tune that returns a letter. Not API-compatible, and there's no accuracy comparison with Jev.
- **Cloudflare Workers AI** `typesafe/jev`: 10,000 free Neurons per day, then $0.011 per 1k Neurons. Jev's Neuron rate isn't published. If it's at parity with TypeSafe's price, that's about 2.6M free tokens a day (unverified). It's reported to work without a TypeSafe invite.
- **Local Kev or Laya**: $0 on your own hardware.
- Cheap generative LLMs list input from about $0.02/Mtok, but they bill output tokens, are slower and aren't calibrated.

Estimated Girdle spend: 400 steps/day × 3 checks × 1.5k tokens ≈ 1.8M tokens ≈ $0.08/day all-hosted, and under $1/month with hosted gates only. **Price isn't the constraint. Access (waitlist, rate limits) and privacy are.**

Open follow-ups, added to the proposal as new decisions: which route to hosted Jev (OpenRouter recommended), and whether to run the gates in shadow mode first (recommended, since the Pi proof step was skipped).

### Update: Jev only (25 Sep 2026, later)
From chat: "just use jev at $TYPESAFE_API_KEY". The route is TypeSafe direct, and the backend changed from "mixed" to **Jev for everything**, so Kev is no longer the default. The key is defined in `~/.zshrc` and verified (HTTP 200). It is *not* in the `launchctl` environment, so non-interactive processes need it passed in.

**Hosted Jev (`jev-1.13`) vs Kev-0.8B**, on the same test sets (`experiments/jev_*.txt`):

| Test | Kev-0.8B | Jev |
|---|---|---|
| 43 harness decisions | 79% | **95%** (misses: `git push --force` → destructive rather than external; "upgrade all deps and open a PR" background p=0.29) |
| High-confidence (≥0.6) accuracy | 9/9, covering 21% of cases | 38/39, covering 91% of cases |
| Shell risk: 4-way Choice, top answer | 6/12 | **12/12** |
| Shell risk: 3 Nouls combined | 9/12 | 11/12 (`npm install` read as a network action, which is defensible) |
| Shell risk: damage Score | AUC 0.97 | **AUC 1.00**, rank correlation 0.97 |
| Complexity Score | rank correlation 0.84 | **0.96**, 9/9 tiers |
| Turn-end status Choice | 6/8 | **8/8** |
| Urgency Score | 0.48 | **0.97** |
| Tool routing (12 tools) | top-1 6/8, top-3 8/8 | top-1 6/8, top-3 8/8 (literal first steps: read_file before edit, grep before a 40-service audit) |
| Latency from this Mac | ~47 ms | **~500 ms per request; a 6-question batch took 504 ms** |

Design consequences:
- One request per checkpoint (batching is free).
- Run checks in parallel with LLM streaming.
- Code rules decide the obvious cases, so they never wait on Jev.
- Pin `jev-1.13` for thresholds.

**Shadow mode: no** (25 Sep, from chat). The gates enforce from day one with conservative thresholds, which are tuned from the decision log. All 11 proposal decisions are now made.
