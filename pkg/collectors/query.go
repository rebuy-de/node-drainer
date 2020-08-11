package collectors

// InstanceQuery returns a dummy selector that selects all instances. It is
// used to make chaining selectors prettier while making sure the type is
// correct.
//
// Without:
//    Selector(HasEC2Data).
//        Select(HasASGData).
//        Filter(LifecycleDeleted)
// With:
//    InstanceQuery().
//        Select(HasEC2Data).
//        Select(HasASGData).
//        Filter(LifecycleDeleted)
func InstanceQuery() Selector {
	return func(i *Instance) bool {
		return true
	}
}

func (s1 Selector) Select(s2 Selector) Selector {
	return Selector(func(i *Instance) bool {
		return s1(i) && s2(i)
	})
}

func (s1 Selector) Filter(s2 Selector) Selector {
	return Selector(func(i *Instance) bool {
		return s1(i) && !s2(i)
	})
}

func (is Selector) FilterByAllPods(ps PodSelector) Selector {
	return Selector(func(i *Instance) bool {
		if !is(i) {
			return false
		}

		for _, pod := range i.Pods {
			if !ps(&pod) {
				return false
			}
		}

		return true
	})
}

// PodQuery returns a dummy selector that selects all pods. It is
// used to make chaining selectors prettier while making sure the type is
// correct. See InstanceQuery for an example.
func PodQuery() PodSelector {
	return func(p *Pod) bool {
		return true
	}
}

func (ps1 PodSelector) Select(ps2 PodSelector) PodSelector {
	return func(p *Pod) bool {
		return ps1(p) && ps2(p)
	}
}

func (ps1 PodSelector) Filter(ps2 PodSelector) PodSelector {
	return func(p *Pod) bool {
		return ps1(p) && !ps2(p)
	}
}

func (ps PodSelector) SelectByInstance(is Selector) PodSelector {
	return func(p *Pod) bool {
		return ps(p) && is(&p.Instance)
	}
}
