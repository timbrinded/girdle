package checkpoint

import (
	"context"
	"sync"
	"time"

	"github.com/timbrinded/girdle/internal/jev"
)

// FileOutline is what Jev sees of a file when judging whether a request
// needs it: its path, first lines and top-level definitions. Paths alone
// weren't enough: judged by path, Jev ranked a docs index first for a code
// task.
type FileOutline struct {
	Path    string `json:"path"`
	Outline string `json:"outline"`
}

var needToRead = map[string]jev.Question{
	"need": jev.Noul("Will someone doing `task` need to read `file.path` to make the change, or to see how similar code in this repository is written?"),
}

// Prefetch is Jev's answer about every candidate file.
type Prefetch struct {
	Scores      map[string]float64
	InputTokens int64
	LatencyMS   int64
	Failed      int
}

// NeedToRead asks Jev, for each file, whether the request will need it.
// Each file gets a request of its own, all sent at once, up to workers at a
// time. Batched into shared requests of 15 files, the same question found
// 7 of the 21 files agents looked up in its top five per task, against 10
// when each file was asked about alone (decision 0013).
func NeedToRead(ctx context.Context, c *jev.Client, request string, files []FileOutline, workers int) Prefetch {
	start := time.Now()
	p := Prefetch{Scores: make(map[string]float64, len(files))}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(workers, 1))
	for _, f := range files {
		wg.Go(func() {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			call, ok := ask(ctx, c, map[string]any{"task": request, "file": f}, needToRead)
			mu.Lock()
			defer mu.Unlock()
			if !ok {
				p.Failed++
				return
			}
			p.Scores[f.Path] = call.Answers["need"].Noul
			p.InputTokens += call.InputTokens
		})
	}
	wg.Wait()
	p.LatencyMS = time.Since(start).Milliseconds()
	return p
}
