package controller

import "github.com/prometheus/client_golang/prometheus"

type Queue struct {
	requests []Request
	gauge    prometheus.Gauge
}

func NewQueue(gauge prometheus.Gauge) *Queue {
	return &Queue{
		requests: make([]Request, 0),
		gauge:    gauge,
	}
}

// Add inserts the specified element into this queue.
func (q *Queue) Add(r Request) {
	defer q.refreshMetrics()

	q.requests = append(q.requests, r)
}

// Poll retrieves, but does not remove, the head of this queue, or returns null
// if this queue is empty.
func (q *Queue) Poll() *Request {
	defer q.refreshMetrics()

	if len(q.requests) < 1 {
		return nil
	}

	r := q.requests[0]
	q.requests = q.requests[1:]

	return &r
}

func (q *Queue) refreshMetrics() {
	q.gauge.Set(float64(len(q.requests)))
}
