package asg

// Deprecated
type InstanceSelector func(i Instance) bool

func IsWaiting(i Instance) bool {
	return !i.Completed
}
