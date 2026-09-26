package shop

import "fmt"

// InvoiceLine formats one invoice line, with a 10% discount for members.
func InvoiceLine(name string, unit, qty int, member bool) string {
	discount := 0
	if member {
		discount = 10
	}
	return fmt.Sprintf("%s x%d: %d", name, qty, calcTotal(unit, qty, discount))
}
