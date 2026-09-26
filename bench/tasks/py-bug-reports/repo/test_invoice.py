import datetime
import unittest

import invoice


class TestInvoice(unittest.TestCase):
    def test_discount(self):
        self.assertEqual(invoice.apply_discount(100.0, "SAVE10"), 90.0)

    def test_tax(self):
        self.assertEqual(invoice.tax_breakdown(100.0, 0.2)["gross"], 120.0)

    def test_invoice_number(self):
        self.assertEqual(invoice.invoice_number(datetime.date(2026, 9, 26), 7), "INV-20260926-007")


if __name__ == "__main__":
    unittest.main()
