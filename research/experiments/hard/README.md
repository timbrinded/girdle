# Building the hard suite

Decision 0015 describes the method. Run these from this directory, with each upstream repository cloned with full history under `repos/`, for example `git clone https://github.com/expr-lang/expr.git repos/expr`.

```bash
python3 mine.py expr goldmark toml pflag go-cmp mux more-itertools   # candidates.json
python3 build_task.py expr:8b8934fc71:ex-find                          # bench/hard/ex-find, from that fix commit
../../../bench/validate.sh ../../../bench/hard/ex-find/
```

`build_task.py` writes `source`, `solution.patch`, `hidden/tests.patch` and `check.sh`. It writes a placeholder `task.md` holding the commit message, to be replaced with a real statement. To list the API a statement must name, compile the hidden tests against the starting code: every `undefined` in the errors is one.
