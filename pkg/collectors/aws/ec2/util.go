package ec2

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func ec2tag(instance *ec2.Instance, key string) string {
	for _, tag := range instance.Tags {
		if aws.StringValue(tag.Key) == key {
			return aws.StringValue(tag.Value)
		}
	}

	return ""
}
