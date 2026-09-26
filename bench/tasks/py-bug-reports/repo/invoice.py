"""Build simple invoices."""

DISCOUNTS = {"SAVE10": 0.10, "SAVE25": 0.25}


def line_total(quantity, unit_price):
    return round(quantity * unit_price, 2)


def apply_discount(amount, code):
    if not code:
        return amount
    rate = DISCOUNTS.get(code)
    if rate is None:
        raise ValueError(f"unknown discount code: {code}")
    return round(amount * (1 - rate), 2)


def tax_breakdown(net, tax_rate):
    tax = round(net * tax_rate, 2)
    return {"net": net, "tax": tax, "gross": round(net + tax, 2), "effective_rate": tax / net / tax_rate}


def invoice_number(date, sequence):
    return f"INV-{date:%Y%m%d}-{sequence}"
