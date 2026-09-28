// Package jobs runs background work against the store.
package jobs

import (
	"context"

	"example.com/kv/store"
)

// Copy copies each key in keys from src to dst, stopping if ctx is cancelled.
func Copy(ctx context.Context, src, dst *store.Store, keys []string) (int, error) {
	n := 0
	for _, k := range keys {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		v, err := src.Get(ctx, k)
		if err != nil {
			continue
		}
		if err := dst.Put(ctx, k, v); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
