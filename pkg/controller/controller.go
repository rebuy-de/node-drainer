package controller

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"

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
}

func New(drainer Drainer, requests chan Request) *Controller {
	return &Controller{
		interval: 5 * time.Second,
		cooldown: 10 * time.Minute,

		drainer:  drainer,
		requests: requests,
		clock:    clock.New(),
	}
}

func (c *Controller) Reconcile(ctx context.Context) error {
	ticker := c.clock.Ticker(c.interval)
	defer ticker.Stop()

	backlog := make(Queue, 0)

	for {
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

			logrus.Debugf("draining next node %s from backlog", request.InstanceID())
			go c.Drain(request)

		case request := <-c.requests:
			if !request.UseFastpath() {
				logrus.Debugf("adding node %s to the backlog", request.InstanceID())
				backlog.Add(request)
				continue
			}

			logrus.Debugf("draining node %s using fast-path", request.InstanceID())
			go c.Drain(request)
		}
	}
}

func (c *Controller) Drain(request Request) {
	c.inProgress += 1
	defer func() {
		c.lastDrain = c.clock.Now()
		c.inProgress -= 1
	}()

	id := request.InstanceID()
	err := c.drainer.Drain(id)
	cmdutil.Must(err) // Not sure how to handle such an error properly.

}
