package shop

// Item is one line in a cart.
type Item struct {
	Unit int
	Qty  int
}

// CartTotal sums the items with no discount.
func CartTotal(items []Item) int {
	sum := 0
	for _, it := range items {
		sum += calcTotal(it.Unit, it.Qty, 0)
	}
	return sum
}
