package collectors

import (
	"context"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/node"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
)

type Lists struct {
	ASG   []asg.Instance
	EC2   []ec2.Instance
	Spot  []spot.Instance
	Nodes []node.Node
	Pods  []pod.Pod
}

type Collectors struct {
	ASG  asg.Client
	EC2  ec2.Client
	Spot spot.Client
	Node node.Client
	Pod  pod.Client
}

func (c Collectors) List(ctx context.Context) Lists {
	return Lists{
		ASG:   c.ASG.List(),
		EC2:   c.EC2.List(),
		Spot:  c.Spot.List(),
		Nodes: c.Node.List(),
		Pods:  c.Pod.List(ctx),
	}
}
