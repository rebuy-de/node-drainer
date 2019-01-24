package sqs

import (
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/rebuy-de/node-drainer/pkg/controller"
	tu "github.com/rebuy-de/node-drainer/pkg/sqs/test_util"
)

func TestHandleMessage(t *testing.T) {
	cases := []struct {
		name                              string
		message                           *sqs.ReceiveMessageOutput
		WantDescribeInstancesCalled       bool
		WantDeleteMessageCalled           bool
		WantCompleteLifecycleActionCalled bool
		WantHeartbeatCalled               bool
		WantDrainCalls                    int
		Ec2ReturnValue                    *ec2.DescribeInstancesOutput
	}{
		{
			name:                              "no_messages",
			message:                           &sqs.ReceiveMessageOutput{},
			WantDescribeInstancesCalled:       false,
			WantDeleteMessageCalled:           false,
			WantCompleteLifecycleActionCalled: false,
			WantHeartbeatCalled:               false,
			WantDrainCalls:                    0,
			Ec2ReturnValue:                    &ec2.DescribeInstancesOutput{},
		},
		{
			name:                              "valid_asg_message",
			message:                           tu.GenerateValidASGMessage(t),
			WantDescribeInstancesCalled:       true,
			WantDeleteMessageCalled:           true,
			WantCompleteLifecycleActionCalled: true,
			WantHeartbeatCalled:               true,
			WantDrainCalls:                    1,
			Ec2ReturnValue:                    tu.GenerateDescribeInstancesOutput(false),
		},
		{
			name:                              "valid_spot_message",
			message:                           tu.GenerateValidSpotMessage(t),
			WantDescribeInstancesCalled:       true,
			WantDeleteMessageCalled:           true,
			WantCompleteLifecycleActionCalled: false,
			WantHeartbeatCalled:               false,
			WantDrainCalls:                    1,
			Ec2ReturnValue:                    tu.GenerateDescribeInstancesOutput(false),
		},
		{
			name:                              "test_message",
			message:                           tu.GenerateTestMessage(t),
			WantDescribeInstancesCalled:       false,
			WantDeleteMessageCalled:           true,
			WantCompleteLifecycleActionCalled: false,
			WantHeartbeatCalled:               false,
			WantDrainCalls:                    0,
			Ec2ReturnValue:                    tu.GenerateDescribeInstancesOutput(true),
		},
		{
			name:                              "invalid_message",
			message:                           tu.GenerateInvalidMessage(t),
			WantDescribeInstancesCalled:       false,
			WantDeleteMessageCalled:           true,
			WantCompleteLifecycleActionCalled: false,
			WantHeartbeatCalled:               false,
			WantDrainCalls:                    0,
			Ec2ReturnValue:                    tu.GenerateDescribeInstancesOutput(true),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests, svcAutoscaling, svcSqs, svcEc2 := tu.GenerateMessageHandlerArgs()
			mh := NewMessageHandler(requests, aws.String("drainQueue"), 10, svcAutoscaling, svcSqs, svcEc2, 0)
			svcEc2.ReturnValue = tc.Ec2ReturnValue
			mh.handleMessage(tc.message)
			if svcEc2.WasDescribeInstancesCalled != tc.WantDescribeInstancesCalled {
				t.Log("WasDescribeInstancesCalled in undesired state: " + strconv.FormatBool(svcEc2.WasDescribeInstancesCalled))
				t.Fail()
			}
			if svcSqs.WasDeleteMessageCalled != tc.WantDeleteMessageCalled {
				t.Log("WasDeleteMessageCalled in undesired state: " + strconv.FormatBool(svcSqs.WasDeleteMessageCalled))
				t.Fail()
			}
			if svcAutoscaling.WasCompleteLifecycleActionCalled != tc.WantCompleteLifecycleActionCalled {
				t.Log("WasCompleteLifecycleActionCalled in undesired state: " + strconv.FormatBool(svcAutoscaling.WasCompleteLifecycleActionCalled))
				t.Fail()
			}
			if svcAutoscaling.WasHeartbeatCalled != tc.WantHeartbeatCalled {
				t.Log("WasHeartbeatCalled in undesired state: " + strconv.FormatBool(svcAutoscaling.WasHeartbeatCalled))
				t.Fail()
			}
			if len(requests) != tc.WantDrainCalls {
				t.Logf("WasDrainCalls in undesired state: %d", len(requests))
				t.Fail()
			}
		})
	}
}

func TestNotifyASG(t *testing.T) {
	message := tu.GeneratePlainMessage()
	autscalingSvc := tu.NewMockAutoScalingClient(false)
	mh := NewMessageHandler(make(chan controller.Request, 10), aws.String("drainQueue"), 10, autscalingSvc, tu.NewMockSQSClient(false), tu.NewMockEC2Client(false), 0)

	cases := []struct {
		name      string
		callFails bool
		want      bool
	}{
		{
			name:      "asg_notification_succeeds",
			callFails: false,
			want:      true,
		},
		{
			name:      "asg_notification_fails",
			callFails: true,
			want:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			autscalingSvc.ReturnError = tc.callFails
			mh.notifyASG(&message)
			have := autscalingSvc.WasCompleteLifecycleActionCalled
			if have != tc.want {
				t.Fail()
			}
		})
	}
}

func TestHeartbeat(t *testing.T) {
	message := tu.GeneratePlainMessage()
	autscalingSvc := tu.NewMockAutoScalingClient(false)
	mh := NewMessageHandler(make(chan controller.Request, 10), aws.String("drainQueue"), 10, autscalingSvc, tu.NewMockSQSClient(false), tu.NewMockEC2Client(false), 0)

	cases := []struct {
		name      string
		callFails bool
		want      bool
	}{
		{
			name:      "heartbeat_succeeds",
			callFails: false,
			want:      true,
		},
		{
			name:      "heartbeat_fails",
			callFails: true,
			want:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			autscalingSvc.ReturnError = tc.callFails
			mh.heartbeat(&message)
			have := autscalingSvc.WasHeartbeatCalled
			if have != tc.want {
				t.Fail()
			}
		})
	}
}

func TestTriggerDrain(t *testing.T) {
	message := tu.GeneratePlainMessage()
	ec2Svc := tu.NewMockEC2Client(false)
	mh := NewMessageHandler(make(chan controller.Request, 10), aws.String("drainQueue"), 10, tu.NewMockAutoScalingClient(false), tu.NewMockSQSClient(false), ec2Svc, 0)

	cases := []struct {
		name      string
		callFails bool
		want      bool
	}{
		{
			name:      "drain_succeeds",
			callFails: false,
			want:      true,
		},
		{
			name:      "drain_fails",
			callFails: true,
			want:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ec2Svc.ReturnError = tc.callFails
			mh.triggerDrain(message.EC2InstanceId, true)
			have := ec2Svc.WasDescribeInstancesCalled
			if have != tc.want {
				t.Fail()
			}
		})
	}
}
func TestTriggerDrainCount(t *testing.T) {
	message := tu.GeneratePlainMessage()
	ec2Svc := tu.NewMockEC2Client(false)
	md := make(chan controller.Request, 10)
	mh := NewMessageHandler(md, aws.String("drainQueue"), 10, tu.NewMockAutoScalingClient(false), tu.NewMockSQSClient(false), ec2Svc, 0)
	cases := []struct {
		name    string
		ec2Conf *ec2.DescribeInstancesOutput
		want    []string
	}{
		{
			name:    "drain_nothing",
			ec2Conf: tu.GenerateDescribeInstancesOutput(false),
			want:    []string{},
		},
		{
			name:    "drain_node",
			ec2Conf: tu.GenerateDescribeInstancesOutput(true),
			want:    []string{"instance"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mh.triggerDrain(message.EC2InstanceId, true)
			ec2Svc.ReturnValue = tc.ec2Conf
			if len(md) != len(tc.want) {
				t.Fail()
			}
		})
	}
}

func TestDeleteConsumedMessage(t *testing.T) {
	if os.Getenv("TEST_DELETECONSUMEDMESSAGE") == "crash" || os.Getenv("TEST_DELETECONSUMEDMESSAGE") == "nocrash" {
		sqsSvc := tu.NewMockSQSClient(false)
		mh := NewMessageHandler(make(chan controller.Request, 10), aws.String("drainQueue"), 10, tu.NewMockAutoScalingClient(false), sqsSvc, tu.NewMockEC2Client(false), 0)
		if os.Getenv("TEST_DELETECONSUMEDMESSAGE") == "crash" {
			sqsSvc.ReturnError = true
			mh.deleteConsumedMessage(aws.String(""))
			return
		} else if os.Getenv("TEST_DELETECONSUMEDMESSAGE") == "nocrash" {
			sqsSvc.ReturnError = false
			mh.deleteConsumedMessage(aws.String(""))
			return
		}
	}

	cases := []struct {
		name       string
		causeCrash string
		want       bool
	}{
		{
			name:       "message_deletion_fails",
			causeCrash: "crash",
			want:       false,
		},
		{
			name:       "message_deletion_succeeds",
			causeCrash: "nocrash",
			want:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestDeleteConsumedMessage")
			cmd.Env = append(os.Environ(), "TEST_DELETECONSUMEDMESSAGE="+tc.causeCrash)
			err := cmd.Run()
			var have bool
			if err == nil {
				have = true
			} else {
				have = false
			}
			if have != tc.want {
				t.Fail()
			}
		})
	}
}
