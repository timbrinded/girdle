A reference link whose label spans two lines isn't recognised. CommonMark allows a line break inside a link label, so

```
This is a [test][foo
bar] 1...2..3...

[foo bar]: /
```

should render `<p>This is a <a href="/">test</a> 1...2..3...</p>`, matching the definition `[foo bar]`. Find the cause and fix it without breaking the CommonMark spec tests. `go test ./...` must pass (its `Performance` tests may be skipped).
