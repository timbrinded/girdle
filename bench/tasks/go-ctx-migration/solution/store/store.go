// Package store is an in-memory key-value store.
package store

import (
	"context"
	"errors"
	"sync"
)

// ErrNotFound is returned by Get for a missing key.
var ErrNotFound = errors.New("store: not found")

// Store is safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	data map[string]string
}

// New returns an empty store.
func New() *Store { return &Store{data: map[string]string{}} }

// Get returns the value for key.
func (s *Store) Get(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Put sets key to value.
func (s *Store) Put(ctx context.Context, key, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}
