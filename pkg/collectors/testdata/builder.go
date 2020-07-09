package testdata

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/node"
	v1 "k8s.io/api/core/v1"
)

type EC2State string

const (
	EC2Missing    EC2State = ""
	EC2Running    EC2State = "running"
	EC2Terminated EC2State = "terminated"
)

type SpotState string

const (
	SpotMissing          SpotState = ""
	SpotRunning          SpotState = "running"
	SpotTerminatedByUser SpotState = "terminated-by-user"
)

type NodeState string

const (
	NodeMissing       NodeState = ""
	NodeSchedulable   NodeState = "schedulable"
	NodeUnschedulable NodeState = "unschedulable"
)

type Template struct {
	Name string
	EC2  EC2State
	Spot SpotState
	Node NodeState
}

type Builder struct {
	rand      *rand.Rand
	templates []Template
}

func NewBuilder() *Builder {
	return &Builder{
		rand: rand.New(rand.NewSource(1)),
	}
}

func (b *Builder) randOfStrings(values ...string) string {
	i := b.rand.Int() % len(values)
	return values[i]
}

func (b *Builder) randTime() time.Time {
	return time.
		Date(2020, time.July, 6, 16, 19, 0, 0, time.UTC).
		Add(time.Second * time.Duration(b.rand.Uint32()%604800))
}

func (b *Builder) Add(n int, template Template) {
	for i := 0; i < n; i++ {
		b.templates = append(b.templates, template)
	}
}

func (b *Builder) Build() collectors.Lists {
	result := collectors.Lists{}

	for i, template := range b.templates {
		// InstanceID consisting of two parts a random one and the actual order
		// number. This is just to make the IDs look more real and "unsorted"
		// while still being able to identify them.
		instanceID := fmt.Sprintf("i-%08x0%08d", b.rand.Uint32(), i+1)

		// Same idea as with the Instance ID.
		nodeName := fmt.Sprintf(
			"ip-10-%d-%d-%d.eu-west-1.compute.internal",
			b.rand.Uint32()%0xff, b.rand.Uint32()%0xff, i+1,
		)

		var (
			ec2  ec2.Instance
			spot spot.Instance
			node node.Node
		)

		if template.EC2 != EC2Missing {
			ec2.InstanceID = instanceID
			ec2.NodeName = nodeName
			ec2.InstanceName = template.Name
			ec2.State = string(template.EC2)
			ec2.LaunchTime = b.randTime()

			ec2.AvailabilityZone = b.randOfStrings(
				"eu-west-1a",
				"eu-west-1b",
				"eu-west-1c",
			)
			ec2.InstanceType = b.randOfStrings(
				"m4.2xlarge",
				"m5.2xlarge",
			)

			// Same idea as with the Instance ID.
			if template.Spot != SpotMissing {
				ec2.InstanceLifecycle = "spot"
			}

			switch template.EC2 {
			case EC2Terminated:
				terminationTime := ec2.LaunchTime.Add(time.Hour)
				ec2.TerminationTime = &terminationTime
			}

			result.EC2 = append(result.EC2, ec2)
		}

		if template.Spot != SpotMissing {
			spot.InstanceID = instanceID

			spot.RequestID = fmt.Sprintf("sir-%08x", b.rand.Uint32())
			spot.CreateTime = ec2.LaunchTime
			spot.StatusUpdateTime = ec2.LaunchTime

			switch template.Spot {
			case SpotRunning:
				spot.State = "active"
				spot.StatusCode = "fulfilled"
			case SpotTerminatedByUser:
				spot.State = "closed"
				spot.StatusCode = "instance-terminated-by-user"

			}

			result.Spot = append(result.Spot, spot)

		}

		if template.Node != NodeMissing {
			node.InstanceID = instanceID
			node.NodeName = nodeName

			if template.Node == NodeUnschedulable {
				node.Unschedulable = true

				node.Taints = append(node.Taints, v1.Taint{
					Key:    "node.kubernetes.io/unschedulable",
					Effect: "NoSchedule",
				})

			}

			result.Nodes = append(result.Nodes, node)
		}

	}

	return result
}
