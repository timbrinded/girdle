Support unstable.Unmarshaler for tables and array tables

With `EnableUnmarshalerInterface()`, a type that implements `unstable.Unmarshaler` is only called for single values. If the type is the target of a `[table]` or of an `[[array table]]` entry, the interface isn't used. I want to decode whole tables myself, for example to choose a config struct based on a `type` key (see #873). Handing implementations an AST node is also awkward. Raw TOML text would be much easier to work with.

Proposal: change the interface in the `unstable` package to take raw bytes:

```go
type Unmarshaler interface {
	UnmarshalTOML(data []byte) error
}
```

With `EnableUnmarshalerInterface()` enabled:

1. **Single values.** `data` is the raw text of the value exactly as it appears in the document. For `foo = "bar"`, data is `"bar"`, including the quotes. Decoding `unmarshalers = [1,2,3]` into a `[]T` gives each element `1`, `2` and `3`. The existing fallback still works: when a struct that implements the interface meets a key it has no field for, it gets that value. So decoding `foo = "bar"` into such a root struct with no `foo` field calls `UnmarshalTOML([]byte("\"bar\""))`. In that case Decode returns the error from UnmarshalTOML unchanged, so `err.Error()` is exactly the original message.

2. **Tables.** When the Go value that a `[table]` header maps to implements the interface, UnmarshalTOML is called once. Its data is every key/value line of that table, each copied exactly as written and followed by `\n`. The header line is not included. The original formatting is kept: spacing around `=`, quote style, number formats such as `0xDEADBEEF`, inline tables such as `{ a = 1, b = 2 }`, and quoted and dotted keys such as `"key with spaces" = ...` and `sub.key = ...`. For example:

   ```toml
   [plugin]
   name = "example"
   version = "1.0"
   ```

   gives exactly `"name = \"example\"\nversion = \"1.0\"\n"`.

   Each table gets only its own lines. Take a document with `[a.b]` holding `C = "1"`, then `[x]` holding `Y = "100"`, then `[a.d]` holding `E = "2"`, decoded into a struct where `a.b`, `a.d` and `x` all implement the interface. Then B receives only `C = "1"`, X only `Y = "100"`, and D only `E = "2"`.

3. **Array tables.** When each `[[plugins]]` entry is decoded into an element of `[]T`, where `*T` implements the interface, each element receives only its own entry's lines. The element can then call `toml.Unmarshal(data, ...)` on them. For example, it can read `type` first and then decode the rest into a type-specific struct.

4. **Pointers.** Fields of type `*T` or `**T`, where T implements the interface, are allocated as needed and then filled.

5. **Errors.** If UnmarshalTOML returns an error for a table, Decode fails, and its error message contains the original error text.

6. **RawMessage.** Add `unstable.RawMessage`, a `[]byte` type similar to `json.RawMessage`. It implements the interface and keeps the raw bytes it receives. With the `[plugin]` example above, a field `Plugin unstable.RawMessage` holds exactly `"name = \"example\"\nversion = \"1.0\"\n"`, and it can be passed straight to `toml.Unmarshal` later. A line such as `enabled = true` is kept as written too.

7. **Parser ranges.** `KeyValue` nodes produced by `unstable.Parser` should have `Raw` set to the span of the whole expression, from the first byte of the key to the last byte of the value. The span excludes trailing whitespace and comments. This applies to key/values nested in inline tables too. Today the range is empty and prints as `1:1->1:1 (0->0)`. In the `ExampleParser_comments` document, the KeyValue lines should become:
   - `key = "value" # Next to simple value.`: `7:1->7:14 (127->140)`
   - `name = { first = "Tom", last = "Preston-Werner" }`: `15:1->15:50 (274->323)`, with its entries at `15:10->15:23 (283->296)` and `15:25->15:48 (298->321)`
   - `array = [ 1, 2, 3 ]`: `19:1->19:20 (386->405)`
   - the multi-line `key5 = [ ... ]`: `23:1->31:2 (474->694)`

   All other node ranges stay as they are. Table, Array and ArrayTable nodes still print `1:1->1:1 (0->0)`.

The `EnableUnmarshalerInterface` docs currently say tables and array tables aren't supported. Please update them.

`go test ./...` should pass, including the `unstable` package.
