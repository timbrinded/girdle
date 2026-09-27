Support TOML's base-prefixed integers: hexadecimal `0xdead_BEEF`, octal `0o777` and binary `0b1101`, with underscores allowed between digits. Report invalid ones with clear errors:

- `0x_d00d`: "invalid digit following hexadecimal base designator"
- `0b_0`: "invalid digit following binary base designator"
- `0b0_`: an underscore must be "surrounded by digits"
- `00`: numbers "cannot have leading zeroes"
- `0z`: "but got 'z' instead"
- `+0x3` and `-0xf00`: a "sign cannot be used" with a base prefix

Decimal numbers and floats must keep working. `go test ./...` must pass.
