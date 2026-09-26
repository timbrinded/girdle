// Command todo is a tiny todo list CLI.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// Todo is one item on the list.
type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
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
		asJSON := fs.Bool("json", false, "print as JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		shown := []Todo{}
		for _, t := range todos {
			if *done && !t.Done {
				continue
			}
			shown = append(shown, t)
		}
		if *asJSON {
			return json.NewEncoder(out).Encode(shown)
		}
		for _, t := range shown {
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
