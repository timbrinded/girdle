## ComputeSI silently loses magnitude outside 1e-30..1e30

`SI(9.1093837015e-31, "kg")`, the electron mass, prints `910.93837 kg`. `ComputeSI` returns `(910.938…, "")`: the value is scaled by 10^-33, but there is no prefix for that exponent, so it comes back empty. Nothing fails, and the number is off by 33 orders of magnitude. Large values have the same problem, for example `ComputeSI(1e34)` returns about `(10, "")`.

Expected: for every finite, non-zero input, including subnormals and `math.MaxFloat64`, `ComputeSI` returns a prefix from the SI prefix table (`""` for none). Multiplying the returned value by that prefix's power of ten must give back the input, within floating-point rounding. Past either end of the table, use the extreme prefix (`q` for tiny values, `Q` for huge ones) and let the value fall outside the usual 1–1000 range:

| input | value | prefix |
|---|---|---|
| 9.1093837015e-31 | 0.91093837015 | q |
| -9.1093837015e-31 | -0.91093837015 | q |
| 6.62607015e-34 | 0.000662607015 | q |
| 1e-33 | 0.001 | q |
| 1e-45 | 1e-15 | q |
| 1e-30 | 1 | q |
| 1e33 | 1000 | Q |
| 1e34 | 10000 | Q |
| 1e60 | 1e30 | Q |
| math.MaxFloat64 | math.MaxFloat64 / 1e30 | Q |

Results for inputs inside the table's range don't change. With this fix, SI and ParseSI round-trip across the whole float64 range.

`go test ./...` should pass.
