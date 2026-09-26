import re

_UNITS = re.compile(r"^(?:(\d+)h)?\s*(?:(\d+)m)?\s*(?:(\d+)s)?$", re.IGNORECASE)


def parse_duration(text: str) -> int:
    """Parse a duration such as "1h30m" into seconds."""
    s = text.strip()
    if re.fullmatch(r"\d+", s):
        return int(s)
    m = _UNITS.fullmatch(s)
    if not s or not m or not any(m.groups()):
        raise ValueError(f"invalid duration: {text!r}")
    h, mi, se = (int(g) if g else 0 for g in m.groups())
    return h * 3600 + mi * 60 + se
