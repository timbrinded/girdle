A `StringToString` flag doesn't behave like other flags when it is given more than once, and it doesn't keep one map.

- **Repeated flags merge.** `--arg a=1,b=2 --arg a=2 --arg=d=4` should give `map[a:2 b:2 d:4]`, later values winning.
- **One map, always.** The map passed as the default is the flag's storage. `StringToString(name, defval, usage)` returns a pointer to that same map, and `GetStringToString` returns that same map, both before and after values are set. The `Var` forms behave the same way.
- **The first value replaces the defaults.** The first `Set` clears the default entries from that map in place, rather than swapping in a new map. Later `Set` calls add to it. After `Set("c=3")` then `Set("d=4")`, the one map holds exactly `c` and `d`.
- **The map is live.** A caller that deletes every entry from it sees `GetStringToString` return an empty map.
- **Defaults still apply** when the flag isn't given.

`go test ./...` must pass.
