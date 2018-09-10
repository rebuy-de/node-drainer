package test_util

import (
	"errors"

	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

type MockAutoScalingClient struct {
	autoscalingiface.AutoScalingAPI
	WasCompleteLifecycleActionCalled bool
	WasHeartbeatCalled               bool
	ReturnError                      bool
}

func NewMockAutoScalingClient(returnError bool) *MockAutoScalingClient {
	m := &MockAutoScalingClient{}
	m.WasCompleteLifecycleActionCalled = false
	m.WasHeartbeatCalled = false
	m.ReturnError = returnError
	return m
}

func (m *MockAutoScalingClient) CompleteLifecycleAction(input *autoscaling.CompleteLifecycleActionInput) (*autoscaling.CompleteLifecycleActionOutput, error) {
	m.WasCompleteLifecycleActionCalled = true
	if m.ReturnError {
		return nil, errors.New("CompleteLifecycleAction mock results in error")
	}
	return &autoscaling.CompleteLifecycleActionOutput{}, nil
}

func (m *MockAutoScalingClient) RecordLifecycleActionHeartbeat(input *autoscaling.RecordLifecycleActionHeartbeatInput) (*autoscaling.RecordLifecycleActionHeartbeatOutput, error) {
	m.WasHeartbeatCalled = true
	if m.ReturnError {
		return nil, errors.New("RecordLifecycleActionHeartbeatOutput mock results in error")
	}
	return &autoscaling.RecordLifecycleActionHeartbeatOutput{}, nil
}

type MockSQSClient struct {
	sqsiface.SQSAPI
	WasDeleteMessageCalled bool
	ReturnError            bool
}

func NewMockSQSClient(returnError bool) *MockSQSClient {
	m := &MockSQSClient{}
	m.WasDeleteMessageCalled = false
	m.ReturnError = returnError
	return m
}

func (m *MockSQSClient) DeleteMessage(*sqs.DeleteMessageInput) (*sqs.DeleteMessageOutput, error) {
	m.WasDeleteMessageCalled = true
	if m.ReturnError {
		return nil, errors.New("DeleteMessage mock results in error")
	}
	return &sqs.DeleteMessageOutput{}, nil
}

type MockEC2Client struct {
	ec2iface.EC2API
	WasDescribeInstancesCalled bool
	ReturnError                bool
	ReturnValue                *ec2.DescribeInstancesOutput
}

func NewMockEC2Client(returnError bool) *MockEC2Client {
	m := &MockEC2Client{}
	m.WasDescribeInstancesCalled = false
	m.ReturnError = returnError
	m.ReturnValue = &ec2.DescribeInstancesOutput{}
	return m
}

func (m *MockEC2Client) DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	m.WasDescribeInstancesCalled = true
	if m.ReturnError {
		return &ec2.DescribeInstancesOutput{}, errors.New("DescribeInstances mock results in errorr")
	}
	return m.ReturnValue, nil
}
