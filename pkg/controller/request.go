package controller

type Request struct {
	InstanceID string
	Fastpath   bool
	OnDone     func()
}
