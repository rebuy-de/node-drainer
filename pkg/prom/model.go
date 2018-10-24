package prom

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	EvictedPods          prometheus.Counter
	LastEvictionDuration prometheus.Gauge
}

var M = Metrics{
	EvictedPods: prometheus.NewCounter(
		prometheus.CounterOpts{
			ConstLabels: prometheus.Labels{},
			Namespace:   "rebuy",
			Subsystem:   "node_drainer",
			Name:        "evicted_pods",
			Help:        "Number of evicted Pods on total.",
		},
	),
	LastEvictionDuration: prometheus.NewGauge(
		prometheus.GaugeOpts{
			ConstLabels: prometheus.Labels{},
			Namespace:   "rebuy",
			Subsystem:   "node_drainer",
			Name:        "last_eviction_duration",
			Help:        "Duration in seconds of the last eviction.",
		},
	),
}

func (m *Metrics) IncreaseEvictedPods() {
	m.EvictedPods.Add(1)
}

func (m *Metrics) SetLastEvictionDuration(duration float64) {
	m.LastEvictionDuration.Set(float64(duration))
}
