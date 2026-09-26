# Girdle

## North Star

**An agent you can leave alone.** Girdle is a terminal coding agent. It keeps working when it should, stops exactly when it needs you, never claims to be done when it isn't, and uses the cheapest model that can do each step. It is built first as its author's daily driver.

The work is split three ways:
- **The LLM decides and does the work.** It chooses the next tool call and its arguments, and writes plans, edits and replies.
- **Jev judges meaning at checkpoints.** Jev is TypeSafe's calibrated decision model. It makes fast, closed-set decisions around every LLM step: keep going or stop, stuck or progressing, which model to use, whether output is relevant, whether an action is catastrophic.
- **Code owns the loop and reads facts.**

### What "working" means
Girdle is succeeding when, in an unattended session on real work, it:
1. Doesn't stop to announce an action it then doesn't take, or to ask a question with an obvious answer.
2. Notices within a few steps that it's stuck or drifting, then changes course or asks.
3. Never reports "done" without evidence from its own tool results.
4. Runs trivial steps on a cheap model and hard steps on a strong one.
5. Keeps context relevant, pruning noisy tool output before it crowds out the task.
6. Stops catastrophic actions and nothing else: no permission prompts in normal use. Catastrophic means deleting outside the project, force-pushing a shared branch, or sending secrets off the machine.

### Principles
- **Code reads facts; Jev judges meaning.** Code reads typed fields, exit codes, token counts and paths. Anything code would otherwise guess with a regex, allowlist, hash or heuristic becomes a Jev question.
- **A hard floor under the tripwire.** A short never-allow list in code sits beneath Jev. Injected text can steer Jev, so Jev is never the only barrier to an irreversible action.
- **Checkpoints are data.** Each checkpoint is a question set plus a policy table mapping probabilities to actions. A new behaviour is a new question and a threshold, not a new heuristic module.
- **One Jev request per checkpoint.** Batching is free: about 0.5 s for one question or six. Run checks while the LLM streams where possible.
- **Read the distribution.** Use Choice for *which*, Score for *how much* and Noul for *whether*. Act on the probabilities, not just the top answer.
- **Log every decision**: state, questions, answers, model version and outcome. The log is how thresholds get tuned and regressions get caught.
- **Degrade deliberately.** If Jev is unreachable, the tripwire fails closed and other checkpoints fall back to simple defaults.

### Non-goals (for now)
- Permission prompts as a feature
- Background agents or daemons
- MCP
- Web, desktop or IDE clients
- Local decision models by default
- Parity with Claude Code

## Constraints
- **Go, written as modern Go.** Use the `use-modern-go` skill before writing or changing Go code.
- **Libraries:** `charmbracelet/fantasy` for LLM providers and the inner step loop. Bubble Tea, Lip Gloss and Bubbles for the TUI.
- **Never copy code from Crush.** Its FSL-1.1 licence forbids it.
- **Licence:** Apache-2.0.
- **Jev:** `POST https://api.typesafe.ai/v1/systemone` with `Authorization: Bearer $TYPESAFE_API_KEY`.
  - The key is set in `~/.zshrc`, not in non-interactive shells.
  - Never print or log the key.
  - Pin `jev-1.13.0` while thresholds are tuned against it.

## How we work
There is no up-front implementation plan. We deliver in **segments**:
- A segment is an outcome plus a gate, tracked as one GitHub issue. Only the current segment is written in detail. The next few are one-liners.
- The first segment is a thin end-to-end loop: TUI → kernel → LLM → tool → Jev → log → TUI. Every later segment must leave the whole thing working.
- A gate is a runnable scenario against a fixture repo, using real Jev and a real LLM, plus a short demo by the user. Mocks don't count as a gate.
- Issues say *what* must be true and *how we'll know*, never *how*. Decide the how from the code as it is. Keep to the segment's scope, and cut scope rather than overrun.
- After making a non-obvious decision, record it in `docs/decisions/`: a few lines covering the decision, why, and the date.
- Test a new idea against the current fast flow only, on the tasks it targets (`girdle-fast` against `girdle-fast+<flag>`). Run the default flow and full suites only at milestones. An idea that doesn't pay is removed or shelved, with its evidence in `docs/decisions/`, and isn't re-run.

## Reference
- Research, measurements and decision history: `research/01-harness-and-jev.md`
- Jev and Kev experiment scripts and results: `research/experiments/`
