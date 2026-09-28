Commaf mangles infinities. `Commaf(math.Inf(1))` returns `+,Inf` and `Commaf(math.Inf(-1))` returns `-+,Inf`. BigCommaf does the same with `big.NewFloat(math.Inf(1))` and `big.NewFloat(math.Inf(-1))`.

Expected:

- `Commaf(math.Inf(1))` gives `+Inf`, and `Commaf(math.Inf(-1))` gives `-Inf`.
- `Commaf(math.NaN())` gives `NaN`.
- `BigCommaf` of +Inf and -Inf gives `+Inf` and `-Inf`.

Output for finite numbers should stay the same. `go test ./...` should pass.
