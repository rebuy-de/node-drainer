package collectors

// By is a function type that defines the order and is used by Sort and
// SortReverse.
type PodsBy func(p1, p2 *Pod) bool

func PodsByNeedsEviction(p1, p2 *Pod) bool {
	return p1.NeedsEviction() && !p2.NeedsEviction()
}
