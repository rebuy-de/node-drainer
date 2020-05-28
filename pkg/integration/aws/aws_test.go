package aws_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"

	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/tftest"
)

func TestAll(t *testing.T) {
	if testing.Verbose() {
		logrus.SetLevel(logrus.DebugLevel)
	}

	helper := testNewHelper(t)

	helper.headline("Initialize Test")
	helper.section("Setup AWS Resources with Terraform")
	tf := tftest.New(t, "test-fixtures")
	defer tf.Destroy()
	defer helper.headline("Destroy Terraform Resources")
	tf.Create()
	autoscalingGroupName := tf.Output("autoscaling_group_name")
	sqsQueueName := tf.Output("aws_sqs_queue_name")

	helper.section("Start SQS Message Handler")
	helper.waitForQueue(sqsQueueName)
	handler, err := asg.NewHandler(helper.sess, sqsQueueName)
	helper.must(err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { helper.must(handler.Run(ctx)) }()

	helper.section("Collect Data from AWS")
	autoscalingGroup := helper.getASG(autoscalingGroupName)

	helper.headline("Start Actual Test")
	helper.section("Verify that List is Empty")
	helper.waitForInstances(handler, 30*time.Second, 0)

	helper.section("Terminate the First Instance")
	helper.teminateASGInstance(autoscalingGroup.Instances[0].InstanceId)
	instances := helper.waitForInstances(handler, 30*time.Second, 1)

	helper.section("Terminate the Second Instance")
	helper.teminateASGInstance(autoscalingGroup.Instances[1].InstanceId)
	instances = helper.waitForInstances(handler, 30*time.Second, 2)

	helper.section("Check Instance IDs and Orders")
	if instances[0].ID != aws.StringValue(autoscalingGroup.Instances[0].InstanceId) {
		t.Fatalf("first instance should be the oldest one, which is %s; go %s",
			aws.StringValue(autoscalingGroup.Instances[0].InstanceId), instances[0].ID)
	}

	if instances[1].ID != aws.StringValue(autoscalingGroup.Instances[1].InstanceId) {
		t.Fatalf("second instance should be the newer one, which is %s; go %s",
			aws.StringValue(autoscalingGroup.Instances[1].InstanceId), instances[1].ID)
	}

	helper.section("Complete First Instance and Wait for it to Disappear")
	helper.must(handler.Complete(ctx, instances[0].ID))
	instances = helper.waitForInstances(handler, 30*time.Second, 1)

	helper.section("Check instance ID")
	if instances[0].ID != aws.StringValue(autoscalingGroup.Instances[1].InstanceId) {
		t.Fatalf("second instance should be the newer one, which is %s; go %s",
			aws.StringValue(autoscalingGroup.Instances[1].InstanceId), instances[0].ID)
	}

	helper.section("Complete second instance and Wait for it to Disappear")
	helper.must(handler.Complete(ctx, instances[0].ID))
	instances = helper.waitForInstances(handler, 30*time.Second, 0)
}

type testHelper struct {
	t    *testing.T
	sess *session.Session
	asg  *autoscaling.AutoScaling
	sqs  *sqs.SQS
}

func testNewHelper(t *testing.T) *testHelper {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	return &testHelper{
		t:    t,
		sess: sess,
		asg:  autoscaling.New(sess),
		sqs:  sqs.New(sess),
	}
}

func (h *testHelper) must(err error) {
	h.t.Helper()
	if err != nil {
		h.t.Fatal(err)
	}
}

func (h *testHelper) headline(message string) {
	prefix := aurora.Bold("### ").Gray(8)
	text := aurora.Bold(message).Blue()

	fmt.Println()
	fmt.Println(prefix)
	fmt.Print(prefix)
	fmt.Print(text)
	fmt.Println()
	fmt.Println(prefix)
}

func (h *testHelper) section(message string) {
	prefix := aurora.Bold("## ").Gray(8)
	text := aurora.Bold(message).Blue()

	fmt.Println()
	fmt.Print(prefix)
	fmt.Print(text)
	fmt.Println()
}

func (h *testHelper) waitForQueue(name string) {
	h.t.Helper()

	var err error
	for i := 0; i < 5; i++ {
		_, err = h.sqs.GetQueueUrl(&sqs.GetQueueUrlInput{
			QueueName: &name,
		})
		if err == nil {
			return
		}

		time.Sleep((1 << i) * time.Second) // Exponential backoff
	}
	h.must(err)
}

func (h *testHelper) getASG(name string) *autoscaling.Group {
	output, err := h.asg.DescribeAutoScalingGroups(&autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []*string{
			&name,
		},
	})
	h.must(err)

	asgs := output.AutoScalingGroups
	if len(asgs) == 0 {
		h.t.Fatalf("Autoscaling group %s not found", name)
	}

	if len(asgs) > 1 {
		h.t.Fatalf("multiple AutoscalingGroups found")
	}

	return output.AutoScalingGroups[0]
}

func (h *testHelper) teminateASGInstance(id *string) {
	_, err := h.asg.TerminateInstanceInAutoScalingGroup(&autoscaling.TerminateInstanceInAutoScalingGroupInput{
		InstanceId:                     id,
		ShouldDecrementDesiredCapacity: aws.Bool(false),
	})
	h.must(err)
}

func (h *testHelper) waitForInstances(handler asg.Handler, timeout time.Duration, want int) []asg.Instance {
	start := time.Now()
	instances := []asg.Instance{}
	for time.Since(start) < timeout {
		instances = handler.List()
		if len(instances) == want {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(instances) != want {
		h.t.Fatalf("exected %d instances, but got %d", want, len(instances))
	}

	return instances
}
