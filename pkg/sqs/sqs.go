package sqs

import (
	"context"
	"encoding/json"
	"time"

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
				WaitTimeSeconds:     aws.Int64(30),
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
			mh.heartbeat(&messageASG)
			mh.triggerDrain(messageASG.EC2InstanceId, false, func() {
				mh.deleteConsumedMessage(messageHandle)
				mh.notifyASG(&messageASG)
			})

		} else if messageSpot.DetailType != nil {
			for _, instanceId := range messageSpot.Resources {
				mh.triggerDrain(instanceId, true, nil)
			}

			// Note: Deleting this message does not wait for drain to be done.
			// This is because it would be quite hard to actually sync multiple
			// drain routines via callback. Therefore we wait tem minutes at
			// least, so we could pick up it again on a possible restart.
			time.AfterFunc(10*time.Minute, func() {
				mh.deleteConsumedMessage(messageHandle)
			})

		} else {
			log.Warn("Invalid message received, skipping...")
			mh.deleteConsumedMessage(messageHandle)
			return

		}
	}
}

func (mh *MessageHandler) heartbeat(msg *util.ASGMessage) {
	log.Info("Sending ASG heartbeat for instance: " + *msg.EC2InstanceId)

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

func (mh *MessageHandler) triggerDrain(instanceID *string, fastpath bool, onDone func()) {
	var filter []*ec2.Filter
	var instanceIDs []*string

	instanceIDs = append(instanceIDs, instanceID)

	input := &ec2.DescribeInstancesInput{
		DryRun:      aws.Bool(false),
		Filters:     filter,
		InstanceIds: instanceIDs,
	}
	out, err := mh.SvcEC2.DescribeInstances(input)
	if err != nil {
		log.Error(err)
		if onDone != nil {
			onDone()
		}
		return
	}

	requested := false
	for r := range out.Reservations {
		for i := range out.Reservations[r].Instances {
			mh.Requests <- controller.Request{
				InstanceID: *out.Reservations[r].Instances[i].PrivateDnsName,
				Fastpath:   fastpath,
				OnDone:     onDone,
			}
			requested = true
		}
	}

	if !requested && onDone != nil {
		onDone()
	}
}

func (mh *MessageHandler) deleteConsumedMessage(receiptHandle *string) {
	log.Debug("Deleting consumed SQS message: " + *receiptHandle)
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
	log.Debug("Notifying ASG about draining completion for node: " + *msg.EC2InstanceId)
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
