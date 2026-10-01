# ast-grep with Jev: experiments

These September 2026 prototypes explore structural code context and shell-command facts. [Decision 0014](../../../docs/decisions/0014-ast-grep-with-jev.md) records the findings; [decision 0019](../../../docs/decisions/0019-harness-time.md) records removal of the structural-context feature. The tripwire evolved into the current implementation, while structural-context scripts remain historical research.

## Inputs

Run from this directory with [ast-grep](https://ast-grep.github.io/guide/quick-start) on PATH. The scripts read benchmark logs under `bench/results`, warm scale checkouts under `bench/.warm` and task statements under `bench/scale`. The original corpus is not bundled; generate new inputs using the [benchmark guide](../../../docs/benchmarks.md).

Scripts that ask Jev load `TYPESAFE_API_KEY` through interactive zsh without printing it. Those calls can incur costs. Generated JSON files are ignored by Git.

## Structural context

With warm scale repositories and logged lookups available, build labels and candidate facts before scoring candidates:

```bash
python3 unitlabels.py    # unitlabels.json: units covered by logged lookups
python3 s1.py            # facts about units using or called by named code
python3 cands.py         # candidates and Jev scores
python3 evalcands.py     # lookup coverage within a 16 KB budget
```

`units.py` extracts top-level units with one `ast-grep scan --inline-rules` call per language. These probes describe the archived idea, rather than an available `-structure` flag in current Girdle.

## Prefetch outlines

`filefan_sg.py` compares ast-grep outlines with the regex-outline method. First create `filefan.json` and its required checkouts/snapshot inputs as described in the [fanout guide](../fanout/README.md), then run:

```bash
python3 filefan_sg.py
```

## Tripwire

- `shellfacts.py` is the Python prototype of [internal/tools/shellfacts.go](../../../internal/tools/shellfacts.go): command facts about deletes, pushes, sends and secrets, followed by the code floor.
- `crafted.py` contains the original 39 catastrophic and 31 safe-looking-dangerous command cases, including nested shells, wrappers and inline code.
- `tripjev.py` asks Jev about cases the floor leaves for judgement.

The original false-positive corpus contained 3,727 distinct agent command lines. A new corpus can be collected from benchmark `tool_call` events: the `bash` tool's command and the `apply` tool's `check` and `reproduce` fields. The original extraction file is not bundled, so retain the commands and collection method with a new report.
