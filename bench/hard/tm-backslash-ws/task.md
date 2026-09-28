In a TOML multi-line basic string, a backslash at the end of a line removes the newline and all whitespace, including further newlines, up to the next non-whitespace character. The decoder gets this wrong when whitespace follows the backslash on its line, or when the next line is blank. For example

```toml
only-ignore-first = """
Here are \
  two
lines of text.
And \

  another
  two.
"""
```

must decode to `"Here are two\nlines of text.\nAnd another\n  two.\n"`. `go test ./...` must pass.
