Rename the exported function `IndentWidth` in package `util` to `MeasureIndent`. Update every caller, and every comment or doc that mentions it by name. `go vet ./...` and `go test ./...` must pass.
