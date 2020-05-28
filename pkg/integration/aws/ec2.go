package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"

	"github.com/rebuy-de/node-drainer/v2/pkg/syncutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
)

type EC2Instance struct {
	InstanceID           string
	NodeName             string
	InstanceType         string
	AutoScalingGroupName string
	AvailabilityZone     string
	InstanceLifecycle    string
	State                string
}

type EC2Client struct {
	api     *ec2.EC2
	refresh time.Duration
	cache   map[string]EC2Instance
	emitter *syncutil.SignalEmitter
}

func NewEC2Client(sess *session.Session, refresh time.Duration) *EC2Client {
	return &EC2Client{
		api:     ec2.New(sess),
		refresh: refresh,
		emitter: new(syncutil.SignalEmitter),
	}
}

func (c *EC2Client) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		err := c.runOnce(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		time.Sleep(c.refresh)
	}

	return nil
}

func (c *EC2Client) runOnce(ctx context.Context) error {
	ctx = logutil.Start(ctx, "update")

	instances, err := c.fetchInstances(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching instances failed")
	}

	changed := false

	// check whether a new instance was added or an existing was changed
	for _, instance := range instances {
		old, ok := c.cache[instance.InstanceID]
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
	for _, instance := range c.cache {
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
	c.cache = instances

	// Emitting a signal AFTER refreshing the cache, if anything changed.
	if changed {
		c.emitter.Emit()
	}

	return nil
}

func (c *EC2Client) fetchInstances(ctx context.Context) (map[string]EC2Instance, error) {
	logutil.Get(ctx).Debug("fetching instances")

	params := &ec2.DescribeInstancesInput{}
	instances := map[string]EC2Instance{}

	for {
		resp, err := c.api.DescribeInstances(params)
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

				instances[id] = EC2Instance{
					InstanceID:           id,
					NodeName:             aws.StringValue(dto.PrivateDnsName),
					State:                aws.StringValue(dto.State.Name),
					InstanceType:         aws.StringValue(dto.InstanceType),
					AutoScalingGroupName: ec2tag(dto, "aws:autoscaling:groupName"),
					AvailabilityZone:     aws.StringValue(dto.Placement.AvailabilityZone),
					InstanceLifecycle:    aws.StringValue(dto.InstanceLifecycle),
				}
			}
		}

		if resp.NextToken == nil {
			break
		}

		params = &ec2.DescribeInstancesInput{
			NextToken: resp.NextToken,
		}
	}

	logutil.Get(ctx).Debug("fetching instances succeeded")
	return instances, nil
}
