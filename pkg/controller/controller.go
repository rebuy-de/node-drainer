package controller

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/rebuy-de/node-drainer/pkg/drainer"
	"github.com/rebuy-de/rebuy-go-sdk/cmdutil"
	"github.com/sirupsen/logrus"
)

type Drainer interface {
	Drain(string) error
}

type Controller struct {
	interval time.Duration
	cooldown time.Duration

	drainer  Drainer
	requests chan Request
	clock    clock.Clock

	lastDrain  time.Time
	inProgress int

	metricDraining     prometheus.Gauge
	metricBacklogSize  prometheus.Gauge
	metricLastActivity *prometheus.GaugeVec
}

func New(drainer Drainer, requests chan Request) *Controller {
	return &Controller{
		interval: 5 * time.Second,
		cooldown: 10 * time.Minute,

		drainer:  drainer,
		requests: requests,
		clock:    clock.New(),

		metricDraining: prometheus.NewGauge(
			prometheus.GaugeOpts{
				ConstLabels: prometheus.Labels{},
				Namespace:   "rebuy",
				Subsystem:   "node_drainer",
				Name:        "draining",
				Help:        "Number of drains that are in progress",
			},
		),
		metricBacklogSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				ConstLabels: prometheus.Labels{},
				Namespace:   "rebuy",
				Subsystem:   "node_drainer",
				Name:        "backlog_size",
				Help:        "Number of non-fastpath drain requests in backlog",
			},
		),
		metricLastActivity: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				ConstLabels: prometheus.Labels{},
				Namespace:   "rebuy",
				Subsystem:   "node_drainer",
				Name:        "last_activity",
				Help:        "Timestamp of last activity",
			},
			[]string{"activity"},
		),
	}
}

func (c *Controller) RegisterMetrics(r *prometheus.Registry) {
	r.MustRegister(c.metricDraining, c.metricBacklogSize, c.metricLastActivity)
}

func (c *Controller) Reconcile(ctx context.Context) error {
	ticker := c.clock.Ticker(c.interval)
	defer ticker.Stop()

	backlog := NewQueue(c.metricBacklogSize)

	for {
		c.metricLastActivity.WithLabelValues("main").SetToCurrentTime()

		select {
		case <-ctx.Done():
			// Note: Requests from the backlog should be discardable, since the
			// messages are persisted in SQS. All other requests are using
			// fast-path and should be finished when reaching the next loop.
			logrus.Info("gracefully exiting main loop")
			return nil

		case <-ticker.C:
			logrus.Debug("checking backlog")

			progress := c.inProgress // copying variable to make sure the log message is consistent
			if progress > 0 {
				logrus.Debugf("skip processing backlog, because there are still %d drains in progress", progress)
				continue
			}

			age := c.clock.Since(c.lastDrain)
			if age < c.cooldown {
				logrus.Debugf("skip processing backlog, because last drain was just %v ago", age)
				continue
			}

			request := backlog.Poll()
			if request == nil {
				logrus.Debug("backlog is empty")
				continue
			}

			logrus.Infof("draining next node %s from backlog", request.InstanceID)
			c.metricLastActivity.WithLabelValues("drain-backlog").SetToCurrentTime()
			go c.Drain(*request)

		case request := <-c.requests:
			if !request.Fastpath {
				logrus.Infof("adding node %s to the backlog", request.InstanceID)
				backlog.Add(request)
				continue
			}

			logrus.Infof("draining node %s using fast-path", request.InstanceID)
			c.metricLastActivity.WithLabelValues("drain-fastpath").SetToCurrentTime()
			go c.Drain(request)
		}
	}
}

func (c *Controller) Drain(request Request) {
	c.inProgress += 1
	c.metricDraining.Set(float64(c.inProgress))
	defer func() {
		c.lastDrain = c.clock.Now()
		c.inProgress -= 1
		c.metricDraining.Set(float64(c.inProgress))
	}()

	err := c.drainer.Drain(request.InstanceID)
	if err != nil && !drainer.IsErrNodeNotAvailable(err) {
		// Unexpected error. Better let us die and try again.
		logrus.Errorf("%+v", err)
		cmdutil.Exit(1)
	}

	if request.OnDone != nil {
		request.OnDone()
	}
}
