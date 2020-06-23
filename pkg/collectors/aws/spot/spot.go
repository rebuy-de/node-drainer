// Package spot provides an interface to Spot requests, that are polled from the
// API. It manages a local cache, which is updated periodically.
package spot

import (
	"context"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"

	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/syncutil"
)

// See https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-bid-status.html
const (
	StateActive    = "active"
	StateCancelled = "cancelled"
	StateFailed    = "failed"
	StateClosed    = "closed"
)

const (
	InstanceStateRunning      = "running"
	InstanceStateTerminated   = "terminated"
	InstanceStateShuttingDown = "shutting-down"
)

// Instance is the instance-related data that is retrieved via API.
type Instance struct {
	InstanceID       string    `logfield:"instance-id"`
	RequestID        string    `logfield:"spot-request-id"`
	CreateTime       time.Time `logfield:"spot-create-time"`
	State            string    `logfield:"spot-state"`
	StatusCode       string    `logfield:"spot-status-code"`
	StatusUpdateTime time.Time `logfield:"spot-status-update-time"`
}

// Changed returns true, if relevant fields of the instance changed.
func (i Instance) Changed(old Instance) bool {
	return i.State != old.State ||
		i.StatusCode != old.StatusCode
}

// Client is an interface to Spot request data.
type Client interface {
	// Run executes the EC2 API poller. It will update the instance cache
	// periodically.
	Run(context.Context) error

	// List returns all Spot requests that are currently in the cache. Those
	// instance cache will be updated in the background.
	List() []Instance

	// SignalEmitter gets triggered every time the cache changes. See syncutil
	// package for more information.
	SignalEmitter() *syncutil.SignalEmitter

	// Healthy indicates whether the background job is running correctly.
	Healthy() bool
}

type store struct {
	api     *ec2.EC2
	refresh time.Duration
	cache   map[string]Instance
	emitter *syncutil.SignalEmitter

	failureCount int
}

// New creates a new client for the EC2 API. It needs to be started with Run so
// it actually reads messages. See Client interface for more information.
func New(sess *session.Session, refresh time.Duration) Client {
	return &store{
		api:     ec2.New(sess),
		refresh: refresh,
		emitter: new(syncutil.SignalEmitter),
	}
}

func (s *store) Healthy() bool {
	return s.failureCount == 0
}

func (s *store) SignalEmitter() *syncutil.SignalEmitter {
	return s.emitter
}

func (s *store) List() []Instance {
	result := []Instance{}

	for _, instance := range s.cache {
		result = append(result, instance)
	}

	sort.Slice(result, func(i, j int) bool {
		// Sorting by something other than LaunchTime is required, because the
		// time has only second precision and it is quite likely that some
		// instances are started at the same time. And since the list is based
		// on a map, the order would be flaky.
		return result[i].InstanceID < result[j].InstanceID
	})

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreateTime.Before(result[j].CreateTime)
	})

	return result
}

func (s *store) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		err := s.runOnce(ctx)
		if err != nil {
			logutil.Get(ctx).
				WithError(errors.WithStack(err)).
				Errorf("main loop run failed %d times in a row", s.failureCount)
			s.failureCount++
		} else {
			s.failureCount = 0
		}

		time.Sleep(s.refresh)
	}

	return nil
}

func (s *store) runOnce(ctx context.Context) error {
	ctx = logutil.Start(ctx, "update")

	instances, err := s.fetchInstances(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching instances failed")
	}

	changed := false

	// check whether a new instance was added or an existing was changed
	for _, instance := range instances {
		old, ok := s.cache[instance.InstanceID]
		if !ok {
			logutil.Get(ctx).
				WithFields(logutil.FromStruct(instance)).
				Debugf("add new spot instance to cache")
			changed = true
			continue
		}

		if instance.Changed(old) {
			logutil.Get(ctx).
				WithFields(logutil.FromStruct(instance)).
				Debugf("cached spot instance changed")
			changed = true
			continue
		}
	}

	// check whether an instance was removed
	for _, instance := range s.cache {
		_, ok := instances[instance.InstanceID]
		if !ok {
			logutil.Get(ctx).
				WithFields(logutil.FromStruct(instance)).
				Debugf("cached spot instance was removed")
			changed = true
			continue
		}
	}

	// Replacing the whole map has the advantage that we do not need locking.
	s.cache = instances

	// Emitting a signal AFTER refreshing the cache, if anything changed.
	if changed {
		s.emitter.Emit()
	}

	return nil
}

func (s *store) fetchInstances(ctx context.Context) (map[string]Instance, error) {
	params := &ec2.DescribeSpotInstanceRequestsInput{}
	instances := map[string]Instance{}

	for {
		resp, err := s.api.DescribeSpotInstanceRequests(params)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		for _, dto := range resp.SpotInstanceRequests {
			id := aws.StringValue(dto.InstanceId)

			if id == "" {
				// No idea how this could happend. If it happens anyways,
				// we at least skip the item and log it, so the alerting
				// gets triggered if it happens more often.
				logutil.Get(ctx).WithField("spot-instance-dto", dto).Error("got instance with empty spot instance ID")
				continue
			}

			instance := Instance{
				InstanceID: id,
				RequestID:  aws.StringValue(dto.SpotInstanceRequestId),
				CreateTime: aws.TimeValue(dto.CreateTime),
				State:      aws.StringValue(dto.State),
			}

			if dto.Status != nil {
				instance.StatusCode = aws.StringValue(dto.Status.Code)
				instance.StatusUpdateTime = aws.TimeValue(dto.Status.UpdateTime)
			}

			instances[id] = instance
		}

		if resp.NextToken == nil {
			break
		}

		params = &ec2.DescribeSpotInstanceRequestsInput{
			NextToken: resp.NextToken,
		}
	}

	return instances, nil
}
