Add running statistics to more-itertools, alongside the existing `running_median`:

- `running_min(iterable, *, maxlen=None)` and `running_max(iterable, *, maxlen=None)` yield the smallest or largest value seen so far, or within a sliding window of the last *maxlen* values.
- `running_mean` gains the same keyword-only *maxlen*.
- `running_statistics(iterable, *, maxlen=None)` yields a `Stats` object for each value, with the `size`, `minimum`, `median`, `maximum` and `mean` of the values so far or in the window. `Stats` is a frozen dataclass with slots (`@dataclass(frozen=True, slots=True)`) with those five fields, in that order, exported from `more_itertools`: comparable, hashable and immutable, with no `__dict__`.

*maxlen* must be a positive integer: `maxlen=0` raises `ValueError`, and `running_statistics` raises it as soon as it is called. Export the new functions, add their type stubs to the `.pyi` files and document them in `docs/api.rst`. The whole test suite must pass: `python3 -m unittest discover -s tests -t .`
