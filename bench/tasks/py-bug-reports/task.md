Fix these four bug reports in `invoice.py`, and add a test for each one:

1. Invoices with a 0% tax rate crash with `ZeroDivisionError`.
2. Line totals are sometimes a cent off. For example 3 × 0.10 should be 0.30, and 0.125 should round to 0.13. Amounts should be rounded half up to 2 decimal places.
3. Discount codes are case-sensitive: `save10` should work exactly like `SAVE10`.
4. The 10th invoice of a day is numbered `INV-20260926-10`, but numbers should be zero-padded to three digits: `INV-20260926-010`.
