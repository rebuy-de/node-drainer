package controller_test

import (
	"testing"

	"github.com/rebuy-de/node-drainer/pkg/controller"
	"github.com/rebuy-de/node-drainer/pkg/controller/fake"
)

func TestQueue(t *testing.T) {
	q := new(controller.Queue)

	q.Add(fake.NewRequest("1", false))
	q.Add(fake.NewRequest("2", false))
	q.Add(fake.NewRequest("3", false))

	have1 := q.Poll().InstanceID()
	have2 := q.Poll().InstanceID()

	q.Add(fake.NewRequest("4", false))

	have3 := q.Poll().InstanceID()
	have4 := q.Poll().InstanceID()
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
