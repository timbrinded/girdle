package shop

// calcTotal returns the total price in cents for qty items at unit cents
// each, after applying a percentage discount.
func calcTotal(unit, qty, discountPct int) int {
	total := unit * qty
	return total - total*discountPct/100
}
