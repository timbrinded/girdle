def parse_duration(text: str) -> int:
    """Parse a duration such as "1h30m", "45s", "2h", "1h 5m 3s" or "90m" into seconds.

    Rules:
    - Units are h (hours), m (minutes) and s (seconds). Each unit may appear at
      most once, and units must appear in that order.
    - Whitespace between parts is allowed. Leading and trailing whitespace is ignored.
    - A bare whole number with no unit means seconds: "30" is 30.
    - Units are case-insensitive: "1H30M" is 5400.
    - Anything else raises ValueError, including an empty string, an unknown
      unit, a negative or fractional number, a repeated unit, units out of
      order, and a bare number combined with units ("1h30").
    """
    raise NotImplementedError
