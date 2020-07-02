package collectors

// Selector is a function type that defines if an instance should be selected
// and is used by Select and Filter.
type PodSelector func(p *Pod) bool

func PodImmuneToEviction(p *Pod) bool { return p.Pod.ImmuneToEviction() }

func PodCanDecrement(p *Pod) bool { return p.Pod.OwnerReady.CanDecrement }
