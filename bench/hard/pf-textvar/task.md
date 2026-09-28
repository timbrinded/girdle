Add the equivalent of the standard library's `flag.TextVar`, for flags whose values implement `encoding.TextUnmarshaler` and `encoding.TextMarshaler`, such as `time.Time`:

- `func (f *FlagSet) TextVar(p encoding.TextUnmarshaler, name string, value encoding.TextMarshaler, usage string)`
- `func (f *FlagSet) TextVarP(p encoding.TextUnmarshaler, name, shorthand string, value encoding.TextMarshaler, usage string)`
- package-level `TextVar` and `TextVarP` with the same arguments
- `func (f *FlagSet) GetText(name string, out encoding.TextUnmarshaler) error`, which reads the flag's current value into `out`

For example, with `f.TextVar(&t, "time", time.Now(), "time stamp")`, `--time=2003-01-02T15:04:05Z` sets `t`, and a value `UnmarshalText` rejects is a parse error. The default must print correctly in the usage text. `go test ./...` must pass.
