package controller_test

import (
	"testing"

	"github.com/rebuy-de/node-drainer/pkg/controller"
)

func TestQueue(t *testing.T) {
	q := new(controller.Queue)

	q.Add(controller.Request{InstanceID: "1", Fastpath: false})
	q.Add(controller.Request{InstanceID: "2", Fastpath: false})
	q.Add(controller.Request{InstanceID: "3", Fastpath: false})

	have1 := q.Poll().InstanceID
	have2 := q.Poll().InstanceID

	q.Add(controller.Request{InstanceID: "4", Fastpath: false})

	have3 := q.Poll().InstanceID
	have4 := q.Poll().InstanceID
	have5 := q.Poll()

	if have1 != "1" {
		t.Errorf("Want 1. Have %s.", have1)
	}

	if have2 != "2" {
		t.Errorf("Want 2. Have %s.", have2)
	}

	if have3 != "3" {
		t.Errorf("Want 3. Have %s.", have3)
	}

	if have4 != "4" {
		t.Errorf("Want 4. Have %s.", have4)
	}

	if have5 != nil {
		t.Errorf("Want nil. Have %+v.", have5)
	}
}
