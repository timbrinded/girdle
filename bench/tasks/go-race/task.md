`go test -race ./...` fails. Make `Counter` safe for concurrent use, and make `Snapshot` return a copy that callers can change without affecting the counter. Don't change the tests.
