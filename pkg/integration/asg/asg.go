// Package asg provides a handler for a ASG Lifecycle Hook, that is delivered
// via SQS. The handler manages a local cache, that is filled from SQS
// messages. The instance lifecycle can be completed, so the ASG can continue
// to terminate an instance. If an instance gets terminated, it will be removed
// from the cache.
package asg

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Handler interface {
	// Run executes the SQS message listener. Will update the instance cache
	// based on SQS Messages. It will poll all messages from the ASG Lifecycle
	// Hook and will keep them inflight until the instance actually disapeared.
	Run(ctx context.Context) error

	// List returns all EC2 Instances that are currently in the cache. Those
	// instance cache will be updated in the background, based on SQS Messages.
	List() []Instance

	// Complete finishes the ASG Lifecycle Hook Action with "CONTINUE". It does
	// not yet remove the instance from the cache until the instance actually
	// terminated.
	Complete(id string) error
}

type Instance struct {
	// ID is the EC2 Instance ID
	ID string

	// Name is the node name of the EC2 Instance which is based on the private
	// DNS name.
	Name string
}

type cacheValue struct {
	MessageId     string
	ReceiptHandle string
	NodeName      string
	Body          messageBody
}

type messageBody struct {
	LifecycleHookName    string
	AccountId            string
	RequestId            string
	LifecycleTransition  string
	AutoScalingGroupName string
	Service              string
	Time                 time.Time
	EC2InstanceId        string
	LifecycleActionToken string
	Event                string
}

type instanceState string

const (
	instanceStateUnknown  instanceState = ""
	instanceStateNotFound               = "not-found"
)

func (s instanceState) IsRunning() bool {
	switch s {
	case "shutting-down":
		fallthrough
	case "terminated":
		return false
	case "running":
		return true
	default:
		// Assume that it is running, so we still can try to drain the node.
		return true
	}
}

type handler struct {
	asg    *autoscaling.AutoScaling
	sqs    *sqs.SQS
	ec2    *ec2.EC2
	url    string
	cache  map[string]cacheValue
	logger logrus.FieldLogger
}

// NewHandler creates a new Handler for ASG Lifecycle Hooks that are delivered
// via SQS. It needs to be started with Run so it actually reads messages. See
// Handler interface for more information.
func NewHandler(sess *session.Session, queueName string) (Handler, error) {
	sqsClient := sqs.New(sess)
	out, err := sqsClient.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: &queueName,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get queue URL")
	}

	return &handler{
		asg:    autoscaling.New(sess),
		sqs:    sqsClient,
		ec2:    ec2.New(sess),
		url:    *out.QueueUrl,
		cache:  map[string]cacheValue{},
		logger: logrus.WithField("subsystem", "asghandler"),
	}, nil
}

func (h *handler) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			result, err := h.sqs.ReceiveMessageWithContext(ctx, &sqs.ReceiveMessageInput{
				AttributeNames: []*string{
					aws.String("All"),
				},
				MessageAttributeNames: []*string{
					aws.String(sqs.QueueAttributeNameAll),
				},
				QueueUrl:            aws.String(h.url),
				MaxNumberOfMessages: aws.Int64(10),
				VisibilityTimeout:   aws.Int64(20),
				WaitTimeSeconds:     aws.Int64(10),
			})
			if err != nil {
				aerr, ok := err.(awserr.Error)
				if ok && aerr.Code() == request.CanceledErrorCode {
					// This is a graceful shutdown, triggered by the context
					// and not an actual error.
					return nil
				}
				return errors.Wrap(err, "failed to receive message from SQS")
			}

			for _, message := range result.Messages {
				err := h.handle(message)
				if err != nil {
					return errors.Wrap(err, "failed handle message")
				}
			}
		}
	}
}

func (h *handler) handle(message *sqs.Message) error {
	cacheItem := cacheValue{
		MessageId:     aws.StringValue(message.MessageId),
		ReceiptHandle: aws.StringValue(message.ReceiptHandle),
	}

	l := h.logger.WithFields(logrus.Fields{
		"message_id": aws.StringValue(message.MessageId),
	})

	l.Debugf("got message: %s", aws.StringValue(message.Body))

	err := json.Unmarshal([]byte(aws.StringValue(message.Body)), &cacheItem.Body)
	if err != nil {
		return errors.Wrap(err, "failed to decode message body")
	}

	l = l.WithFields(logrus.Fields{
		"asg_name":     cacheItem.Body.AutoScalingGroupName,
		"message_time": cacheItem.Body.Time,
		"transistion":  cacheItem.Body.LifecycleTransition,
		"instance_id":  cacheItem.Body.EC2InstanceId,
	})

	if cacheItem.Body.Event == "autoscaling:TEST_NOTIFICATION" {
		l.Info("Skipping autoscaling:TEST_NOTIFICATION event")
		h.sqs.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      aws.String(h.url),
			ReceiptHandle: aws.String(cacheItem.ReceiptHandle),
		})
		return nil
	}

	id := cacheItem.Body.EC2InstanceId
	nodeName, state, err := h.getInstance(id)
	cacheItem.NodeName = nodeName
	if err != nil {
		return errors.Wrapf(err, "failed to get instance state for ID %s", id)
	}

	l = l.WithFields(logrus.Fields{
		"node_name": nodeName,
		"state":     state,
	})

	if !state.IsRunning() {
		h.sqs.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      aws.String(h.url),
			ReceiptHandle: aws.String(cacheItem.ReceiptHandle),
		})
		delete(h.cache, id)
		l.Info("removed non-existing instance from cache")
		return nil
	}

	_, exists := h.cache[id]
	h.cache[id] = cacheItem

	if exists {
		l.Debug("instance already in cache")
	} else {
		l.Info("added instance to cache")
	}

	return nil
}

func (h *handler) getInstance(id string) (string, instanceState, error) {
	l := h.logger.WithField("instance_id", id)

	statusOutput, err := h.ec2.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{
			aws.String(id),
		},
	})

	if err != nil {
		awsErr := err.(awserr.Error)
		if awsErr.Code() == "InvalidInstanceID.NotFound" {
			l.Warnf("instance with ID %s not found", id)
			return "", instanceStateNotFound, nil
		}
		return "", instanceStateUnknown, errors.Wrap(err, "failed to describe instance")
	}

	if len(statusOutput.Reservations) > 1 || len(statusOutput.Reservations[0].Instances) > 1 {
		return "", instanceStateUnknown, errors.Errorf("found multiple instances")
	}

	if len(statusOutput.Reservations[0].Instances) == 0 {
		return "", instanceStateUnknown, nil
	}

	var (
		ec2instance = statusOutput.Reservations[0].Instances[0]
		nodeName    = aws.StringValue(ec2instance.PrivateDnsName)
		state       = aws.StringValue(ec2instance.State.Name)
	)

	return nodeName, instanceState(state), nil
}

func (h *handler) List() []Instance {
	messages := []cacheValue{}
	for _, m := range h.cache {
		messages = append(messages, m)
	}

	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Body.Time.Before(messages[j].Body.Time)
	})

	result := []Instance{}
	for _, m := range messages {
		result = append(result, Instance{
			ID:   m.Body.EC2InstanceId,
			Name: m.NodeName,
		})
	}

	return result
}

func (h *handler) Complete(id string) error {
	message, ok := h.cache[id]
	if !ok {
		logrus.Warnf("instance %s not found in cache, assuming it is already gone", id)
		return nil
	}

	_, err := h.asg.CompleteLifecycleAction(&autoscaling.CompleteLifecycleActionInput{
		InstanceId:            &id,
		AutoScalingGroupName:  &message.Body.AutoScalingGroupName,
		LifecycleHookName:     &message.Body.LifecycleHookName,
		LifecycleActionResult: aws.String("CONTINUE"),
	})

	// Note: We neither remove the instance from cache, nor do we delete the
	// message. This is done in the next SQS message receive to be a bit more failsafe.

	return errors.WithStack(err)
}
