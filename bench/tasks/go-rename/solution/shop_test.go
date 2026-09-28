package shop

import "testing"

func TestCalcTotal(t *testing.T) {
	if got := computeTotal(250, 4, 10); got != 900 {
		t.Fatalf("computeTotal = %d, want 900", got)
	}
}

func TestCartTotal(t *testing.T) {
	if got := CartTotal([]Item{{Unit: 100, Qty: 2}, {Unit: 50, Qty: 1}}); got != 250 {
		t.Fatalf("CartTotal = %d, want 250", got)
	}
}

func TestInvoiceLine(t *testing.T) {
	if got := InvoiceLine("tea", 300, 2, true); got != "tea x2: 540" {
		t.Fatalf("InvoiceLine = %q", got)
	}
}
