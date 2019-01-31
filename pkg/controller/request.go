package controller

type Request struct {
	NodeName string
	Fastpath bool
	OnDone   func()
}
