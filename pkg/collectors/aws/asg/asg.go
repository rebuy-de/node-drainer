// Package asg provides an interface to ASG Lifecycle Hooks, that are delivered
// via SQS. It manages a local cache, which is filled from SQS messages. The
// instance lifecycle can be completed, so the ASG can continue to terminate an
// instance.
package asg

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/smithy-go"
	"github.com/pkg/errors"

	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/logutil"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/syncutil"
)

// Client is an interface to ASG Lifecycle Hooks.
type Client interface {
	// Run executes the SQS message listener. It will update the instance cache
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

	// SignalEmitter gets triggered every time the cache changes. See syncutil
	// package for more information.
	SignalEmitter() *syncutil.SignalEmitter

	// Healthy indicates whether the background job is running correctly.
	Healthy() bool
}

// Instance is the instance-related data that is retrieved via SQS.
type Instance struct {
	// ID is the EC2 Instance ID
	ID string `logfield:"instance-id,omitempty"`

	// TriggeredAt is the thime then the shutdown was triggered.
	TriggeredAt time.Time `logfield:"lifecycle-triggered-at"`

	// Completed indicates that Complete() was called.
	Completed bool `logfield:"lifecycle-completed"`

	// Deleted indicates that Delete() was called.
	Deleted bool `logfield:"lifecycle-deleted"`
}

type cacheValue struct {
	MessageId     string      `logfield:"message-id"`
	ReceiptHandle string      `logfield:"receipt-handle"`
	Body          messageBody `logfield:",squash"`
	completed     bool        `logfield:"completed"`
	deletedAt     time.Time   `logfield:"deleted-at"`
}

// messageBody is used for decoding JSON from the SQS messages.
type messageBody struct {
	LifecycleHookName    string `logfield:"lifecycle-hook-name"`
	AccountId            string
	RequestId            string `logfield:"request-id"`
	LifecycleTransition  string
	AutoScalingGroupName string `logfield:"autoscaling-group-name"`
	Service              string
	Time                 time.Time `logfield:"triggered-at"`
	EC2InstanceId        string    `logfield:"instance-id,omitempty"`
	LifecycleActionToken string    `logfield:"lifecycle-action-token"`
	Event                string
}

type handler struct {
	asg     *autoscaling.Client
	sqs     *sqs.Client
	url     string
	emitter *syncutil.SignalEmitter

	cache map[string]*cacheValue

	failureCount int
}

// New creates a new client for ASG Lifecycle Hooks that are delivered via SQS.
// It needs to be started with Run so it actually reads messages. See Client
// interface for more information.
func New(ctx context.Context, conf *aws.Config, queueName string) (Client, error) {
	sqsClient := sqs.NewFromConfig(*conf)
	out, err := sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: &queueName,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get queue URL")
	}

	return &handler{
		asg:     autoscaling.NewFromConfig(*conf),
		sqs:     sqsClient,
		url:     *out.QueueUrl,
		cache:   map[string]*cacheValue{},
		emitter: new(syncutil.SignalEmitter),
	}, nil
}

func (h *handler) Healthy() bool {
	return h.failureCount == 0
}

func (h *handler) SignalEmitter() *syncutil.SignalEmitter {
	return h.emitter
}

func (h *handler) Run(ctx context.Context) error {
	ctx = logutil.Start(ctx, "asgclient")

	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			err := h.runOnce(ctx)
			if err != nil {
				logutil.Get(ctx).
					WithError(errors.WithStack(err)).
					Errorf("main loop run failed %d times in a row", h.failureCount)
				h.failureCount++

				// Sleep shortly because we do not want to DoS our logging system.
				time.Sleep(100 * time.Millisecond)
			} else {
				h.failureCount = 0
			}
		}

		for key, value := range h.cache {
			// We delete the deletion from the cache, to give SQS time to
			// propagate the deletion and prevent that it gets readded to the
			// chache.
			if !value.deletedAt.IsZero() && time.Since(value.deletedAt) > 5*time.Minute {
				delete(h.cache, key)
			}
		}
	}
}

func (h *handler) runOnce(ctx context.Context) error {
	result, err := h.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		AttributeNames:        []types.QueueAttributeName{types.QueueAttributeNameAll},
		MessageAttributeNames: []string{"All"},
		QueueUrl:              aws.String(h.url),
		MaxNumberOfMessages:   10,
		VisibilityTimeout:     20,
		WaitTimeSeconds:       10,
	})
	if err != nil {
		var ce *smithy.CanceledError
		if errors.As(err, &ce) {
			// This is a graceful shutdown, triggered by the context
			// and not an actual error.
			return nil
		}
		return errors.Wrap(err, "failed to receive message from SQS")
	}

	for _, message := range result.Messages {
		err := h.handle(ctx, &message)
		if err != nil {
			return errors.Wrap(err, "failed handle message")
		}
	}

	return nil
}

func (h *handler) handle(ctx context.Context, message *types.Message) error {
	ctx = logutil.Start(ctx, "handle")
	cacheItem := cacheValue{
		MessageId:     aws.ToString(message.MessageId),
		ReceiptHandle: aws.ToString(message.ReceiptHandle),
	}

	ctx = logutil.WithFields(ctx, logutil.FromStruct(cacheItem))

	logutil.Get(ctx).
		WithField("message-body", aws.ToString(message.Body)).
		Debugf("got asg lifecycle message")

	err := json.Unmarshal([]byte(aws.ToString(message.Body)), &cacheItem.Body)
	if err != nil {
		return errors.Wrap(err, "failed to decode message body")
	}

	ctx = logutil.WithFields(ctx, logutil.FromStruct(cacheItem))

	if cacheItem.Body.Event == "autoscaling:TEST_NOTIFICATION" {
		logutil.Get(ctx).Info("Skipping autoscaling:TEST_NOTIFICATION event")
		h.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(h.url),
			ReceiptHandle: aws.String(cacheItem.ReceiptHandle),
		})
		return nil
	}

	id := cacheItem.Body.EC2InstanceId

	_, exists := h.cache[id]

	if !exists {
		logutil.Get(ctx).Info("received new ASG lifecycle message")
		h.cache[id] = &cacheItem
		h.emitter.Emit()
	}

	return nil
}

func (h *handler) List() []Instance {
	messages := []Instance{}
	for _, m := range h.cache {
		instance := Instance{
			ID:          m.Body.EC2InstanceId,
			TriggeredAt: m.Body.Time,
			Completed:   m.completed,
			Deleted:     !m.deletedAt.IsZero(),
		}

		messages = append(messages, instance)
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

	if message.completed {
		l.Debugf("instance %s already marked as completed", id)
		return nil
	}

	_, err := h.asg.CompleteLifecycleAction(ctx, &autoscaling.CompleteLifecycleActionInput{
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
	message.completed = true
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

	if !cacheItem.deletedAt.IsZero() {
		l.Debugf("instance %s already marked as deleted", id)
		return nil
	}

	_, err := h.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
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
