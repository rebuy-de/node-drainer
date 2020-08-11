package collectors

import "strings"

// InstancesBy is a function type that defines the order and is used by Sort and
// SortReverse.
type InstancesBy func(i1, i2 *Instance) bool

// InstancesByID defines the order based on the Instance ID.
func InstancesByID(i1, i2 *Instance) bool {
	return i1.InstanceID < i2.InstanceID
}

// InstancesByLaunchTime defines the order based on the instance start time.
func InstancesByLaunchTime(i1, i2 *Instance) bool {
	return i1.EC2.LaunchTime.Before(i2.EC2.LaunchTime)
}

// InstancesByTriggeredAt defines the order based on the time of the ASG Shudown
// Lifecycle.
func InstancesByTriggeredAt(i1, i2 *Instance) bool {
	return i1.ASG.TriggeredAt.Before(i2.ASG.TriggeredAt)
}

func InstancesByEC2State(i1, i2 *Instance) bool {
	// The state simply gets looked up in this list and the ordering happens by
	// the returned index. If the state does not get found, the index is -1 and
	// therefore it will be put at the beginning of the list.
	const ec2StateOrder = "pending running stopping stopped shutting-down terminated"

	order1 := strings.Index(ec2StateOrder, i1.EC2.State)
	order2 := strings.Index(ec2StateOrder, i2.EC2.State)
	return order1 < order2
}
