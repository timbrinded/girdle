I'd like to compare two parsed constraint sets for equivalence, and to put a set into a canonical order. Right now I have to compare `String()` output, which breaks on whitespace and ordering.

1. **`Constraints.Equals(other Constraints) bool`.** It should return true when both sides hold the same constraints, meaning the same operator and the same version, in any order and whatever the whitespace. A bare version means `=`.
   - `"0.0.1"` equals `"0.0.1"`, `" 0.0.1 "` and `"=0.0.1 "`.
   - `">0.1.0, <=1.0.0"` equals `"<=1.0.0, >0.1.0"`.
   - `"=0.0.1"` does not equal `"=0.0.2"`, and `">0.0.1"` does not equal `"=0.0.1"`.

2. **`Constraints` should implement `sort.Interface`**, so that `sort.Sort(cs)` gives a deterministic order. Sort first by operator in this order: `<`, `<=`, `=` (a bare version counts as `=`), `!=`, `>`, `>=`, `~>`. Within one operator, sort by version, lowest first. `String()` should still print each constraint as it was written, joined with `,`. After sorting:
   - `">= 0.1.0,< 1.12"` prints `"< 1.12,>= 0.1.0"`, and `"< 1.12,>= 0.1.0"` stays as it is.
   - `"< 1.12,>= 0.1.0,0.2.0"` prints `"< 1.12,0.2.0,>= 0.1.0"`.
   - `">1.0,>0.1.0,>0.3.0,>0.2.0"` prints `">0.1.0,>0.2.0,>0.3.0,>1.0"`.

`go test ./...` should pass.
