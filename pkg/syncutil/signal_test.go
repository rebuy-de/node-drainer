package syncutil

import (
	"context"
	"testing"
	"time"
)

func TestSignalInitiallyEmpty(t *testing.T) {
	emitter := new(SignalEmitter)
	signaler := SignalerFromEmitters(emitter)

	select {
	case <-signaler.C(context.Background(), time.Hour):
		t.Fatal("signaler should not trigger yet")
	default:
	}
}

func TestSignalShouldStoreEvent(t *testing.T) {
	emitter := new(SignalEmitter)
	signaler := SignalerFromEmitters(emitter)

	emitter.Emit()

	select {
	case <-signaler.C(context.Background(), time.Hour):
	default:
		t.Fatal("signaler forgot about the previous emit")
	}
}

func TestSignalDeduplication(t *testing.T) {
	emitter := new(SignalEmitter)
	signaler := SignalerFromEmitters(emitter)

	for i := 0; i < 5; i++ {
		emitter.Emit()
	}

	select {
	case <-signaler.C(context.Background(), time.Hour):
	default:
		t.Fatal("signaler forgot about the previous emits")
	}

	select {
	case <-signaler.C(context.Background(), time.Hour):
		t.Fatal("signaler should not trigger a second time")
	default:
	}
}

func TestSignalMultipleEmitters(t *testing.T) {
	emitter1 := new(SignalEmitter)
	emitter2 := new(SignalEmitter)
	sig := SignalerFromEmitters(emitter1, emitter2)

	select {
	case <-sig.C(context.Background(), time.Hour):
		t.Fatal("signaler should not trigger yet")
	default:
	}

	for i := 0; i < 3; i++ {
		emitter1.Emit()
		emitter2.Emit()
	}

	select {
	case <-sig.C(context.Background(), time.Hour):
	default:
		t.Fatal("signaler forgot about the previous emits")
	}

	select {
	case <-sig.C(context.Background(), time.Hour):
		t.Fatal("signaler should not trigger a second time")
	default:
	}
}
