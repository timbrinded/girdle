# 0005 Fast flow: fewer, larger LLM steps

**Date:** 2026-09-26 · **Branch:** `perf/fast-flow`

**Decision:** `-fast` changes the high-level flow so that most tasks finish in one LLM step:
- **Snapshot.** Each request carries the repository's files: every path, plus the full text of as many as fit in 64 KB. The LLM no longer spends steps listing and reading files.
- **Apply.** The fast toolset is `read`, `apply` and `bash`. One `apply` call takes every change (exact-text edits, `replace_all` for renames, or whole-file writes) plus a required `check` command. The changes are applied in order, then the check runs, so one LLM step both changes and verifies the code. The prompt asks for a check that proves the whole task, such as tests plus a grep that an old name is gone, and for small edits rather than whole-file rewrites.
- **Early stop.** After any step whose latest check exited 0, in a request that changed files, a `step_end` checkpoint asks Jev whether the tool results show the task done. It asks one `complete` Noul plus the per-requirement coverage Nouls. It stops only at `complete` ≥ 0.8 with every instruction covered. The run then ends without the LLM step that would only write a summary, and code writes the reply from facts: the files changed and the check that passed. If Jev is unreachable, the LLM carries on and the turn-end checkpoint decides as usual.

**Why:** In the segment 1 benchmark, model time was 97% of a run: 8.6 LLM steps of about 5.6 s each. About 3 steps per run were exploration, about 3 were single edits, one ran the tests and one wrote the reply. Each step pays 1 to 3 s before its first token, and output streams at about 150 to 400 tokens a second. So the flow was changed to cut steps, and prompted to cut output.

**What was measured on the way (13 tasks, 3 reps each):**

| Change | Pass | Mean time |
|---|---|---|
| Segment 1 flow (v4) | 65/65 | 49 s |
| Snapshot, batch prompt with edit + bash, early stop | 3/3 smoke | 14 s |
| `apply` tool with a check | 37/39 | 20 s |
| Checks that prove the whole task, `replace_all` | 38/39 | 22 s |
| Small edits instead of whole-file writes (with racing, below) | 39/39 | 15 s |

- The two misses at the `apply` stage were go-rename. Working from the snapshot, the model renamed the code but left the old name in a comment. Its tests passed, so the evidence looked complete. Asking for a check that proves the whole task fixed it.
- The later miss was a real js-csv edge case (CRLF inside quoted fields) that the model's own tests didn't cover.

**Tried and dropped:**
- **Reasoning `none`**: this model rejects it ("Reasoning is mandatory").
- **Reasoning `minimal`**: 10.6 s mean but 34/39, failing exactly the spec-heavy tasks.
- **Split step**: one LLM call wrote the implementation while another wrote the tests, then both were applied together. It passed 4/4 but was no faster (35 s against 31 s). The implementation half does nearly all the reasoning, so the run waits on it as long as on one combined call. The code was removed.

## Update 2026-09-26: fewer wasted steps

In the three-way benchmark, 24 of 65 fast runs needed more than one LLM step. The most expensive extra steps followed a rejected apply call. The model had sent its changes as a string holding the JSON array, or two edits touched the same lines, so it had to write every file again. That cost up to 60 s on go-ttl-cache.
- `apply` now accepts the string form.
- `check` is required, so every apply can end the run.
- The prompt says changes apply in order and must not overlap, and that a check must never hide its exit code with `; echo` or `|| true`.

Side by side, LLM steps per run fell from 1.85 to 1.54, and the fast flow passed 65/65 at a 15.5 s mean against 35.9 s for the default flow. Results are in `research/03-fast-flow-results.md`.
