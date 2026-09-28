import datetime
import unittest

import invoice


class TestHiddenInvoice(unittest.TestCase):
    def test_zero_tax(self):
        b = invoice.tax_breakdown(50.0, 0.0)
        self.assertEqual((b["net"], b["tax"], b["gross"]), (50.0, 0.0, 50.0))

    def test_rounding(self):
        self.assertEqual(invoice.line_total(3, 0.10), 0.30)
        self.assertEqual(invoice.line_total(1, 0.125), 0.13)
        self.assertEqual(invoice.line_total(1, 2.675), 2.68)
        self.assertEqual(invoice.line_total(7, 1.005), 7.04)

    def test_discount_case(self):
        self.assertEqual(invoice.apply_discount(100.0, "save10"), 90.0)
        self.assertEqual(invoice.apply_discount(100.0, "Save25"), 75.0)
        with self.assertRaises(ValueError):
            invoice.apply_discount(100.0, "nope")

    def test_invoice_number_padding(self):
        d = datetime.date(2026, 9, 26)
        self.assertEqual(invoice.invoice_number(d, 10), "INV-20260926-010")
        self.assertEqual(invoice.invoice_number(d, 7), "INV-20260926-007")
        self.assertEqual(invoice.invoice_number(d, 1234), "INV-20260926-1234")


if __name__ == "__main__":
    unittest.main()
