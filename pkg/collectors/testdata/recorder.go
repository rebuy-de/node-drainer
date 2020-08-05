package testdata

import "testing"

type Recorder <-chan string

func (r Recorder) poll() *string {
	select {
	case event := <-r:
		return &event
	default:
		return nil
	}
}

func (r Recorder) Expect(t *testing.T, want string) {
	t.Helper()

	have := r.poll()
	if have == nil {
		t.Fatalf("Missing recorder value. WANT: %#v. HAVE nothing.", want)
	}

	if *have != want {
		t.Fatalf("Wrong recorder value. WANT: %#v. HAVE: %#v.", want, *have)
	}
}

func (r Recorder) ExpectEmpty(t *testing.T) {
	t.Helper()

	have := r.poll()
	if have != nil {
		t.Fatalf("Unexpected recorder value. WANT nothing. HAVE: %#v.", *have)
	}
}
