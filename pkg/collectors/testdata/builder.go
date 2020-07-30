package testdata

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/node"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
	v1 "k8s.io/api/core/v1"
)

type EC2State string

const (
	EC2Missing      EC2State = ""
	EC2Pending      EC2State = "pending"
	EC2Running      EC2State = "running"
	EC2ShuttingDown EC2State = "shutting-down"
	EC2Terminated   EC2State = "terminated"
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

type ASGState string

const (
	ASGMissing       ASGState = ""
	ASGPending       ASGState = "pending"
	ASGOnlyCompleted ASGState = "only-completed"
	ASGOnlyDeleted   ASGState = "only-deleted"
	ASGDone          ASGState = "done"
)

type InstanceTemplate struct {
	Name string
	EC2  EC2State
	Spot SpotState
	Node NodeState
	ASG  ASGState
}

type OwnerKind string

const (
	OwnerMissing     OwnerKind = ""
	OwnerDeployment  OwnerKind = "Deployment"
	OwnerStatefulSet OwnerKind = "StatefulSet"
	OwnerNode        OwnerKind = "Node"
	OwnerDaemonSet   OwnerKind = "DaemonSet"
	OwnerReplicaSet  OwnerKind = "ReplicaSet"
	OwnerJob         OwnerKind = "Job"
)

type PodTemplate struct {
	Owner     OwnerKind
	Name      string
	Namespace string

	TotalReplicas   int32
	UnreadyReplicas int32
}

type Builder struct {
	rand              *rand.Rand
	instanceTemplates []InstanceTemplate
	podTemplates      []PodTemplate
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

func (b *Builder) AddInstance(n int, template InstanceTemplate) {
	for i := 0; i < n; i++ {
		b.instanceTemplates = append(b.instanceTemplates, template)
	}
}

func (b *Builder) AddWorkload(template PodTemplate) {
	b.podTemplates = append(b.podTemplates, template)
}

func (b *Builder) Build() collectors.Lists {
	result := collectors.Lists{}
	result = b.buildInstances(result)
	result = b.buildPods(result)
	return result
}

func (b *Builder) buildInstances(result collectors.Lists) collectors.Lists {
	for i, template := range b.instanceTemplates {
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
			asg  asg.Instance
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

			if template.EC2 == EC2ShuttingDown || template.EC2 == EC2Terminated {
				terminationTime := ec2.LaunchTime.Add(time.Hour)
				ec2.TerminationTime = &terminationTime
			}

			if template.EC2 == EC2Running || template.EC2 == EC2ShuttingDown {
				ec2.NodeName = nodeName
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

			if template.Name != "stateful" {
				node.Taints = append(node.Taints, v1.Taint{
					Key:    "rebuy.com/pool",
					Value:  template.Name,
					Effect: "NoSchedule",
				})
			}

			if template.Node == NodeUnschedulable {
				node.Unschedulable = true

				node.Taints = append(node.Taints, v1.Taint{
					Key:    "node.kubernetes.io/unschedulable",
					Effect: "NoSchedule",
				})

			}

			result.Nodes = append(result.Nodes, node)
		}

		if template.ASG != ASGMissing {
			asg.ID = instanceID
			asg.TriggeredAt = ec2.LaunchTime.Add(time.Hour / 2)

			if template.ASG == ASGDone || template.ASG == ASGOnlyCompleted {
				asg.Completed = true
			}

			if template.ASG == ASGDone || template.ASG == ASGOnlyDeleted {
				asg.Deleted = true
			}

			result.ASG = append(result.ASG, asg)

		}

	}

	return result
}

func (b *Builder) buildPods(result collectors.Lists) collectors.Lists {
	for _, template := range b.podTemplates {
		nodeMax := len(result.Nodes)

		specReplicas := template.TotalReplicas
		if specReplicas == math.MaxInt32 {
			specReplicas = int32(nodeMax)
		}

		nodePerm := b.rand.Perm(int(specReplicas))

		for j := int32(0); j < specReplicas; j++ {
			replica := pod.Pod{}

			node := result.Nodes[nodePerm[j]%nodeMax]
			replica.NodeName = node.NodeName

			replica.Namespace = "default"
			if template.Namespace != "" {
				replica.Namespace = template.Namespace
			}

			switch template.Owner {
			case OwnerNode:
				replica.Name = fmt.Sprintf("%s-%s", template.Name, replica.NodeName)
			default:
				replica.Name = fmt.Sprintf("%s-%d", template.Name, j)
			}

			replica.OwnerKind = string(template.Owner)
			if template.Owner != OwnerMissing {
				replica.OwnerName = template.Name
			}

			result.Pods = append(result.Pods, replica)
		}
	}

	return result
}
