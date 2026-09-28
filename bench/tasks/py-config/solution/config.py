"""Load app settings from a JSON file, with environment overrides."""

import json
import os

DEFAULTS = {"host": "127.0.0.1", "port": 8080, "debug": False}


def validate(cfg):
    port = cfg.get("port")
    if not isinstance(port, int) or isinstance(port, bool) or not 1 <= port <= 65535:
        raise ValueError(f"port must be an int between 1 and 65535, got {port!r}")
    if not cfg.get("host"):
        raise ValueError("host must not be empty")


def load(path):
    with open(path) as f:
        data = json.load(f)
    cfg = dict(DEFAULTS)
    cfg.update(data)
    if "APP_HOST" in os.environ:
        cfg["host"] = os.environ["APP_HOST"]
    if "APP_PORT" in os.environ:
        cfg["port"] = int(os.environ["APP_PORT"])
    if "APP_DEBUG" in os.environ:
        cfg["debug"] = os.environ["APP_DEBUG"].lower() in ("1", "true", "yes")
    validate(cfg)
    return cfg
