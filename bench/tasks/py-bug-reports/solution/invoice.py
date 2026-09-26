"""Build simple invoices."""

from decimal import ROUND_HALF_UP, Decimal

DISCOUNTS = {"SAVE10": 0.10, "SAVE25": 0.25}


def _money(x):
    return float(Decimal(str(x)).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP))


def line_total(quantity, unit_price):
    return _money(Decimal(str(quantity)) * Decimal(str(unit_price)))


def apply_discount(amount, code):
    if not code:
        return amount
    rate = DISCOUNTS.get(code.upper())
    if rate is None:
        raise ValueError(f"unknown discount code: {code}")
    return _money(Decimal(str(amount)) * (1 - Decimal(str(rate))))


def tax_breakdown(net, tax_rate):
    tax = _money(Decimal(str(net)) * Decimal(str(tax_rate)))
    effective = tax / net / tax_rate if net and tax_rate else 0.0
    return {"net": net, "tax": tax, "gross": _money(Decimal(str(net)) + Decimal(str(tax))), "effective_rate": effective}


def invoice_number(date, sequence):
    return f"INV-{date:%Y%m%d}-{sequence:03d}"
