package instutil

import (
	"context"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/cmdutil"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/logutil"
)

type contextKeyCounter string
type contextKeyCounterVec string
type contextKeyGauge string
type contextKeyGaugeVec string
type contextKeyHistogram string

var namespace string

func init() {
	re := regexp.MustCompile("[^a-zA-Z0-9]+")
	n := re.ReplaceAllString(cmdutil.Name, "")
	namespace = strings.ToLower(n)
}

func NewCounter(ctx context.Context, name string) context.Context {
	metric := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      name,
	})
	err := prometheus.Register(metric)
	if err != nil {
		logutil.Get(ctx).
			WithError(errors.WithStack(err)).
			Errorf("failed to register counter with name '%s'", name)
	}
	return context.WithValue(ctx, contextKeyCounter(name), metric)
}

func Counter(ctx context.Context, name string) (prometheus.Counter, bool) {
	metric, ok := ctx.Value(contextKeyCounter(name)).(prometheus.Counter)
	if !ok {
		logutil.Get(ctx).Warnf("counter with name '%s' not found", name)
	}
	return metric, ok
}

func NewCounterVec(ctx context.Context, name string, labels ...string) context.Context {
	metric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      name,
	}, labels)
	err := prometheus.Register(metric)
	if err != nil {
		logutil.Get(ctx).
			WithError(errors.WithStack(err)).
			Errorf("failed to register counter vector with name '%s'", name)
	}
	return context.WithValue(ctx, contextKeyCounterVec(name), metric)
}

func CounterVec(ctx context.Context, name string) (*prometheus.CounterVec, bool) {
	metric, ok := ctx.Value(contextKeyCounterVec(name)).(*prometheus.CounterVec)
	if !ok {
		logutil.Get(ctx).Warnf("counter vec with name '%s' not found", name)
	}
	return metric, ok
}

func BucketScale(factor float64, values ...float64) []float64 {
	for i := range values {
		values[i] = values[i] * factor
	}
	return values
}

func NewHistogram(ctx context.Context, name string, buckets ...float64) context.Context {
	metric := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      name,
		Buckets:   buckets,
	})
	err := prometheus.Register(metric)
	if err != nil {
		logutil.Get(ctx).
			WithError(errors.WithStack(err)).
			Errorf("failed to register histogram with name '%s'", name)
	}
	return context.WithValue(ctx, contextKeyHistogram(name), metric)
}

func Histogram(ctx context.Context, name string) (prometheus.Histogram, bool) {
	metric, ok := ctx.Value(contextKeyHistogram(name)).(prometheus.Histogram)
	if !ok {
		logutil.Get(ctx).Warnf("histogram with name '%s' not found", name)
	}
	return metric, ok
}

func NewGauge(ctx context.Context, name string) context.Context {
	metric := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      name,
	})
	err := prometheus.Register(metric)
	if err != nil {
		logutil.Get(ctx).
			WithError(errors.WithStack(err)).
			Errorf("failed to register gauge with name '%s'", name)
	}
	return context.WithValue(ctx, contextKeyGauge(name), metric)
}

func Gauge(ctx context.Context, name string) (prometheus.Gauge, bool) {
	metric, ok := ctx.Value(contextKeyGauge(name)).(prometheus.Gauge)
	if !ok {
		logutil.Get(ctx).Warnf("gauge with name '%s' not found", name)
	}
	return metric, ok
}

func NewGaugeVec(ctx context.Context, name string, labels ...string) context.Context {
	metric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      name,
	}, labels)
	err := prometheus.Register(metric)
	if err != nil {
		logutil.Get(ctx).
			WithError(errors.WithStack(err)).
			Errorf("failed to register gauge vector with name '%s'", name)
	}
	return context.WithValue(ctx, contextKeyGaugeVec(name), metric)
}

func GaugeVec(ctx context.Context, name string) (*prometheus.GaugeVec, bool) {
	metric, ok := ctx.Value(contextKeyGaugeVec(name)).(*prometheus.GaugeVec)
	if !ok {
		logutil.Get(ctx).Warnf("gauge vector with name '%s' not found", name)
	}
	return metric, ok
}
