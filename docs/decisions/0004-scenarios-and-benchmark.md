# 0004 Scenarios and benchmark

**Date:** 2026-09-26 · **Segment:** 1

**Decision:**

- Scenarios and the benchmark are shell scripts plus small Python scorers under `bench/`.
- Each task has:
  - a fixture `repo/`
  - a prompt (`task.md`)
  - a `check.sh`, which often uses `hidden/` tests the agent never sees
  - a reference `solution/`
- `bench/validate.sh` proves every check fails on the untouched repo and passes on the reference solution.
- `bench/gate.sh` runs Segment 1's two gate scenarios.
- `announce-then-stop` starts from a seeded conversation (`-seed`), because a real LLM can't be made to stop early on demand. Jev and the LLM are still real.
- The baseline is **vanilla Pi**, with the same model and reasoning effort:
  - It runs in tmux with `PI_CODING_AGENT_DIR=bench/pi-agent`, so the user's Pi packages and settings aren't used or changed.
  - Extensions, skills, prompt templates and context files are all off.
  - Retries are on.

**Why:** Hidden tests stop an agent from passing by weakening checks. Validating against reference solutions stops a broken task from counting as an agent failure. Keeping Pi vanilla and isolated makes the comparison fair.
