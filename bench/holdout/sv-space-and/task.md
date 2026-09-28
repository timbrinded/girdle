Constraints currently need a comma between AND conditions. `NewConstraint(">= 1.2.3, < 2.0")` works, but `NewConstraint(">= 1.2.3 < 2.0")` fails with "improper constraint". Most other semver tools accept whitespace as an AND separator, so ranges copied from them don't parse here.

Please let whitespace separate AND conditions, and keep commas working as they do now:

- Any amount of whitespace can separate conditions. `">1.1 <2"` accepts 1.2.1 and rejects 1.1.1. `">=1.1    <2    !=1.2.3"` rejects 1.2.3.
- Whitespace between an operator and its version still belongs to that one condition. `"> 1.1 < 2"` is the two conditions `>1.1` and `<2`, and `">= 1.1 <2 != 1.2.3"` is three conditions.
- Whitespace-separated ANDs combine with `||`. `">= 1.2.3 < 2.0"` parses as one OR group of two conditions. `">= 1.2.3 < 2.0 || => 3.0 < 4"` parses as two OR groups, and the first group has two conditions. `">=1.1 <2 !=1.2.3 || > 3"` accepts 4.1.2 but rejects 3.1.2, 3.0.0 and 1.2.3. With `>= 3` in place of `> 3`, it accepts 3.0.0.
- Commas with extra spaces still work: `"> 1.1, <     2"`, `">= 1.1, <2, != 1.2.3 || > 3"`.
- Hyphen ranges and the existing forms keep their current meaning: `"1.1 - 2"`, `"3 - 4 || => 3.0, < 4"`, `"1.1-3"`.
- Invalid input is still rejected. `NewConstraint(">= bar")` and `NewConstraint("BAR >= 1.2.3")` must both return an error.

Check results for comma-separated constraints must not change. `go test ./...` should pass.
