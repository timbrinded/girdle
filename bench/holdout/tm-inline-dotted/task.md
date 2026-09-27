An inline table with dotted keys inside an array doesn't decode correctly. For example

```toml
arr = [
	{a.b.c = 1},
]
```

should decode to an array holding one table `{a = {b = {c = 1}}}`, and the same inside arrays that mix inline tables with other values or nest them. Fix it so the toml-test suite passes, including new valid cases like these. `go test ./...` must pass.
