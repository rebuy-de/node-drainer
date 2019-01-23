package controller

type Request interface {
	InstanceID() string
	Heartbeat() error
	UseFastpath() bool
	Clean() error
}
