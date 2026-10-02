# 0024 How the TUI presents a session

**Date:** 2026-10-01 · **Branch:** `feat/tui-polish`

- **Decision.** The TUI shows each part of a session as its own kind of entry, with an input box always visible at the bottom, as Claude Code, Codex, OpenCode and Pi do. Before this, the transcript was a list of coloured strings and the prompt had no frame, so it was hard to see where to type, and replies, tool output and Jev's decisions ran together.

  | Entry | How it shows |
  |---|---|
  | Your request | A teal bar and a tinted row |
  | Reply | `◆ girdle`, then the reply rendered as Markdown with Glamour |
  | Thinking | `✻ thinking`, the step's reasoning summary in italics, four lines until `ctrl+o` |
  | Tool call | A spinner while it runs, then `●`; its result underneath, attached to its own call |
  | Jev decision | `◇ jev`, the decision in a colour for what it means, then its scores |
  | Note from Girdle | A dim line, gold to notice or red for a failure |
  | End of a run | A rule: done, over to you, stopped or failed, and how long it took |

- **Glamour renders replies.** It is Charm's Markdown renderer, built on the same Lip Gloss as the rest of the TUI, and highlights code with Chroma. Its standard dark and light styles are used with the margins removed and the accents taken from the TUI's palette. A reply still streaming is drawn as Markdown up to its last finished paragraph, cached until another finishes, and as plain text after that. Rendering the whole reply on each delta took 10.5 s of rendering for a 7 KB reply in 20-byte deltas; the incremental version took 65 ms for a 3 KB reply in 159 deltas.
- **Entries follow fantasy's order within a step.** A step's text streams, its stream ends, its tool calls run, then its reasoning and whole text arrive. The reasoning goes before the step's reply or tool calls, and the whole text replaces what streamed rather than adding a second copy.
- **The status says what the LLM is writing.** The kernel emits `tool_start` when the LLM begins writing a tool call. Like `text_delta`, it reaches the UI only and stays out of the log and JSON output. With `-race`, calls are buffered until one copy completes, so the phase shows only briefly.
- **A run ends on its `run_end` event.** That event arrives in order after everything the run emitted, so the status can't say "done" while the run's last entries are still queued.
- **Entries are drawn for the current width.** Each finished entry is drawn once per width and theme and cached, so resizing the terminal re-wraps everything. Before, text was styled once and never re-wrapped.
- **Paths are shown relative to the project**, so tool calls fit on a line.
- **Animation runs only while something moves.** A tick drives the spinner, a highlight moving across the phase (`Thinking`, `Running bash`, `Writing`), the input border breathing, and the belt that buckles under the wordmark at start-up. Idle, nothing ticks.
- **Light and dark terminals.** The TUI asks the terminal for its background and switches between two palettes taken from the Girdle mark.
- **Scrolling stays where you put it.** New output follows the end of the transcript only if you are already there; otherwise the status line says there is more below.
- **The welcome screen states facts.** It says whether Jev checkpoints are on, rather than always claiming so, and shows the model, effort, project and log.
