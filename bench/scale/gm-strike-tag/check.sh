set -e
cp "$TASK_DIR/hidden/zz_hidden_strike_test.go" extension/
go vet ./...
go test -skip "Performance" ./...  # upstream timing tests fail under benchmark load
