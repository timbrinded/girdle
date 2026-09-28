Bytes() rounds some sizes up by a whole unit

`humanize.Bytes(31450000)` returns `"32 MB"`. That's 31.45 MB, which to two significant digits is 31 MB, so I'd expect `"31 MB"`. It looks like the value gets rounded up to 31.5 first and then rounded up again when it's formatted.

Expected:

```go
humanize.Bytes(31350000) // "31 MB" (already correct, should stay that way)
humanize.Bytes(31450000) // "31 MB" (currently "32 MB")
```

Nothing else should change: all the other existing outputs of Bytes, BytesN, IBytes and IBytesN (for example `Bytes(9999*1000) == "10 MB"`, `Bytes(GByte-KByte) == "1000 MB"`, `BytesN(1234, 3) == "1.23 kB"`) must stay the same.

`go test ./...` should pass.
