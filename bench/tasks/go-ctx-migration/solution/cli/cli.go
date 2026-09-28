// Package cli implements the kv command line.
package cli

import (
	"context"
	"fmt"
	"io"

	"example.com/kv/store"
)

// Run executes one command: "get KEY" or "set KEY VALUE".
func Run(s *store.Store, args []string, out io.Writer) error {
	switch {
	case len(args) == 2 && args[0] == "get":
		v, err := s.Get(context.Background(), args[1])
		if err != nil {
			return err
		}
		fmt.Fprintln(out, v)
		return nil
	case len(args) == 3 && args[0] == "set":
		return s.Put(context.Background(), args[1], args[2])
	}
	return fmt.Errorf("usage: get KEY | set KEY VALUE")
}
