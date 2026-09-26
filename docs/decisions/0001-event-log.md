# 0001 Event log format

**Date:** 2026-09-26 · **Segment:** 1

**Decision:** Each session writes one JSON object per line (JSONL) to `~/.local/state/girdle/sessions/<timestamp>-<pid>.jsonl`, or to the path given with `-log`. Event types: `session_start`, `user_message`, `assistant_text`, `tool_call`, `tool_result`, `turn_end`, `decision`, `nudge`, `run_end`, `error`. Streaming `text_delta` events go to the UI only and are never logged. A `decision` event carries the full Jev state, answers, model version, latency and the policy rule that fired.

**Why:** Append-only JSONL is trivial to write, tail and replay, and it's the dataset for tuning thresholds. Logs stay outside the working repo so Girdle never dirties it.
