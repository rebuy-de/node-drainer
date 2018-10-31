package sqs

import (
	"encoding/json"
	"os"
	"os/signal"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
	"github.com/rebuy-de/node-drainer/pkg/util"
	"github.com/rebuy-de/rebuy-go-sdk/cmdutil"
)

type Trigger interface {
	Drain(string) error
}

type MessageHandler struct {
	Drainer        Trigger
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

func NewMessageHandler(drainer Trigger, drainQueue *string, timeout int, svcAutoscaling autoscalingiface.AutoScalingAPI, svcSQS sqsiface.SQSAPI, svcEC2 ec2iface.EC2API) *MessageHandler {
	return &MessageHandler{
		Drainer:        drainer,
		DrainQueue:     drainQueue,
		Timeout:        timeout,
		SvcAutoscaling: svcAutoscaling,
		SvcSQS:         svcSQS,
		SvcEC2:         svcEC2,
	}
}

func (mh *MessageHandler) Run() error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	log.Info("Waiting for messages")

	for {
		select {
		case <-c:
			return nil
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
				VisibilityTimeout:   aws.Int64(1),
				WaitTimeSeconds:     aws.Int64(int64(mh.Timeout)),
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
		var message util.Message
		err := json.Unmarshal([]byte(*msg.Messages[m].Body), &message)

		log.Debugf("Message body: ", message)

		messageHandle := msg.Messages[m].ReceiptHandle

		if err != nil {
			log.Error(err)
			mh.deleteConsumedMessage(messageHandle)
			return
		}

		if message.EC2InstanceId == nil {
			log.Warn("Invalid message received, skipping...")
			mh.deleteConsumedMessage(messageHandle)
			return
		}

		// Sending a heartbeat to ASG to ensure enough time is given for draining
		mh.heartbeat(&message)
		mh.triggerDrain(&message)
		mh.deleteConsumedMessage(messageHandle)
		mh.notifyASG(&message)
	}
}

func (mh *MessageHandler) heartbeat(msg *util.Message) {
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

func (mh *MessageHandler) triggerDrain(msg *util.Message) {
	var filter []*ec2.Filter
	var instanceIDs []*string

	instanceIDs = append(instanceIDs, msg.EC2InstanceId)

	input := &ec2.DescribeInstancesInput{DryRun: aws.Bool(false), Filters: filter, InstanceIds: instanceIDs}
	out, err := mh.SvcEC2.DescribeInstances(input)
	if err != nil {
		log.Error(err)
	}

	for r := range out.Reservations {
		for i := range out.Reservations[r].Instances {
			mh.Drainer.Drain(*out.Reservations[r].Instances[i].PrivateDnsName)
		}
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

func (mh *MessageHandler) notifyASG(msg *util.Message) {
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
