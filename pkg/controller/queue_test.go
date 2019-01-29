package controller_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rebuy-de/node-drainer/pkg/controller"
)

func TestQueue(t *testing.T) {
	q := controller.NewQueue(prometheus.NewGauge(prometheus.GaugeOpts{}))

	q.Add(controller.Request{NodeName: "1", Fastpath: false})
	q.Add(controller.Request{NodeName: "2", Fastpath: false})
	q.Add(controller.Request{NodeName: "3", Fastpath: false})

	have1 := q.Poll().NodeName
	have2 := q.Poll().NodeName

	q.Add(controller.Request{NodeName: "4", Fastpath: false})

	have3 := q.Poll().NodeName
	have4 := q.Poll().NodeName
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
