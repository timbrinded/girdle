import unittest

from handlers import invite, register, update_email


class TestHandlers(unittest.TestCase):
    def test_register(self):
        self.assertEqual(register({"email": " Ann@Example.com "}), {"ok": True, "user": "ann@example.com"})
        self.assertFalse(register({"email": "nope"})["ok"])

    def test_invite(self):
        self.assertEqual(invite({"email": "bo@x.io"}), {"ok": True, "invited": "bo@x.io"})

    def test_update_email(self):
        self.assertEqual(update_email("u1", {}), {"ok": False, "error": "invalid email"})


if __name__ == "__main__":
    unittest.main()
