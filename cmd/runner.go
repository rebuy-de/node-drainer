package cmd

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/cmdutil"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/kubeutil"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/node"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
	"github.com/rebuy-de/node-drainer/v2/pkg/vaultutil"
)

type Runner struct {
	noMainloop bool
	sqsQueue   string

	kube  kubeutil.Params
	vault vaultutil.Params
}

func (r *Runner) Bind(cmd *cobra.Command) error {
	cmd.PersistentFlags().StringVar(
		&r.sqsQueue, "queue", "",
		`Name of the SQS queue that contains the ASG lifecycle hook messages.`)
	cmd.PersistentFlags().BoolVar(
		&r.noMainloop, "no-mainloop", false,
		`Disable the mainloop and make the drainer read-only.`)

	r.kube.Bind(cmd)
	r.vault.Bind(cmd)
	r.vault.BindAWS(cmd)

	return nil
}

func (r *Runner) Run(ctx context.Context, cmd *cobra.Command, args []string) {
	ctx = InitIntrumentation(ctx)

	awsSession, err := r.vault.AWSSession()
	cmdutil.Must(err)

	kubeInterface, err := r.kube.Client()
	cmdutil.Must(err)

	collectors := collectors.Collectors{
		EC2:  ec2.New(awsSession, 1*time.Second),
		Spot: spot.New(awsSession, 1*time.Second),
		Node: node.New(kubeInterface),
		Pod:  pod.New(kubeInterface),
	}

	collectors.ASG, err = asg.New(awsSession, r.sqsQueue)
	cmdutil.Must(err)

	mainLoop := NewMainLoop(collectors)

	server := &Server{
		collectors: collectors,
		mainloop:   mainLoop,
	}

	egrp, ctx := errgroup.WithContext(ctx)
	egrp.Go(func() error {
		return errors.Wrap(collectors.EC2.Run(ctx), "failed to run ec2 watch client")
	})
	egrp.Go(func() error {
		return errors.Wrap(collectors.Spot.Run(ctx), "failed to run spot watch client")
	})
	egrp.Go(func() error {
		return errors.Wrap(collectors.ASG.Run(ctx), "failed to run ASG Lifecycle client")
	})
	egrp.Go(func() error {
		return errors.Wrap(collectors.Node.Run(ctx), "failed to run Kubernetes node client")
	})
	egrp.Go(func() error {
		return errors.Wrap(collectors.Pod.Run(ctx), "failed to run Kubernetes pod client")
	})
	egrp.Go(func() error {
		return errors.Wrap(server.Run(ctx), "failed to run HTTP server")
	})
	if !r.noMainloop {
		egrp.Go(func() error {
			return errors.Wrap(mainLoop.Run(ctx), "failed to run main loop")
		})
	}
	cmdutil.Must(egrp.Wait())
}
