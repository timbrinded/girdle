package kernel

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// commonTools are the commands whose presence is worth telling the model
// about, so it doesn't waste a step guessing (for example python vs python3).
var commonTools = []string{"git", "go", "python3", "python", "node", "npm", "bun", "cargo", "java", "ruby", "make"}

func availableTools() string {
	var found []string
	for _, t := range commonTools {
		if _, err := exec.LookPath(t); err == nil {
			found = append(found, t)
		}
	}
	if len(found) == 0 {
		return "none detected"
	}
	return strings.Join(found, ", ")
}

func systemPrompt(cfg Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, `You are Girdle, a coding agent working in the repository at %s (%s/%s). Today is %s. Commands available on PATH: %s.

Use the tools to read and change files and to run commands. Work until the task is completely done: make the change, then verify it by building and running the relevant tests, and fix anything that fails.

When you finish, reply briefly with what you changed and the evidence that it works. Only ask the user a question when you need a decision that only they can make.`,
		cfg.Dir, runtime.GOOS, runtime.GOARCH, time.Now().Format("2 January 2006"), availableTools())
	if cfg.Snapshot {
		b.WriteString(`

Each request starts with a repository snapshot, taken just before the request. For a small repository it holds the full text of every file: work from it, and don't read those files again. For a large one it lists every file, includes the repository's instructions for agents, and already shows the code the request names: its files or definitions, its uses and the tests beside it. Don't look those up again.`)
	}
	if cfg.Batch {
		b.WriteString(`

Every response you send costs the user several seconds, so finish in as few as you can. Work in at most three moves: gather, change, and only if needed fix.
- Gather: if the snapshot isn't enough, make one lookup call with every file, line range, definition and search you will need. Ask generously rather than coming back for more.
- Change: make one apply call with every edit and new file the task needs, and set its check to a command that proves the whole task is done. That means building the code and running the tests, plus a quick check for any part of the task the tests can't show, such as grep confirming a renamed name is gone everywhere, comments included. When the task reports a bug, put a test that reproduces it in the same apply as the fix, so the check shows it fixed.`)
		if cfg.Reproduce {
			b.WriteString(` Set reproduce to a command that runs only that test: Girdle runs it once without your fix to show it fails there.`)
		}
		// This once listed examples taken from a benchmark task's hidden
		// test. They were removed at no cost (decision 0016).
		b.WriteString(` Before you write, work out the edge cases the task's words imply, and make the code handle them and the tests cover them: one attempt has to be right. apply runs the check straight after the changes, so one response both changes and verifies the code.
- Fix: if the check fails, send one more apply with the fixes. Start its check with a quick run of just what failed, joined to the full proof with &&, so a repeat failure shows in seconds.
The check must fail when anything is wrong, so never hide its exit code with "; echo" or "|| true". Keep any text to a sentence.

Writing takes time too, so write as little as the task allows. Change existing files with old_text and new_text edits, each old_text short but unique; use content only for new files or files you are mostly rewriting. Changes apply in order, so never let two changes touch the same lines: merge them into one. Keep new tests compact: one focused test per behaviour.`)
	}
	return b.String()
}
