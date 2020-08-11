package collectors

import "time"

// Selector is a function type that defines if an instance should be selected.
type Selector func(i *Instance) bool

func HasEC2Data(i *Instance) bool { return i.HasEC2Data() }

func HasASGData(i *Instance) bool { return i.HasASGData() }

func WantsShutdown(i *Instance) bool { return i.WantsShutdown() }

func PendingLifecycleCompletion(i *Instance) bool { return i.PendingLifecycleCompletion() }

func HasLifecycleMessage(i *Instance) bool { return i.HasLifecycleMessage() }

func HasEC2State(states ...string) Selector {
	return func(i *Instance) bool { return i.HasEC2State(states...) }
}

func LifecycleTriggeredOlderThan(age time.Duration) Selector {
	return func(i *Instance) bool {
		return time.Since(i.ASG.TriggeredAt) > age
	}
}

func LifecycleDeleted(i *Instance) bool {
	return i.ASG.Deleted
}

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
