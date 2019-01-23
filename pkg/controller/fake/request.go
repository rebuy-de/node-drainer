package fake

type Request struct {
	ID       string
	Fastpath bool
}

func NewRequest(id string, fp bool) Request {
	return Request{
		ID:       id,
		Fastpath: fp,
	}
}

func (r Request) InstanceID() string { return r.ID }
func (r Request) Heartbeat() error   { return nil }
func (r Request) UseFastpath() bool  { return r.Fastpath }
func (r Request) Clean() error       { return nil }
