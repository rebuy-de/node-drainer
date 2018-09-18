package test_util

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/rebuy-de/node-drainer/pkg/util"
)

func GenerateDescribeInstancesOutput(empty bool) *ec2.DescribeInstancesOutput {
	output := &ec2.DescribeInstancesOutput{}
	reservation := &ec2.Reservation{}
	output.Reservations = []*ec2.Reservation{
		reservation,
	}
	instances := []*ec2.Instance{}
	if !empty {
		instances = append(instances, &ec2.Instance{PrivateDnsName: aws.String("instance")})
	}
	reservation.SetInstances(instances)
	return output
}

func GenerateMessageHandlerArgs() (*MockDrainer, *MockAutoScalingClient, *MockSQSClient, *MockEC2Client) {
	return NewMockDrainer(), NewMockAutoScalingClient(false), NewMockSQSClient(false), NewMockEC2Client(false)
}

func GenerateSqsMessageHandle() string {
	return "foobar"
}

func GenerateValidMessage(t *testing.T) *sqs.ReceiveMessageOutput {
	output := sqs.ReceiveMessageOutput{}
	msg := sqs.Message{}
	msgList := []*sqs.Message{&msg}
	msg.SetBody("{\"LifecycleHookName\":\"lifecycle-hook-name\",\"AccountId\":\"000000000000\",\"RequestId\":\"00000000-0000-0000-0000-00000000000000\",\"LifecycleTransition\":\"autoscaling:EC2_INSTANCE_TERMINATING\",\"AutoScalingGroupName\":\"autoscaling-group-name\",\"Service\":\"AWS Auto Scaling\",\"Time\":\"2000-01-01T00:00:00.000Z\",\"EC2InstanceId\":\"i-00000000000000000\",\"LifecycleActionToken\":\"00000000-0000-0000-0000-00000000000000\"}")
	msg.SetReceiptHandle(GenerateSqsMessageHandle())
	output.SetMessages(msgList)
	return &output
}

func GenerateTestMessage(t *testing.T) *sqs.ReceiveMessageOutput {
	output := sqs.ReceiveMessageOutput{}
	msg := sqs.Message{}
	msgList := []*sqs.Message{&msg}
	msg.SetBody("{\"AccountId\":\"000000000000\",\"RequestId\":\"00000000-0000-0000-0000-000000000000\",\"AutoScalingGroupARN\":\"arn:aws:autoscaling:eu-west-1:000000000000:autoScalingGroup:00000000-0000-0000-0000-000000000000:autoScalingGroupName/autoscaling-group-name\",\"AutoScalingGroupName\":\"autoscaling-group-name\",\"Service\":\"AWS Auto Scaling\",\"Event\":\"autoscaling:TEST_NOTIFICATION\",\"Time\":\"2000-01-01T00:00:00.000Z\"}")
	msg.SetReceiptHandle(GenerateSqsMessageHandle())
	output.SetMessages(msgList)
	return &output
}

func GenerateInvalidMessage(t *testing.T) *sqs.ReceiveMessageOutput {
	output := sqs.ReceiveMessageOutput{}
	msg := sqs.Message{}
	msgList := []*sqs.Message{&msg}
	msg.SetBody("{\"LifecycleHookName\"eActionToken\":\"00000000-0000-0000-0000-00000000000000\"{}")
	msg.SetReceiptHandle(GenerateSqsMessageHandle())
	output.SetMessages(msgList)
	return &output
}

func GeneratePlainMessage() util.Message {
	return util.Message{
		LifecycleHookName:    aws.String(""),
		AccountId:            aws.String(""),
		RequestId:            aws.String(""),
		LifecycleTransition:  aws.String(""),
		AutoScalingGroupName: aws.String(""),
		Service:              aws.String(""),
		Time:                 aws.String(""),
		EC2InstanceId:        aws.String(""),
		LifecycleActionToken: aws.String(""),
	}
}
