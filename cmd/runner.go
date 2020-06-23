package cmd

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/cmdutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/kubeutil"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/node"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
)

type Runner struct {
	awsProfile string
	sqsQueue   string

	kube kubeutil.Params
}

func (r *Runner) Bind(cmd *cobra.Command) error {
	cmd.PersistentFlags().StringVar(
		&r.awsProfile, "profile", "",
		`use a specific AWS profile from your credential file`)
	cmd.PersistentFlags().StringVar(
		&r.sqsQueue, "queue", "",
		`name of the SQS queue that contains the ASG lifecycle hook messages`)

	r.kube.Bind(cmd)

	return nil
}

func (r *Runner) Run(ctx context.Context, cmd *cobra.Command, args []string) {
	ctx = InitIntrumentation(ctx)

	awsSession, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Profile:           r.awsProfile,
	})
	cmdutil.Must(err)

	kubeInterface, err := r.kube.Client()
	cmdutil.Must(err)

	asgClient, err := asg.New(awsSession, r.sqsQueue)
	cmdutil.Must(err)

	ec2Client := ec2.New(awsSession, 1*time.Second)
	spotClient := spot.New(awsSession, 1*time.Second)
	nodeClient := node.New(kubeInterface)
	podClient := pod.New(kubeInterface)

	mainLoop := NewMainLoop(asgClient, ec2Client, spotClient, nodeClient, podClient)

	server := &Server{
		ec2:      ec2Client,
		asg:      asgClient,
		spot:     spotClient,
		nodes:    nodeClient,
		pods:     podClient,
		mainloop: mainLoop,
	}

	egrp, ctx := errgroup.WithContext(ctx)
	egrp.Go(func() error {
		return errors.Wrap(ec2Client.Run(ctx), "failed to run ec2 watch client")
	})
	egrp.Go(func() error {
		return errors.Wrap(spotClient.Run(ctx), "failed to run spot watch client")
	})
	egrp.Go(func() error {
		return errors.Wrap(asgClient.Run(ctx), "failed to run ASG Lifecycle client")
	})
	egrp.Go(func() error {
		return errors.Wrap(nodeClient.Run(ctx), "failed to run Kubernetes node client")
	})
	egrp.Go(func() error {
		return errors.Wrap(podClient.Run(ctx), "failed to run Kubernetes node client")
	})
	egrp.Go(func() error {
		return errors.Wrap(server.Run(ctx), "failed to run HTTP server")
	})
	egrp.Go(func() error {
		return errors.Wrap(mainLoop.Run(ctx), "failed to run main loop")
	})
	cmdutil.Must(egrp.Wait())
}
