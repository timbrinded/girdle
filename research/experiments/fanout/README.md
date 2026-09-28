# Jev fan-out experiments

These scripts replay logged benchmark runs through hosted Jev (`jev-1.13.0`) with far more questions per checkpoint than Girdle asked. Decision 0013 has the findings.

They read `bench/results/*/*` and run from this directory. Each script loads `TYPESAFE_API_KEY` from `~/.zshrc` through an interactive zsh, and never prints it. Replaying everything costs well under a dollar in Jev tokens.

## Step end: is the work done?

```bash
python3 build.py          # step_rows.json (1,410 decisions), apply_rows.json (583 failed applies)
python3 replay_step.py    # step_answers.jsonl: today's questions on the logged state, the broad bank on a richer state
python3 analyse_step.py   # AUC of every question: all decisions, within stops, within continues
uv run --with numpy python loto.py    # stop policies, thresholds chosen leave-one-task-out
uv run --with numpy python loto3.py   # status plus "does the check exercise the task?"
uv run --with numpy python wrongstops.py   # each false "done" against the requirement Jev flagged
uv run --with numpy python specfan.py      # one question per rule of a bulleted spec comment
```

`bank.py` holds the question bank, and `policies.py` sweeps single thresholds.

## After a failed check: tests or code?

```bash
python3 replay_apply.py && python3 analyse_apply.py
```

## Which files will the agent need?

`filefan.py` compares Jev's picks with the files that agents looked up, minus what the snapshot already showed. It needs `snap-<task>.txt` for each scale task, holding the snapshot Girdle takes for that task. The session produced them with a temporary test in `internal/kernel`, run against each warm repository in `bench/.warm/girdle-fast/<task>/repo`:

```go
func TestZZSnapDump(t *testing.T) {
	dir, req, out := os.Getenv("SNAPDUMP_DIR"), os.Getenv("SNAPDUMP_REQ"), os.Getenv("SNAPDUMP_OUT")
	if dir == "" {
		t.Skip()
	}
	s := TakeSnapshot(t.Context(), dir, DefaultSnapshotBudget, req)
	if err := os.WriteFile(out, []byte(s.Text), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

```bash
python3 filefan.py && python3 filefan_eval.py   # one request per file
python3 filefan_batch.py                        # 15 files per request, for comparison
python3 lookups.py && python3 lookfan.py 16     # after each lookup: which files next?
```

## Other scripts

- `probe.py` measures latency and tokens against the number of questions in one request.
- `live_eval.py <results dir>` compares benchmark variants on lookups, LLM steps and early stops.
