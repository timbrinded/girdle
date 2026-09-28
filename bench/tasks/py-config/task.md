Make these changes:

1. In `config.py`, environment variables override values from the file: `APP_HOST`, `APP_PORT` and `APP_DEBUG` override `host`, `port` and `debug`. Convert `APP_PORT` to an int. `APP_DEBUG` is true for `1`, `true` or `yes` (any case) and false for anything else.
2. Add `validate(cfg)` to `config.py`. It raises `ValueError` if `port` is not an int between 1 and 65535, or if `host` is empty.
3. Make `load` call `validate` before returning.
4. Document the environment variables and the validation rules in the Usage section of `README.md`.
5. Add tests for all of the above.
