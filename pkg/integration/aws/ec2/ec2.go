package ec2

import (
	"context"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"

	"github.com/rebuy-de/node-drainer/v2/pkg/syncutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
)

type Instance struct {
	InstanceID           string
	InstanceName         string
	HostName             string
	InstanceType         string
	AutoScalingGroupName string
	AvailabilityZone     string
	InstanceLifecycle    string
	State                string
	LaunchTime           time.Time
	TerminationTime      *time.Time
}

type Store struct {
	api     *ec2.EC2
	refresh time.Duration
	cache   map[string]Instance
	emitter *syncutil.SignalEmitter
}

func New(sess *session.Session, refresh time.Duration) *Store {
	return &Store{
		api:     ec2.New(sess),
		refresh: refresh,
		emitter: new(syncutil.SignalEmitter),
	}
}

func (s *Store) SignalEmitter() *syncutil.SignalEmitter {
	return s.emitter
}

func (s *Store) List() []Instance {
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
		return result[i].LaunchTime.Before(result[j].LaunchTime)
	})

	return result
}

func (s *Store) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		err := s.runOnce(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		time.Sleep(s.refresh)
	}

	return nil
}

func (s *Store) runOnce(ctx context.Context) error {
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
				WithFields(logFieldsFromStruct(instance)).
				Debugf("add new instance to cache")
			changed = true
			continue
		}

		if old != instance {
			logutil.Get(ctx).
				WithFields(logFieldsFromStruct(instance)).
				Debugf("cached instance changed")
			changed = true
			continue
		}
	}

	// check whether an instance was removed
	for _, instance := range s.cache {
		_, ok := instances[instance.InstanceID]
		if !ok {
			logutil.Get(ctx).
				WithFields(logFieldsFromStruct(instance)).
				Debugf("cached instance was removed")
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

func (s *Store) fetchInstances(ctx context.Context) (map[string]Instance, error) {
	params := &ec2.DescribeInstancesInput{}
	instances := map[string]Instance{}

	for {
		resp, err := s.api.DescribeInstances(params)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		for _, reservation := range resp.Reservations {
			for _, dto := range reservation.Instances {
				id := aws.StringValue(dto.InstanceId)

				if id == "" {
					// No idea how this could happend. If it happens anyways,
					// we at least skip the item and log it, so the alerting
					// gets triggered if it happens more often.
					logutil.Get(ctx).WithField("instance-dto", dto).Error("got instance with empty instance ID")
					continue
				}

				instance := Instance{
					InstanceID:        id,
					HostName:          aws.StringValue(dto.PrivateDnsName),
					State:             aws.StringValue(dto.State.Name),
					InstanceType:      aws.StringValue(dto.InstanceType),
					InstanceName:      ec2tag(dto, "Name"),
					AvailabilityZone:  aws.StringValue(dto.Placement.AvailabilityZone),
					InstanceLifecycle: aws.StringValue(dto.InstanceLifecycle),
					LaunchTime:        aws.TimeValue(dto.LaunchTime),
				}

				if instance.State == InstanceStateTerminated || instance.State == InstanceStateShuttingDown {
					// Parsing the termination date from the
					// StateTransitionReason is not very reliable, since it is
					// not standarized and we do tolarate other reasons. This
					// is fine, since we use it only for displaying purposes.
					// If we need a reliable value, we would need to get it
					// from CloudTrail.
					terminationTime, err := time.Parse("User initiated (2006-01-02 15:04:05 MST)", aws.StringValue(dto.StateTransitionReason))
					if err != nil {
						logutil.Get(ctx).
							WithField("state-transition-reason", dto.StateTransitionReason).
							WithError(errors.WithStack(err)).
							Warn("failed to parse state transition reason")
					} else {
						instance.TerminationTime = &terminationTime
					}
				}

				instances[id] = instance
			}
		}

		if resp.NextToken == nil {
			break
		}

		params = &ec2.DescribeInstancesInput{
			NextToken: resp.NextToken,
		}
	}

	return instances, nil
}