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
		name                        string
		message                     *sqs.ReceiveMessageOutput
		WantDescribeInstancesCalled bool
		WantDeleteMessageCalled     bool
		WantHeartbeatCalled         bool
		WantDrainCalls              int
		Ec2ReturnValue              *ec2.DescribeInstancesOutput
	}{
		{
			name:                        "no_messages",
			message:                     &sqs.ReceiveMessageOutput{},
			WantDescribeInstancesCalled: false,
			WantHeartbeatCalled:         false,
			WantDrainCalls:              0,
			Ec2ReturnValue:              &ec2.DescribeInstancesOutput{},
		},
		{
			name:                        "valid_asg_message",
			message:                     tu.GenerateValidASGMessage(t),
			WantDescribeInstancesCalled: true,
			WantHeartbeatCalled:         true,
			WantDrainCalls:              1,
			Ec2ReturnValue:              tu.GenerateDescribeInstancesOutput(false),
		},
		{
			name:                        "valid_spot_message",
			message:                     tu.GenerateValidSpotMessage(t),
			WantDescribeInstancesCalled: true,
			WantHeartbeatCalled:         false,
			WantDrainCalls:              1,
			Ec2ReturnValue:              tu.GenerateDescribeInstancesOutput(false),
		},
		{
			name:                        "test_message",
			message:                     tu.GenerateTestMessage(t),
			WantDescribeInstancesCalled: false,
			WantHeartbeatCalled:         false,
			WantDrainCalls:              0,
			Ec2ReturnValue:              tu.GenerateDescribeInstancesOutput(true),
		},
		{
			name:                        "invalid_message",
			message:                     tu.GenerateInvalidMessage(t),
			WantDescribeInstancesCalled: false,
			WantHeartbeatCalled:         false,
			WantDrainCalls:              0,
			Ec2ReturnValue:              tu.GenerateDescribeInstancesOutput(true),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests, svcAutoscaling, svcSqs, svcEc2 := tu.GenerateMessageHandlerArgs()
			mh := NewMessageHandler(requests)
			mh.DrainQueue = aws.String("drainQueue")
			mh.SvcAutoscaling = svcAutoscaling
			mh.SvcSQS = svcSqs
			mh.SvcEC2 = svcEc2
			svcEc2.ReturnValue = tc.Ec2ReturnValue
			mh.handleMessage(tc.message)
			if svcEc2.WasDescribeInstancesCalled != tc.WantDescribeInstancesCalled {
				t.Log("WasDescribeInstancesCalled in undesired state: " + strconv.FormatBool(svcEc2.WasDescribeInstancesCalled))
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
	mh := NewMessageHandler(make(chan controller.Request, 10))
	mh.DrainQueue = aws.String("drainQueue")
	mh.SvcAutoscaling = autscalingSvc
	mh.SvcSQS = tu.NewMockSQSClient(false)
	mh.SvcEC2 = tu.NewMockEC2Client(false)

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
	mh := NewMessageHandler(make(chan controller.Request, 10))
	mh.DrainQueue = aws.String("drainQueue")
	mh.SvcAutoscaling = autscalingSvc
	mh.SvcSQS = tu.NewMockSQSClient(false)
	mh.SvcEC2 = tu.NewMockEC2Client(false)

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

func TestDeleteConsumedMessage(t *testing.T) {
	if os.Getenv("TEST_DELETECONSUMEDMESSAGE") == "crash" || os.Getenv("TEST_DELETECONSUMEDMESSAGE") == "nocrash" {
		sqsSvc := tu.NewMockSQSClient(false)
		mh := NewMessageHandler(make(chan controller.Request, 10))
		mh.DrainQueue = aws.String("drainQueue")
		mh.SvcAutoscaling = tu.NewMockAutoScalingClient(false)
		mh.SvcSQS = sqsSvc
		mh.SvcEC2 = tu.NewMockEC2Client(false)

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

func TestResolveNodeName(t *testing.T) {
	cases := []struct {
		Name           string
		InstanceID     *string
		EC2ReturnError bool
		EC2ReturnValue *ec2.DescribeInstancesOutput
		WantNodeName   *string
		WantError      bool
	}{
		{
			Name:         "ResistNilInstance",
			InstanceID:   nil,
			WantNodeName: nil,
			WantError:    true,
		},
		{
			Name:         "ResistEmptyInstance",
			InstanceID:   aws.String(""),
			WantNodeName: nil,
			WantError:    true,
		},
		{
			Name:           "HandleEmptyReservations",
			InstanceID:     aws.String("i-000"),
			WantNodeName:   nil,
			WantError:      false,
			EC2ReturnValue: &ec2.DescribeInstancesOutput{},
		},
		{
			Name:         "HandleEmptyInstances",
			InstanceID:   aws.String("i-000"),
			WantNodeName: nil,
			WantError:    false,
			EC2ReturnValue: &ec2.DescribeInstancesOutput{
				Reservations: []*ec2.Reservation{
					&ec2.Reservation{},
				},
			},
		},
		{
			Name:         "ResistMultipleReservations",
			InstanceID:   aws.String("i-000"),
			WantNodeName: nil,
			WantError:    true,
			EC2ReturnValue: &ec2.DescribeInstancesOutput{
				Reservations: []*ec2.Reservation{
					&ec2.Reservation{},
					&ec2.Reservation{},
				},
			},
		},
		{
			Name:         "ResistMultipleInstances",
			InstanceID:   aws.String("i-000"),
			WantNodeName: nil,
			WantError:    true,
			EC2ReturnValue: &ec2.DescribeInstancesOutput{
				Reservations: []*ec2.Reservation{
					&ec2.Reservation{
						Instances: []*ec2.Instance{
							&ec2.Instance{},
							&ec2.Instance{},
						},
					},
				},
			},
		},
		{
			Name:         "ValidResponse",
			InstanceID:   aws.String("i-000"),
			WantNodeName: aws.String("blub"),
			WantError:    false,
			EC2ReturnValue: &ec2.DescribeInstancesOutput{
				Reservations: []*ec2.Reservation{
					&ec2.Reservation{
						Instances: []*ec2.Instance{
							&ec2.Instance{
								PrivateDnsName: aws.String("blub"),
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ec2Mock := &tu.MockEC2Client{
				ReturnError: tc.EC2ReturnError,
				ReturnValue: tc.EC2ReturnValue,
			}

			mh := &MessageHandler{
				SvcEC2: ec2Mock,
			}

			haveNodeName, haveError := mh.resolveNodeName(tc.InstanceID)

			if haveError == nil && tc.WantError {
				t.Errorf("Expected error.")
			}
			if haveError != nil && !tc.WantError {
				t.Errorf("Got unexpeted error: %v", haveError)
			}

			if haveNodeName != tc.WantNodeName && aws.StringValue(haveNodeName) != aws.StringValue(tc.WantNodeName) {
				t.Errorf("Asserion failed. Have: %s. Want: %s",
					aws.StringValue(haveNodeName), aws.StringValue(tc.WantNodeName))
			}

		})
	}
}
