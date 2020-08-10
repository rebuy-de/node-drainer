package collectors

import "time"

// Selector is a function type that defines if an instance should be selected
// and is used by Select and Filter.
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
