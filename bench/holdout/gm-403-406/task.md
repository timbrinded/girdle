Two bugs.

1. A raw HTML tag split across two lines isn't recognised. `<img src=./.assets/logo.svg\n/>` should pass through as raw HTML, `<p><img src=./.assets/logo.svg\n/></p>`. A tag split by a blank line isn't one tag, and must still be escaped as two paragraphs.
2. Links and images with a `javascript:` URL are made safe only when the scheme is written in lower case. `[x](JaVaScRiPt:alert(1))` must be neutralised like `[x](javascript:alert(1))`, rendering `href=""`. Checks for dangerous URLs must ignore case.

`go test ./...` must pass (its `Performance` tests may be skipped).
