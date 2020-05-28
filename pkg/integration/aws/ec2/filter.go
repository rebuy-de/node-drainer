package ec2

const (
	InstanceStateRunning = "running"
)

type InstanceFilter func(i Instance) bool

func IsRunning(i Instance) bool {
	return i.State == InstanceStateRunning
}

func NotRunning(i Instance) bool {
	return !IsRunning(i)
}
