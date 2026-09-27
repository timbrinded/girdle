# ast-grep with Jev: experiments

Decision 0014 has the findings. These scripts need `ast-grep` on the PATH (`brew install ast-grep`), and read `bench/results`, `bench/.warm` and `bench/scale`. Scripts that ask Jev load `TYPESAFE_API_KEY` from `~/.zshrc` through an interactive zsh, and never print it.

## Structural context: the units agents look up

```bash
python3 unitlabels.py    # unitlabels.json: top-level units (ast-grep) that each scale task's lookups covered
python3 s1.py            # units that use the named code, or that it calls, as pure facts
python3 cands.py         # candidates, including test helpers, each scored by Jev
python3 evalcands.py     # how much of the lookup weight each strategy covers within 16 KB
```

`units.py` extracts top-level units with one `ast-grep scan --inline-rules` call per language.

## Prefetch outlines

`filefan_sg.py` compares Jev's file picks from ast-grep outlines with the regex outlines from `../fanout`. It needs `filefan.json` from `../fanout/filefan.py`.

## Tripwire

- `shellfacts.py` is the Python prototype of `internal/tools/shellfacts.go`. It parses a command line with ast-grep's bash grammar, and returns what it deletes, pushes and sends, and the secrets it touches, plus the floor.
- `crafted.py` holds the test sets: 39 catastrophic commands, including evasions such as `bash -c`, `eval`, `sudo`, `xargs` and inline Python, and 31 safe commands that look dangerous.
- `tripjev.py` asks Jev about the commands that the floor doesn't decide.

The false-positive corpus is every command the benchmark agents ran, 3,727 distinct command lines. Collect them from the `tool_call` events in `bench/results` for the bash tool, and from apply's `check` and `reproduce` inputs.
