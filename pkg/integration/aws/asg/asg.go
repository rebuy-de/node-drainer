// Package asg provides a handler for a ASG Lifecycle Hook, that is delivered
// via SQS. The handler manages a local cache, which is filled from SQS
// messages. The instance lifecycle can be completed, so the ASG can continue
// to terminate an instance. If an instance gets terminated, it will be removed
// from the cache.
package asg

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/rebuy-de/node-drainer/v2/pkg/syncutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
)

type Handler interface {
	// Run executes the SQS message listener. Will update the instance cache
	// based on SQS Messages. It will poll all messages from the ASG Lifecycle
	// Hook and will keep them inflight until the instance actually disapeared.
	Run(ctx context.Context) error

	// List returns all EC2 Instances that are currently in the cache. Those
	// instance cache will be updated in the background, based on SQS Messages.
	List() []Instance

	// Complete finishes the ASG Lifecycle Hook Action with "CONTINUE".
	Complete(ctx context.Context, id string) error

	// Delete deletes the message from SQS.
	Delete(ctx context.Context, id string) error

	SignalEmitter() *syncutil.SignalEmitter
}

type Instance struct {
	// ID is the EC2 Instance ID
	ID string

	// TriggeredAt is the thime then the shutdown was triggered.
	TriggeredAt time.Time

	// CompletedAt is the time when Complete() was called.
	CompletedAt time.Time

	// DeletedAt is the time when Delete() was called. Deleted instaces get
	// deleted after one hour.
	DeletedAt time.Time
}

type cacheValue struct {
	MessageId     string
	ReceiptHandle string
	Body          messageBody
	completedAt   time.Time
	deletedAt     time.Time
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

type handler struct {
	asg     *autoscaling.AutoScaling
	sqs     *sqs.SQS
	url     string
	cache   map[string]*cacheValue
	emitter *syncutil.SignalEmitter
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
		asg:     autoscaling.New(sess),
		sqs:     sqsClient,
		url:     *out.QueueUrl,
		cache:   map[string]*cacheValue{},
		emitter: new(syncutil.SignalEmitter),
	}, nil
}

func (h *handler) SignalEmitter() *syncutil.SignalEmitter {
	return h.emitter
}

func (h *handler) Run(ctx context.Context) error {
	ctx = logutil.Start(ctx, "asglifecycle")

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
				err := h.handle(ctx, message)
				if err != nil {
					return errors.Wrap(err, "failed handle message")
				}
			}
		}
	}
}

func (h *handler) handle(ctx context.Context, message *sqs.Message) error {
	ctx = logutil.Start(ctx, "handle")
	cacheItem := cacheValue{
		MessageId:     aws.StringValue(message.MessageId),
		ReceiptHandle: aws.StringValue(message.ReceiptHandle),
	}

	ctx = logutil.WithField(ctx, "message-id", aws.StringValue(message.MessageId))

	logutil.Get(ctx).Debugf("got message: %s", aws.StringValue(message.Body))

	err := json.Unmarshal([]byte(aws.StringValue(message.Body)), &cacheItem.Body)
	if err != nil {
		return errors.Wrap(err, "failed to decode message body")
	}

	ctx = logutil.WithFields(ctx, logrus.Fields{
		"asg_name":     cacheItem.Body.AutoScalingGroupName,
		"message_time": cacheItem.Body.Time,
		"transistion":  cacheItem.Body.LifecycleTransition,
		"instance_id":  cacheItem.Body.EC2InstanceId,
	})

	if cacheItem.Body.Event == "autoscaling:TEST_NOTIFICATION" {
		logutil.Get(ctx).Info("Skipping autoscaling:TEST_NOTIFICATION event")
		h.sqs.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      aws.String(h.url),
			ReceiptHandle: aws.String(cacheItem.ReceiptHandle),
		})
		return nil
	}

	id := cacheItem.Body.EC2InstanceId

	_, exists := h.cache[id]
	h.cache[id] = &cacheItem

	if !exists {
		logutil.Get(ctx).Info("added instance to cache")
		h.emitter.Emit()
	}

	return nil
}

func (h *handler) List() []Instance {
	messages := []Instance{}
	for _, m := range h.cache {
		messages = append(messages, Instance{
			ID:          m.Body.EC2InstanceId,
			TriggeredAt: m.Body.Time,
			CompletedAt: m.completedAt,
		})
	}

	sort.Slice(messages, func(i, j int) bool {
		return messages[i].TriggeredAt.Before(messages[j].TriggeredAt)
	})

	return messages
}

func (h *handler) Complete(ctx context.Context, id string) error {
	l := logutil.Get(ctx).WithField("instance-id", id)

	message, ok := h.cache[id]
	if !ok {
		l.Warnf("instance %s not found in cache, assuming it is already gone", id)
		return nil
	}

	if message.completedAt.IsZero() {
		l.Debugf("instance %s already marked as completed", id)
		return nil
	}

	_, err := h.asg.CompleteLifecycleAction(&autoscaling.CompleteLifecycleActionInput{
		InstanceId:            &id,
		AutoScalingGroupName:  &message.Body.AutoScalingGroupName,
		LifecycleHookName:     &message.Body.LifecycleHookName,
		LifecycleActionResult: aws.String("CONTINUE"),
	})

	if err != nil && strings.HasPrefix(err.Error(),
		"ValidationError: No active Lifecycle Action found with instance") {
		// Unfortunately this error does not have a proper error code. Anyway,
		// the Complete call should be idempotent, so we ignore this error.
	} else if err != nil {
		return errors.WithStack(err)
	}

	// Note: We neither remove the instance from cache, nor do we delete the
	// message. This is done in the next SQS message receive to be a bit more
	// failsafe. Anyway, it gets marked as completed in the cache to avoid a
	// stale List() output which could cause a unnecessary delay in the main
	// loop.
	message.completedAt = time.Now()
	h.emitter.Emit()

	return nil
}

func (h *handler) Delete(ctx context.Context, id string) error {
	l := logutil.Get(ctx).WithField("instance-id", id)

	cacheItem, ok := h.cache[id]
	if !ok {
		l.Warnf("instance %s not found in cache, assuming it is already gone", id)
		return nil
	}

	if cacheItem.deletedAt.IsZero() {
		l.Debugf("instance %s already marked as deleted", id)
		return nil
	}

	_, err := h.sqs.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(h.url),
		ReceiptHandle: aws.String(cacheItem.ReceiptHandle),
	})
	if err != nil {
		return errors.WithStack(err)
	}

	cacheItem.deletedAt = time.Now()
	h.emitter.Emit()

	return nil
}
