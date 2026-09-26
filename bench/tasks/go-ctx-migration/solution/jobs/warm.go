package jobs

import (
	"context"

	"example.com/kv/store"
)

// Warm fills the store with default values that aren't set yet.
func Warm(s *store.Store, defaults map[string]string) error {
	for k, v := range defaults {
		if _, err := s.Get(context.Background(), k); err == nil {
			continue
		}
		if err := s.Put(context.Background(), k, v); err != nil {
			return err
		}
	}
	return nil
}
