package collectors

import "github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"

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

// Selector is a function type that defines if an instance should be selected
// and is used by Select and Filter.
type Selector func(i *Instance) bool

// IsWaiting returns true, if the instance is waiting for shutdown.
func IsWaiting(i *Instance) bool {
	return HasEC2Data(i) &&
		i.ASG.ID != "" &&
		i.EC2.State == ec2.InstanceStateRunning &&
		i.ASG.Completed == false
}

// HasEC2Data returns true, if EC2 data is present.
func HasEC2Data(i *Instance) bool {
	return i != nil && i.EC2.InstanceID != ""
}

// HasASGData returns true, if ASG data is present.
func HasASGData(i *Instance) bool {
	return i != nil && i.ASG.ID != ""
}

// HasLifecycleMessage returns true, if an undeleted Lifecycle message is
// present.
func HasLifecycleMessage(i *Instance) bool {
	return HasASGData(i) && i.ASG.Deleted == false
}

func HasEC2State(states ...string) Selector {
	return func(i *Instance) bool {
		for _, state := range states {
			if i.EC2.State == state {
				return true
			}
		}

		return false
	}
}
