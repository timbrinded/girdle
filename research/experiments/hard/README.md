# Building upstream-fix tasks

This September 2026 machinery mines fix commits and constructs benchmark tasks. [Decision 0015](../../../docs/decisions/0015-hard-suite.md) describes the method; [decision 0016](../../../docs/decisions/0016-no-benchmark-fitting.md) records the later development/holdout split. Existing held-out task statements remain fixed inputs.

## Inputs and generation

Run from this directory with Git, Python 3 and full-history upstream clones under `repos/`. The mining command below expects directories named `expr`, `goldmark`, `toml`, `pflag`, `go-cmp`, `mux` and `more-itertools`. Their URLs are available in the existing tasks' `source` files. For a single example, clone expr:

```bash
git clone https://github.com/expr-lang/expr.git repos/expr
python3 mine.py expr                                           # candidates.json
python3 build_task.py 'expr:<fix-commit>:<new-task>'             # bench/hard/<new-task>
```

Replace `<fix-commit>` with a candidate fix commit and `<new-task>` with a new development-task name. Select a fix outside the held-out corpus; renaming a held-out task does not make it a development task. The clone cache and generated data are local artifacts, not bundled source.

`build_task.py` writes `source`, `solution.patch`, `hidden/tests.patch` and `check.sh`. For a new task it also writes a placeholder `task.md` containing the commit message. Replace that placeholder with a statement describing the required behavior and API. Compile the hidden tests against the starting code to identify required API names, then check that the statement and tests agree.

Validate the new task:

```bash
../../../bench/validate.sh ../../../bench/hard/<new-task>/
```

Validation must fail on the starting repository and pass with the reference fix. Read the [benchmark guide](../../../docs/benchmarks.md) before running agents. Preserve the pinned upstream commit and its license notice when sharing derived patches or tests; see [third-party notices](../../../THIRD_PARTY_NOTICES.md).
