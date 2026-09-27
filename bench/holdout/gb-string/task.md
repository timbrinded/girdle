Feature request: let a compiled Pattern report its source and separators

Once a pattern is compiled there's no way to get back what it was built from, so logging or displaying a `*glob.Pattern` isn't useful. Please add two methods to `*Pattern`:

**String() string** returns the pattern text exactly as it was passed to Compile or MustCompile, with escapes kept as written. With this, `*Pattern` satisfies fmt.Stringer, so `fmt.Sprint(p)` prints the same text. For example, if each of these is compiled with separators `'.', '/'`, it should come back unchanged:

```
""                  (empty pattern)
"foo"
"*.github.com"
"{cat,bat,[fr]at}"
`\*escaped\?`
"ångstr[ö]m"
```

**Separators() []rune** returns the separators the pattern was compiled with, in the order given, e.g. `[]rune{'ö', '/', '.'}`. If the pattern was compiled without separators, it returns nil, not an empty slice.

Separators() should return the caller's slice itself, not a copy. After `p := glob.MustCompile("*", seps...)`, `p.Separators()` shares the backing array of `seps`. Matching, however, must be fixed when the pattern is compiled. If the caller later changes that slice, Separators() shows the change, but Match does not:

```go
seps := []rune{'.'}
p := glob.MustCompile("*", seps...)
seps[0] = 'x'
p.Separators()   // []rune{'x'}
p.Match("a.b")   // false: '.' is still a separator
p.Match("axb")   // true: 'x' did not become one
```

`go test ./...` should pass.
