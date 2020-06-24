package collectors

import "strings"

// By is a function type that defines the order and is used by Sort and
// SortReverse.
type By func(i1, i2 *Instance) bool

// ByInstanceID defines the order based on the Instance ID.
func ByInstanceID(i1, i2 *Instance) bool {
	return i1.InstanceID < i2.InstanceID
}

// ByLaunchTime defines the order based on the instance start time.
func ByLaunchTime(i1, i2 *Instance) bool {
	return i1.EC2.LaunchTime.Before(i2.EC2.LaunchTime)
}

// ByTriggeredAt defines the order based on the time of the ASG Shudown
// Lifecycle.
func ByTriggeredAt(i1, i2 *Instance) bool {
	return i1.ASG.TriggeredAt.Before(i2.ASG.TriggeredAt)
}

func ByEC2State(i1, i2 *Instance) bool {
	// The state simply gets looked up in this list and the ordering happens by
	// the returned index. If the state does not get found, the index is -1 and
	// therefore it will be put at the beginning of the list.
	const ec2StateOrder = "pending running stopping stopped shutting-down terminated"

	order1 := strings.Index(ec2StateOrder, i1.EC2.State)
	order2 := strings.Index(ec2StateOrder, i2.EC2.State)
	return order1 < order2
}

// Selector is a function type that defines if an instance should be selected
// and is used by Select and Filter.
type Selector func(i *Instance) bool
