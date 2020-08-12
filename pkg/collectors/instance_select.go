package collectors

import "time"

// Selector is a function type that defines if an instance should be selected.
type InstanceSelector func(i *Instance) bool

func HasEC2Data(i *Instance) bool { return i.HasEC2Data() }

func HasASGData(i *Instance) bool { return i.HasASGData() }

// Deprecated: Should use filters instead.
func PendingLifecycleCompletion(i *Instance) bool { return i.PendingLifecycleCompletion() }

// Deprecated: Should use filters instead.
func HasLifecycleMessage(i *Instance) bool { return i.HasLifecycleMessage() }

func HasEC2State(states ...string) InstanceSelector {
	return func(i *Instance) bool { return i.HasEC2State(states...) }
}

func LifecycleTriggeredOlderThan(age time.Duration) InstanceSelector {
	return func(i *Instance) bool {
		return time.Since(i.ASG.TriggeredAt) > age
	}
}

func LifecycleDeleted(i *Instance) bool {
	return i.ASG.Deleted
}

func LifecycleCompleted(i *Instance) bool {
	return i.ASG.Completed
}

func HasTaint(key string) InstanceSelector {
	return func(i *Instance) bool {
		for _, taint := range i.Node.Taints {
			if taint.Key == key {
				return true
			}
		}
		return false
	}
}
