package aws

import "github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/ec2"

type By func(i1, i2 *Instance) bool

func ByInstanceID(i1, i2 *Instance) bool {
	return i1.InstanceID < i2.InstanceID
}

func ByLaunchTime(i1, i2 *Instance) bool {
	return i1.EC2.LaunchTime.Before(i2.EC2.LaunchTime)
}

func ByTriggeredAt(i1, i2 *Instance) bool {
	return i1.ASG.TriggeredAt.Before(i2.ASG.TriggeredAt)
}

type Selector func(i *Instance) bool

func IsWaiting(i *Instance) bool {
	return HasEC2Data(i) &&
		i.ASG.ID != "" &&
		i.EC2.State == ec2.InstanceStateRunning &&
		i.ASG.Completed == false
}

func HasEC2Data(i *Instance) bool {
	return i != nil && i.EC2.InstanceID != ""
}

func HasLifecycleMessage(i *Instance) bool {
	return !i.ASG.Deleted
}
