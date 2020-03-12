package syncutil

import (
	"context"
	"time"
)

// Signal represents an event, but without containing data. Underneath it is a
// simple receiving channel that gets closes as the signal fires.
type Signal <-chan struct{}

// SignalEmitter implements a sync mechanism for Goroutines. It can be used by
// one Goroutine to notify other Goroutines that something happenend. This can
// be useful to implement a control loop, that depends on data that is fetched
// asynchronously:
//
// - With a single emitter it helps skipping bursts, because it does not
// acumulate Signals while nothing is waiting for them.
//
// - With multiple emitters it helps by having a single blocking call.
type SignalEmitter struct {
	signalers []*signaler
}

// Emit fires a new Signal to all Signalers that were created with
// NewSignaler.
func (e *SignalEmitter) Emit() {
	for _, s := range e.signalers {
		s.emit()
	}
}

// Signaler is provided by a SignalEmitter and used by Goroutines that are
// waiting for a Signal. A Signaler should not be shared between Goroutines and
// its function/channels must not be called in parallel.
type Signaler interface {
	// Wait blocks until a Signal fires or the timeout passed. It does not make
	// any distiction which of those two happened.
	Wait(ctx context.Context, timeout time.Duration)

	// C provides a channel that gets closed when a Signal fires or the timeout
	// passed. The channel processing should happen immediately:
	//
	//		<-signaler.C(time.Minute) // Identical to signaler.Wait(time.Minute)
	//
	// Using C over Wait is only useful when using select:
	//
	//		select {
	//		case <-signaler.C(time.Minute):
	//			// Signal emitted or timed out.
	//		case <-something.Done():
	//			// Notification from some generic channel.
	//		}
	C(ctx context.Context, timeout time.Duration) Signal
}

type signaler struct {
	emits chan struct{}
}

func newSignaler() *signaler {
	return &signaler{
		emits: make(chan struct{}, 1),
	}
}

func (s *signaler) emit() {
	// The select with the default prevents the channel send from blocking.
	select {
	case s.emits <- struct{}{}:
	default:
		// Swallowing emit, because there is already a signal pending.
	}
}

func (s *signaler) Wait(ctx context.Context, timeout time.Duration) {
	<-s.C(ctx, timeout)
}

func (s *signaler) C(ctx context.Context, timeout time.Duration) Signal {
	signal := make(chan struct{})

	select {
	case <-s.emits:
		close(signal)
	default:
		go s.wait(ctx, timeout, signal)
	}

	return Signal(signal)
}

func (s *signaler) wait(ctx context.Context, timeout time.Duration, signal chan struct{}) {
	timer := time.NewTimer(timeout)
	defer func() {
		if !timer.Stop() {
			<-timer.C
		}
	}()

	// Wait for either a new emit call or the timeout.
	select {
	case <-s.emits:
	case <-timer.C:
	case <-ctx.Done():
	}

	close(signal)
}

// NewSignaler creates a new Signaler, that fires on Emit calls. The resulting
// Signaler should not be shared between multiple Goroutines, but it is
// possible to create multiple Signalers from the same Emitters.
func SignalerFromEmitters(emitters ...*SignalEmitter) Signaler {
	result := newSignaler()

	for _, e := range emitters {
		e.signalers = append(e.signalers, result)
	}

	return result
}
