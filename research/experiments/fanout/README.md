# Jev fan-out experiments

These September 2026 scripts replay logged benchmark runs through hosted Jev (`jev-1.13.0`) with more questions per checkpoint. [Decision 0013](../../../docs/decisions/0013-jev-fan-out.md) records the original findings. The scripts preserve the method; the original run corpus and generated datasets are not bundled.

## Inputs and execution

Run from this directory, with benchmark runs under `bench/results/<run>/<task.agent.rep>/` containing `result.json` and `events.jsonl`. File-selection experiments also need the warm scale repositories under `bench/.warm/girdle-fast/<task>/repo` and matching `snap-<task>.txt` dumps.

Use the [benchmark guide](../../../docs/benchmarks.md) to generate new input runs. Results from new inputs are a new experiment, not a reconstruction of the original measurements. Replay scripts call hosted Jev and can incur costs. Their `jevlib.py` loads `TYPESAFE_API_KEY` through interactive zsh without printing it. Analysis scripts primarily use the standard library; the commands below use uv for NumPy where needed.

## Step end and failed checks

Build the replay datasets first, then obtain answers, then analyse them:

```bash
python3 build.py                      # step_rows.json and apply_rows.json
python3 replay_step.py                # step_answers.jsonl
python3 analyse_step.py               # question AUCs
uv run --with numpy python loto.py    # leave-one-task-out thresholds
uv run --with numpy python loto3.py   # status and check-coverage policies
uv run --with numpy python wrongstops.py
uv run --with numpy python specfan.py # questions for rules in a bulleted spec

python3 replay_apply.py
python3 analyse_apply.py
```

The original corpus produced 1,410 step decisions and 583 failed applies. Those are recorded dataset sizes, not expected counts for new runs. `bank.py` holds the question bank; `policies.py` sweeps thresholds.

## File selection

`filefan.py` compares Jev's picks with the files passing agents looked up, excluding files already in the snapshot. Before running it, provide all six scale-task checkouts and the corresponding snapshot dumps. The original session generated the dumps through a temporary kernel helper; that helper and its outputs are not bundled. A new experiment needs matching dumps from the checkout's `TakeSnapshot` API and each task's request.

```bash
python3 filefan.py                    # filefan.json, one request per file
python3 filefan_eval.py
python3 filefan_batch.py              # batched comparison
python3 lookups.py
python3 lookfan.py 16                 # file picks after each lookup
```

`filefan_batch.py` uses the filefan inputs. `lookups.py` creates the lookup dataset consumed by `lookfan.py`. Ensure input data exists before invoking a downstream script; an empty log directory is not the original corpus.

## Other probes

- `probe.py` measures latency and tokens as question counts change.
- `live_eval.py <results-dir>` compares lookups, LLM steps and early stops across benchmark variants.

Generated JSON/JSONL and snapshot files are local artifacts ignored by Git. Keep sensitive provider context out of shared outputs.
