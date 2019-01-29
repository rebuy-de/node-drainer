package sqs

import (
	"context"
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"

	"github.com/rebuy-de/node-drainer/pkg/controller"
	"github.com/rebuy-de/node-drainer/pkg/util"
	"github.com/rebuy-de/rebuy-go-sdk/cmdutil"
)

type MessageHandler struct {
	Requests       chan controller.Request
	DrainQueue     *string
	Timeout        int
	SvcAutoscaling autoscalingiface.AutoScalingAPI
	SvcSQS         sqsiface.SQSAPI
	SvcEC2         ec2iface.EC2API
}

type Message struct {
	LifecycleHookName    *string
	AccountId            *string
	RequestId            *string
	LifecycleTransition  *string
	AutoScalingGroupName *string
	Service              *string
	Time                 *string
	EC2InstanceId        *string
	LifecycleActionToken *string
}

func NewMessageHandler(requests chan controller.Request, drainQueue *string, svcAutoscaling autoscalingiface.AutoScalingAPI, svcSQS sqsiface.SQSAPI, svcEC2 ec2iface.EC2API) *MessageHandler {
	return &MessageHandler{
		Requests:       requests,
		DrainQueue:     drainQueue,
		SvcAutoscaling: svcAutoscaling,
		SvcSQS:         svcSQS,
		SvcEC2:         svcEC2,
	}
}

func (mh *MessageHandler) Run(ctx context.Context) {
	log.Info("Waiting for messages")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			result, err := mh.SvcSQS.ReceiveMessage(&sqs.ReceiveMessageInput{
				AttributeNames: []*string{
					aws.String("All"),
				},
				MessageAttributeNames: []*string{
					aws.String(sqs.QueueAttributeNameAll),
				},
				QueueUrl:            mh.DrainQueue,
				MaxNumberOfMessages: aws.Int64(1),
				VisibilityTimeout:   aws.Int64(10),
				WaitTimeSeconds:     aws.Int64(20),
			})
			if err != nil {
				log.Error(err)
				cmdutil.Exit(1)
			}
			mh.handleMessage(result)
		}
	}
}

func (mh *MessageHandler) handleMessage(msg *sqs.ReceiveMessageOutput) {
	if len(msg.Messages) == 0 {
		log.Debug("Received no messages")
		return
	}

	for m := range msg.Messages {
		log.Debugf("Message body: %s", string(*msg.Messages[m].Body))

		messageHandle := msg.Messages[m].ReceiptHandle

		var messageASG util.ASGMessage
		errASG := json.Unmarshal([]byte(*msg.Messages[m].Body), &messageASG)
		if errASG != nil {
			log.Error(errASG)
			mh.deleteConsumedMessage(messageHandle)
		}

		var messageSpot util.SpotMessage
		errSpot := json.Unmarshal([]byte(*msg.Messages[m].Body), &messageSpot)
		if errSpot != nil {
			log.Error(errSpot)
			mh.deleteConsumedMessage(messageHandle)
		}

		if messageASG.AutoScalingGroupName != nil && messageASG.EC2InstanceId != nil {
			name := mh.mustResolveNodeName(messageASG.EC2InstanceId)
			if name == nil {
				mh.deleteConsumedMessage(messageHandle)
				continue
			}

			mh.heartbeat(&messageASG)
			mh.triggerDrain(*name, false, func() {
				mh.notifyASG(&messageASG)
			})

		} else if messageSpot.DetailType != nil {
			name := mh.mustResolveNodeName(messageSpot.Detail.InstanceId)
			if name == nil {
				mh.deleteConsumedMessage(messageHandle)
				continue
			}

			mh.triggerDrain(*name, true, nil)

		} else {
			log.Warn("Invalid message received, skipping...")
			mh.deleteConsumedMessage(messageHandle)
			return

		}
	}
}

func (mh *MessageHandler) heartbeat(msg *util.ASGMessage) {
	log.Infof("Sending ASG heartbeat for instance %s", aws.StringValue(msg.EC2InstanceId))

	input := &autoscaling.RecordLifecycleActionHeartbeatInput{
		AutoScalingGroupName: msg.AutoScalingGroupName,
		InstanceId:           msg.EC2InstanceId,
		LifecycleActionToken: msg.LifecycleActionToken,
		LifecycleHookName:    msg.LifecycleHookName,
	}

	_, err := mh.SvcAutoscaling.RecordLifecycleActionHeartbeat(input)
	if err != nil {
		log.Error(err)
	}
}

func (mh *MessageHandler) triggerDrain(nodeName string, fastpath bool, onDone func()) {
	mh.Requests <- controller.Request{
		NodeName: nodeName,
		Fastpath: fastpath,
		OnDone:   onDone,
	}
}

func (mh *MessageHandler) mustResolveNodeName(instanceID *string) *string {
	name, err := mh.resolveNodeName(instanceID)
	if err != nil {
		log.Error(err)
		cmdutil.Exit(1)
	}
	return name
}

func (mh *MessageHandler) resolveNodeName(instanceID *string) (*string, error) {
	// This indicates an error in either the program code or the AWS API
	// response. Precautionally return an error.
	if instanceID == nil {
		return nil, fmt.Errorf("Cannot resolve nil instance ID")
	}
	if *instanceID == "" {
		return nil, fmt.Errorf("Cannot resolve nil instance ID")
	}

	// Get Instance from EC2 API.
	input := &ec2.DescribeInstancesInput{
		DryRun: aws.Bool(false),
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("instance-state-name"),
				Values: []*string{
					aws.String("pending"),
					aws.String("running"),
				},
			},
		},
		InstanceIds: []*string{instanceID},
	}

	out, err := mh.SvcEC2.DescribeInstances(input)
	if err != nil {
		return nil, err
	}

	// Avoid null dereference.
	if len(out.Reservations) == 0 {
		return nil, nil
	}
	if len(out.Reservations[0].Instances) == 0 {
		return nil, nil
	}

	// Sanity check, since an empty instance ID is a wildcard.
	if len(out.Reservations) > 1 {
		return nil, fmt.Errorf("Found multiple instances for ID %s", aws.StringValue(instanceID))
	}
	if len(out.Reservations[0].Instances) > 1 {
		return nil, fmt.Errorf("Found multiple instances for ID %s", aws.StringValue(instanceID))
	}

	name := out.Reservations[0].Instances[0].PrivateDnsName
	log.Infof("Resolved Instance ID %s to Node Name %s",
		aws.StringValue(instanceID), aws.StringValue(name))
	return name, nil
}

func (mh *MessageHandler) deleteConsumedMessage(receiptHandle *string) {
	log.Debugf("Deleting consumed SQS message for handle %s", aws.StringValue(receiptHandle))
	input := &sqs.DeleteMessageInput{
		QueueUrl:      mh.DrainQueue,
		ReceiptHandle: receiptHandle,
	}

	_, err := mh.SvcSQS.DeleteMessage(input)
	if err != nil {
		log.Error(err)
		cmdutil.Exit(1)
	}
}

func (mh *MessageHandler) notifyASG(msg *util.ASGMessage) {
	log.Debugf("Notifying ASG about draining completion for node %s", aws.StringValue(msg.EC2InstanceId))
	input := &autoscaling.CompleteLifecycleActionInput{
		AutoScalingGroupName:  msg.AutoScalingGroupName,
		InstanceId:            msg.EC2InstanceId,
		LifecycleActionResult: aws.String("CONTINUE"),
		LifecycleActionToken:  msg.LifecycleActionToken,
		LifecycleHookName:     msg.LifecycleHookName,
	}

	_, err := mh.SvcAutoscaling.CompleteLifecycleAction(input)
	if err != nil {
		log.Error(err)
	}
}
