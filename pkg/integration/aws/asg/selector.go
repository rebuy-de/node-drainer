package asg

type InstanceSelector func(i Instance) bool

func IsWaiting(i Instance) bool {
	return i.CompletedAt.IsZero() && i.DeletedAt.IsZero()
}
