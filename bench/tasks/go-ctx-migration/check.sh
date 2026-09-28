set -e
grep -q "func (s \*Store) Get(ctx context.Context, key string) (string, error)" store/store.go || { echo "FAIL: Get signature"; exit 1; }
grep -q "func (s \*Store) Put(ctx context.Context, key, value string) error" store/store.go || { echo "FAIL: Put signature"; exit 1; }
grep -q "r.Context()" api/handler.go || { echo "FAIL: api does not pass the request context"; exit 1; }
if grep -q "context.Background()" api/handler.go jobs/copy.go; then echo "FAIL: callers with a context use Background"; exit 1; fi
go vet ./...
go test ./...
cat > store/zz_hidden_test.go <<'GO'
package store

import (
	"context"
	"testing"
)

func TestHiddenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := New()
	if err := s.Put(ctx, "a", "1"); err != context.Canceled {
		t.Fatalf("Put with cancelled ctx = %v", err)
	}
	if _, err := s.Get(ctx, "a"); err != context.Canceled {
		t.Fatalf("Get with cancelled ctx = %v", err)
	}
}
GO
go test ./store/
