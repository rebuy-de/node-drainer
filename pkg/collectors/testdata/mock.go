package testdata

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/syncutil"
	"github.com/stretchr/testify/mock"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/node"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
)

func GenerateCollectors(t *testing.T, lists collectors.Lists) collectors.Collectors {
	return collectors.Collectors{
		ASG: &ASGClientMock{
			instances: lists.ASG,
		},
		EC2: &EC2ClientMock{
			instances: lists.EC2,
		},
		Spot: &SpotClientMock{
			instances: lists.Spot,
		},
		Node: &NodeClientMock{
			nodes: lists.Nodes,
		},
		Pod: &PodClientMock{
			pods: lists.Pods,
		},
	}
}

func errNotImplemented() error {
	return errors.Errorf("this function is not implemented in this mock")
}

type ASGClientMock struct {
	mock.Mock
	instances []asg.Instance
}

func (c *ASGClientMock) Complete(ctx context.Context, id string) error {
	args := c.Called(ctx, id)
	return args.Error(0)
}

func (c *ASGClientMock) Delete(ctx context.Context, id string) error {
	args := c.Called(ctx, id)
	return args.Error(0)
}

type EC2ClientMock struct{ instances []ec2.Instance }
type SpotClientMock struct{ instances []spot.Instance }
type NodeClientMock struct{ nodes []node.Node }
type PodClientMock struct{ pods []pod.Pod }

func (c *ASGClientMock) List() []asg.Instance               { return c.instances }
func (c *EC2ClientMock) List() []ec2.Instance               { return c.instances }
func (c *SpotClientMock) List() []spot.Instance             { return c.instances }
func (c *NodeClientMock) List() []node.Node                 { return c.nodes }
func (c *PodClientMock) List(ctx context.Context) []pod.Pod { return c.pods }

func (c *ASGClientMock) Healthy() bool  { return true }
func (c *EC2ClientMock) Healthy() bool  { return true }
func (c *SpotClientMock) Healthy() bool { return true }
func (c *NodeClientMock) Healthy() bool { return true }
func (c *PodClientMock) Healthy() bool  { return true }

func (c *ASGClientMock) SignalEmitter() *syncutil.SignalEmitter  { return new(syncutil.SignalEmitter) }
func (c *EC2ClientMock) SignalEmitter() *syncutil.SignalEmitter  { return new(syncutil.SignalEmitter) }
func (c *SpotClientMock) SignalEmitter() *syncutil.SignalEmitter { return new(syncutil.SignalEmitter) }
func (c *NodeClientMock) SignalEmitter() *syncutil.SignalEmitter { return new(syncutil.SignalEmitter) }
func (c *PodClientMock) SignalEmitter() *syncutil.SignalEmitter  { return new(syncutil.SignalEmitter) }

func (c *ASGClientMock) Run(ctx context.Context) error  { return errNotImplemented() }
func (c *EC2ClientMock) Run(ctx context.Context) error  { return errNotImplemented() }
func (c *SpotClientMock) Run(ctx context.Context) error { return errNotImplemented() }
func (c *NodeClientMock) Run(ctx context.Context) error { return errNotImplemented() }
func (c *PodClientMock) Run(ctx context.Context) error  { return errNotImplemented() }
