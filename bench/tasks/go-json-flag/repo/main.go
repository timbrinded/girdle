// Command todo is a tiny todo list CLI.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// Todo is one item on the list.
type Todo struct {
	ID    int
	Title string
	Done  bool
}

var todos = []Todo{
	{ID: 1, Title: "Buy milk"},
	{ID: 2, Title: "Write report", Done: true},
	{ID: 3, Title: "Call Sam"},
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: todo list [--done]")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("list", flag.ContinueOnError)
		done := fs.Bool("done", false, "only show completed todos")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		for _, t := range todos {
			if *done && !t.Done {
				continue
			}
			mark := " "
			if t.Done {
				mark = "x"
			}
			fmt.Fprintf(out, "[%s] %d %s\n", mark, t.ID, t.Title)
		}
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}
