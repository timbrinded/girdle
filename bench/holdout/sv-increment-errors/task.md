## Add increment methods that return an error instead of panicking

`IncPatch`, `IncMinor` and `IncMajor` panic when the segment they bump is already `math.MaxUint64`. That value is easy to reach from untrusted input, because `NewVersion("1.2.18446744073709551615")` parses fine. Right now the only way to bump such a version safely is to wrap the call in `recover`.

Please add error-returning versions of the three methods on `Version`:

```go
func (v Version) IncPatchE() (Version, error)
func (v Version) IncMinorE() (Version, error)
func (v Version) IncMajorE() (Version, error)
```

- When nothing overflows, they return exactly what the existing method returns, including `String()` and `Original()`, and a nil error. For example, `v1.2.4-beta+meta` with IncMinorE gives `1.3.0` with original `v1.3.0`.
- On a prerelease, IncPatchE drops the prerelease and keeps the patch number, just as IncPatch does, so it can't overflow. For example, `1.2.18446744073709551615-beta` becomes `1.2.18446744073709551615` with no error.
- On overflow, they return the receiver unchanged (an identical `Version` value) and an error. The error must satisfy `errors.Is(err, ErrIncrementOverflow)`, where `ErrIncrementOverflow` is a new exported error variable. Its `Error()` text must be exactly one of:
  - `patch version increment would overflow uint64`
  - `minor version increment would overflow uint64`
  - `major version increment would overflow uint64`
- Only the segment being bumped matters. For the parsed version above, IncPatchE errors, but IncMinorE and IncMajorE succeed.

The existing `IncPatch`, `IncMinor` and `IncMajor` should still panic on overflow. The panic value should now be that same error value rather than a string, so that code that recovers can check it with `errors.Is(r.(error), ErrIncrementOverflow)`.

`go test ./...` should pass.
