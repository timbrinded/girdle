# app

A small web app.

## Usage

Settings are read from a JSON file:

```python
from config import load

cfg = load("settings.json")
```

Missing keys fall back to the defaults in `config.DEFAULTS`.
