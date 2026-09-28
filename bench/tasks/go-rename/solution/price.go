package shop

// computeTotal returns the total price in cents for qty items at unit cents
// each, after applying a percentage discount.
func computeTotal(unit, qty, discountPct int) int {
	total := unit * qty
	return total - total*discountPct/100
}
