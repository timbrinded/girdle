set -e
for f in $(cd "$TASK_DIR/repo" && ls *_test.go); do
  cmp -s "$TASK_DIR/repo/$f" "$f" || { echo "FAIL: $f was changed"; exit 1; }
done
go test ./...
