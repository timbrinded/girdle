import json
import os
import tempfile
import unittest

import config


class TestLoad(unittest.TestCase):
    def test_defaults(self):
        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False) as f:
            json.dump({"port": 9000}, f)
        try:
            cfg = config.load(f.name)
        finally:
            os.unlink(f.name)
        self.assertEqual(cfg, {"host": "127.0.0.1", "port": 9000, "debug": False})


if __name__ == "__main__":
    unittest.main()
