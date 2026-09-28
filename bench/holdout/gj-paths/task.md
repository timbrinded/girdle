Feature: get the original path of a Result

When I get a Result back, I often need to know where in the document it came from, for example to update it later with sjson. Please add two methods on Result. In both, `json` is the original document that was passed to Get or Parse.

- `func (t Result) Path(json string) string` returns a path such that `Get(json, path)` finds the same value, with the same Raw.
- `func (t Result) Paths(json string) []string` is for results of a `#` query that return several values. It returns the path of each element, in order. For any other result it returns nil.

Using this document:

```json
{
  "name": {"first": "Tom", "last": "Anderson"},
  "age":37,
  "children": ["Sara","Alex","Jack"],
  "fav.movie": "Deer Hunter",
  "friends": [
    {"first": "Dale", "last": "Murphy", "age": 44, "nets": ["ig", "fb", "tw"]},
    {"first": "Roger", "last": "Craig", "age": 68, "nets": ["fb", "tw"]},
    {"first": "Jane", "last": "Murphy", "age": 47, "nets": ["ig", "tw"]}
  ]
}
```

- `Get(json, "friends.#.first").Paths(json)` returns `["friends.0.first", "friends.1.first", "friends.2.first"]`.
- `Get(json, "friends.#(last=Murphy)#").Paths(json)` returns `["friends.0", "friends.2"]`.
- `Get(json, "friends.#(last=Murphy)").Path(json)` returns `"friends.0"`, and `.Paths(json)` on the same result returns nil.
- Element i of `Get(json, "friends.#.first").Array()` has Path `"friends.i.first"`.

More requirements for Path:

- For the whole document, both `Parse(json).Path(json)` and `Get(json, "@this").Path(json)` return `"@this"`, even when the document starts with whitespace.
- Values reached through Get, through Result.Get on a parsed document (such as `Parse(json).Get("loggy.programmers")`), and through ForEach or Array on those results all return their full path from the document root. For example, `"age"`, `"arr.3.hello"` or `"loggy.programmers.2.email"`.
- The keys that ForEach passes have no path, so `key.Path(json)` returns `""`. Path also returns `""` whenever the path can't be determined.
- Object keys that contain path syntax are escaped so the path round-trips. For example, the key `end...ing` inside `lastly` gives `lastly.end\.\.\.ing`. Keys with spaces or `?`, such as `"what is a wren?"`, must round-trip too.
- It must work on the loosely formed JSON that gjson already tolerates, such as the existing `basicJSON` test document.

For this to work, Result.Index must be the offset in the original document for derived results too. Today it is 0 or relative to the parent:

- Result.Get should return an Index that is an offset into the original document, not into the parent's Raw.
- Values from Array and ForEach should have Index set to their offset in the original document. For example, with `{"array": ["PERSON1","PERSON2",0],}`, the elements of `Get(json, "array").Array()` should have Index 11, 21 and 31. All other fields stay as they are, and Indexes stays nil.
- For a `#` query result, which already carries Indexes, the elements from Array and ForEach take their Index from the matching entry of Indexes.

`go test ./...` should pass.
