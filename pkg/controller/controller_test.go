package controller

import (
	"context"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/sirupsen/logrus"

	"github.com/rebuy-de/node-drainer/pkg/controller/fake"
)

func initTestController(t *testing.T) (*fake.Drainer, chan Request, *clock.Mock, func()) {
	if testing.Verbose() {
		logrus.SetLevel(logrus.DebugLevel)
	}

	clk := clock.NewMock()

	drainer := &fake.Drainer{
		Clock:         clk,
		DrainDuration: 5 * time.Minute,
		States:        make(map[string]string),
	}

	requests := make(chan Request)

	ctl := New(drainer, requests)
	ctl.clock = clk

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		err := ctl.Reconcile(ctx)
		if err != nil {
			t.Error(err)
		}
		close(done)
	}()

	return drainer, requests, clk, func() {
		cancel()
		<-done
	}
}

func TestControllerSimpleBacklog(t *testing.T) {
	drainer, requests, clk, deferer := initTestController(t)
	defer deferer()

	requests <- Request{InstanceID: "i-010", Fastpath: false}
	drainer.Assert(t, "", "")

	time.Sleep(10 * time.Millisecond) // give reconcile routine time to act
	clk.Add(10 * time.Second)
	drainer.Assert(t, "i-010", "")

	clk.Add(5 * time.Minute)
	drainer.Assert(t, "", "i-010")
}

func TestControllerSimpleFastpath(t *testing.T) {
	drainer, requests, clk, deferer := initTestController(t)
	defer deferer()

	requests <- Request{InstanceID: "i-020", Fastpath: true}

	time.Sleep(10 * time.Millisecond) // give reconcile routine time to act
	clk.Add(10 * time.Second)
	drainer.Assert(t, "i-020", "")

	clk.Add(5 * time.Minute)
	drainer.Assert(t, "", "i-020")
}

func TestControllerBacklogBlocking(t *testing.T) {
	drainer, requests, clk, deferer := initTestController(t)
	defer deferer()

	requests <- Request{InstanceID: "i-030", Fastpath: false}
	requests <- Request{InstanceID: "i-031", Fastpath: false}

	// wait for loop to pick it up
	time.Sleep(10 * time.Millisecond) // give reconcile routine time to act
	clk.Add(10 * time.Second)
	drainer.Assert(t, "i-030", "")

	// wait for actual drain to be finished
	clk.Add(5 * time.Minute)
	drainer.Assert(t, "", "i-030")

	// wait for cooldown and loop picking up the next request
	clk.Add(10 * time.Minute)
	drainer.Assert(t, "i-031", "i-030")

	// wait for next actual drain to be finished
	clk.Add(5 * time.Minute)
	drainer.Assert(t, "", "i-030,i-031")
}

func TestControllerAcknowledgeFastpath(t *testing.T) {
	drainer, requests, clk, deferer := initTestController(t)
	defer deferer()

	requests <- Request{InstanceID: "i-040", Fastpath: false}
	time.Sleep(10 * time.Millisecond) // give reconcile routine time to act

	// wait for loop to pick it up
	clk.Add(2 * time.Minute)
	drainer.Assert(t, "i-040", "")

	// insert fastpath request, while another drain is in progress
	requests <- Request{InstanceID: "i-041", Fastpath: true}
	time.Sleep(10 * time.Millisecond) // give reconcile routine time to act

	// wait for loop to pick it up
	clk.Add(1 * time.Minute)
	drainer.Assert(t, "i-040,i-041", "")

	// wait for first drain to be finished
	clk.Add(3 * time.Minute)
	drainer.Assert(t, "i-041", "i-040")

	// wait for second drain to be finished
	clk.Add(5 * time.Minute)
	drainer.Assert(t, "", "i-040,i-041")
}
