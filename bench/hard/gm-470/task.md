`Node.Text(source)` returns the wrong text for many block nodes, and for some inline nodes, especially when the node is the last thing in the document. Make every node return its text as follows. Each block case must also hold when the block is the last thing in the document.

| Node | Source | `Text(source)` |
|---|---|---|
| ATX heading | `# l1` | `l1` |
| Setext heading | `l1\nl2\n===` | `l1\nl2` |
| Indented code block | `    l1\n    l2` | `l1\nl2\n` |
| Fenced code block | a fence around `l1\nl2` | `l1\nl2\n` |
| Blockquote | `> l1\n> l2` | `l1\nl2` |
| List item | `- l1\n  l2` | `l1\nl2` |
| HTML block | `<div>\nl1\nl2\n</div>` | `<div>\nl1\nl2\n</div>\n` (unterminated at the end: `<div>\nl3\nl4`) |
| Definition list (extension) | `c1\n:   c2\n    c3` | `c1c2\nc3` |
| Table (extension) | `\| h1 \| h2 \|` / `\| -- \| -- \|` / `\| c1 \| c2 \|` | `h1h2c1c2` |
| Code span | `` `c1` `` | `c1` |
| Emphasis | `*c1 **c2***` | `c1 c2` |
| Link | `[label](url)` | `label` |
| Autolink | `<http://url>` | `http://url` |
| Raw HTML | `<span>c1</span>` | `<span>` (the first raw HTML node) |
| Strikethrough (extension) | `~c1 *c2*~` | `c1 c2` |

`go test ./...` must pass (its `Performance` tests may be skipped).
