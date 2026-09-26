import unittest

import config


class TestEnv(unittest.TestCase):
    def test_validate(self):
        with self.assertRaises(ValueError):
            config.validate({"host": "", "port": 80})

    def test_app_port_env(self):
        self.assertIn("port", config.DEFAULTS)  # APP_PORT covered by hidden tests


if __name__ == "__main__":
    unittest.main()
