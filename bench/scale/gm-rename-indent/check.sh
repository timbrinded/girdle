set -e
if grep -rnw "IndentWidth" --include='*.go' --include='*.md' .; then
  echo "FAIL: IndentWidth is still mentioned"; exit 1
fi
grep -q "^func MeasureIndent(" util/util.go
go vet ./...
go test ./...
