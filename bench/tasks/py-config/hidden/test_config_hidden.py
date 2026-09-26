import json
import os
import tempfile
import unittest
from unittest import mock

import config


def write(data):
    f = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
    json.dump(data, f)
    f.close()
    return f.name


class TestHiddenConfig(unittest.TestCase):
    def test_env_overrides(self):
        path = write({"host": "file-host", "port": 1000})
        env = {"APP_HOST": "env-host", "APP_PORT": "2000", "APP_DEBUG": "Yes"}
        with mock.patch.dict(os.environ, env, clear=False):
            cfg = config.load(path)
        self.assertEqual(cfg["host"], "env-host")
        self.assertEqual(cfg["port"], 2000)
        self.assertIs(cfg["debug"], True)

    def test_debug_false_values(self):
        path = write({})
        for value in ["0", "no", "false", "maybe", ""]:
            with mock.patch.dict(os.environ, {"APP_DEBUG": value}, clear=False):
                self.assertIs(config.load(path)["debug"], False, value)

    def test_no_env_keeps_file(self):
        path = write({"host": "h", "port": 81})
        clean = {k: v for k, v in os.environ.items() if not k.startswith("APP_")}
        with mock.patch.dict(os.environ, clean, clear=True):
            cfg = config.load(path)
        self.assertEqual((cfg["host"], cfg["port"]), ("h", 81))

    def test_validate(self):
        config.validate({"host": "h", "port": 1, "debug": False})
        config.validate({"host": "h", "port": 65535, "debug": False})
        for bad in [{"host": "h", "port": 0}, {"host": "h", "port": 70000}, {"host": "", "port": 80}, {"host": "h", "port": "80"}]:
            with self.assertRaises(ValueError, msg=str(bad)):
                config.validate(bad)

    def test_load_validates(self):
        path = write({"port": 0})
        clean = {k: v for k, v in os.environ.items() if not k.startswith("APP_")}
        with mock.patch.dict(os.environ, clean, clear=True):
            with self.assertRaises(ValueError):
                config.load(path)


if __name__ == "__main__":
    unittest.main()
