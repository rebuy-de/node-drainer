package testdata

import (
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
)

func GenerateEC2() {
}

const (
	ec2Types = []ec2.Instance{
		ec2.Instance{
			InstanceName:      "master",
			InstanceType:      "t3.large",
			AvailabilityZone:  "eu-west-1a",
			InstanceLifecycle: "",
		},
		ec2.Instance{
			InstanceName:      "worker-spot",
			InstanceType:      "m5.2xlarge",
			AvailabilityZone:  "eu-west-1a",
			InstanceLifecycle: "spot",
		},
		ec2.Instance{
			InstanceName:      "worker-ondemand",
			InstanceType:      "m5.2xlarge",
			AvailabilityZone:  "eu-west-1a",
			InstanceLifecycle: "",
		},
	}

	ec2States = []ec2.Instance{
		ec2.Instance{State: "pending"},
		ec2.Instance{State: "running"},
		ec2.Instance{State: "stopping"},
		ec2.Instance{State: "stopped"},
		ec2.Instance{State: "shutting-down"},
		ec2.Instance{State: "terminated"},
	}
)
