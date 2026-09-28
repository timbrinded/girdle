import datetime
import unittest

import invoice


class TestBugs(unittest.TestCase):
    def test_zero_tax(self):
        self.assertEqual(invoice.tax_breakdown(50.0, 0.0)["gross"], 50.0)

    def test_rounding(self):
        self.assertEqual(invoice.line_total(1, 0.125), 0.13)

    def test_case(self):
        self.assertEqual(invoice.apply_discount(100.0, "save10"), 90.0)

    def test_padding(self):
        self.assertEqual(invoice.invoice_number(datetime.date(2026, 9, 26), 10), "INV-20260926-010")


if __name__ == "__main__":
    unittest.main()
