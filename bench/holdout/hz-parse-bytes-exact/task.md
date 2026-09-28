ParseBytes is inexact for large byte counts

ParseBytes gets large whole numbers wrong:

- `ParseBytes("9007199254740993")` (2^53 + 1) returns 9007199254740992, which is off by one.
- `ParseBytes("18446744073709551615")` (math.MaxUint64) fails with a "too large" error, even though the value fits in a uint64.

A whole-number byte count should parse exactly across the full uint64 range, with or without a `B` suffix and with or without a space before it:

| input | expected |
|---|---|
| `"9007199254740993"` | 9007199254740993 |
| `"9007199254740993B"` | 9007199254740993 |
| `"18446744073709551615"` | 18446744073709551615 |
| `"18446744073709551615 B"` | 18446744073709551615 |

All current behaviour must stay the same: fractional and unit inputs such as `"42.5 MiB"` and `"1,005.03 MB"`, the "unhandled size name" error for unknown units, and the error for values that overflow a uint64, such as `"16 EiB"`.

Run `go test ./...` to confirm.
