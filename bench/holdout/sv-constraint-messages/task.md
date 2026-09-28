## Validate gives the wrong reason for failed caret/tilde constraints

With the constraint `^1.12.7` and the version `1.6.6`, `Validate` says `1.6.6 does not have same major version as 1.12.7`. That's false, because the majors match. The real problem is that 1.6.6 is below the constraint. The message is picked from the operator alone, not from the check that actually failed.

`Validate` should report why the version failed. Each message is `<version> ...` followed by the constraint's version as written, for example `2.x`. Pass/fail results must not change.

**Caret (`^`)**, checked in this order:
- Version below the constraint: `%s is less than %s`. This applies even when the majors also differ. Examples: `^1.12.7` with 1.6.6 gives `1.6.6 is less than 1.12.7`, `^2.x` with 1.1.1 gives `1.1.1 is less than 2.x`, `^0.2` with 0.1.1 gives `0.1.1 is less than 0.2`, and `^0.0.3` with 0.0.2 gives `0.0.2 is less than 0.0.3`.
- Wrong major, when the constraint's major is above 0 (or its minor is a wildcard), or when the constraint's major is 0 and the version's isn't: `%s does not have same major version as %s`. Examples: `^1.1` with 4.3.2, and `^1.x` with 2.1.1.
- Constraint major 0 and minor above 0 (or a wildcard or missing patch), with a different minor: `0.3.0 does not have same minor version as 0.2. Expected minor versions to match when constraint major version is 0`.
- Constraint of the form `0.0.z` with any other version not below it: `0.1.1 does not equal 0.0.3. Expect version and constraint to equal when major and minor versions are 0`. The same message applies to 0.0.4 against `^0.0.3`. Note the wording "Expect" here and "Expected" in the previous message.

**Tilde (`~`)**, and also bare or `=` constraints with a wildcard or missing part (these already behave like tilde):
- Version below the constraint: `is less than`. Examples: `~1.2.3` with 1.2.2 gives `1.2.2 is less than 1.2.3`, `2.x` with 1.2.3 gives `1.2.3 is less than 2.x`, `2` with 1.2.3 gives `1.2.3 is less than 2`, and `= 2.0` with 1.2.3 gives `1.2.3 is less than 2.0`.
- Different major: `does not have same major version as`. Examples: `~1` with 2.1.2, `~1.x` with 2.1.1, and `~1.3` with 2.4.5.
- Same major, different minor: `does not have same major and minor version as`. Examples: `~1.2.3` with 1.3.2, and `~1.1` with 1.2.3.

Messages for the other operators stay as they are. Exact `=` mismatches say `is not equal to`. `!=`, `<`, `>`, `<=` and `>=` keep their current wording, and so does the prerelease message.

End to end, validating 1.2.3 against `!= 1.2.5, ^2, <= 1.1.x` should return exactly two errors, `1.2.3 is less than 2` and `1.2.3 is greater than 1.1.x`.

Inside the package, the unexported `check` method on `*constraint` should return `(bool, error)` instead of just `bool`, where the error carries the reason above. Package code then calls it as `ok, err := c.check(v)`.

`go test ./...` should pass.
