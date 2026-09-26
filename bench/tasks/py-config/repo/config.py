"""Load app settings from a JSON file."""

import json

DEFAULTS = {"host": "127.0.0.1", "port": 8080, "debug": False}


def load(path):
    with open(path) as f:
        data = json.load(f)
    cfg = dict(DEFAULTS)
    cfg.update(data)
    return cfg
