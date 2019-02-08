package cmd

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	awssqs "github.com/aws/aws-sdk-go/service/sqs"
	log "github.com/sirupsen/logrus"
	cobra "github.com/spf13/cobra"

	"github.com/rebuy-de/node-drainer/pkg/controller"
	"github.com/rebuy-de/node-drainer/pkg/drainer"
	"github.com/rebuy-de/node-drainer/pkg/prom"
	"github.com/rebuy-de/node-drainer/pkg/sqs"
	"github.com/rebuy-de/node-drainer/pkg/util"
	"github.com/rebuy-de/rebuy-go-sdk/cmdutil"
)

type NodeDrainer struct {
	Kubeconfig  string
	Profile     *util.AWSProfile
	LogLevel    string
	QueueURL    string
	AWSRegion   string
	VaultServer string
	MetricsPort string
	CoolDown    time.Duration
}

func (nd *NodeDrainer) Run(ctx context.Context, cmd *cobra.Command, args []string) {
	logLevel, err := log.ParseLevel(nd.LogLevel)
	if err != nil {
		log.Error("incorrect log level set, exiting...\n" + err.Error())
		cmdutil.Exit(1)
	}
	log.SetLevel(logLevel)

	if nd.VaultServer == "" {
		log.Error("No vault server specified, exiting...")
		cmdutil.Exit(1)
	}

	if nd.AWSRegion == "" {
		log.Error("no AWS region specified, exiting...")
		cmdutil.Exit(1)
	}

	profile, err := util.GenerateAWSCredentials(nd.VaultServer)
	if err != nil {
		log.Error("Couldn't get credentials from vault...\n" + err.Error())
		cmdutil.Exit(1)
	}

	nd.Profile = &profile
	log.Debugf("Sleeping for %d seconds", 10)
	time.Sleep(10 * time.Second)

	metricsRegistry := prom.Run(nd.MetricsPort)

	if nd.QueueURL == "" {
		log.Error("no SQS url specified, exiting...")
		cmdutil.Exit(1)
	}
	url := nd.QueueURL

	session := nd.Profile.BuildSession(nd.AWSRegion)
	svcAutoscaling := autoscaling.New(session)
	svcSqs := awssqs.New(session)
	svcEc2 := ec2.New(session)
	queueUrl := util.GetQueueURL(session, url, nd.AWSRegion, nd.Profile)

	requests := make(chan controller.Request, 100)
	drainer := drainer.NewDrainer(util.KubernetesClientset(nd.Kubeconfig))

	sqs := sqs.NewMessageHandler(requests, &queueUrl, svcAutoscaling, svcSqs, svcEc2)
	ctl := controller.New(drainer, requests, nd.CoolDown)
	ctl.RegisterMetrics(metricsRegistry)

	go sqs.Run(ctx)
	cmdutil.Must(ctl.Reconcile(ctx))
}

func (nd *NodeDrainer) Bind(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(
		&nd.Kubeconfig, "kubeconfig", "k", "",
		"Location of the kubeconfig file for local deployment.")

	cmd.PersistentFlags().StringVarP(
		&nd.LogLevel, "log-level", "l", "info",
		"Log level. Defults to info.")

	cmd.PersistentFlags().StringVarP(
		&nd.QueueURL, "queue-name", "q", "",
		"The name of the sqs Queue, used to generate the queue address. This argument is mandatory.")

	cmd.PersistentFlags().StringVarP(
		&nd.AWSRegion, "region", "r", "",
		"AWS region. This argument is mandatory.")

	cmd.PersistentFlags().StringVar(
		&nd.VaultServer, "vault", "",
		"Vault server address. This argument is mandatory.")

	cmd.PersistentFlags().StringVarP(
		&nd.MetricsPort, "metrics-port", "m", "8080",
		"Port on which prometheus `/metrics` will be exposed.")

	cmd.PersistentFlags().DurationVarP(
		&nd.CoolDown, "cool-down", "c", 10*time.Minute,
		"Time node-drainer should sleep after draining a node before starting to handle the next one.")
}

func NewRootCommand() *cobra.Command {
	nd := new(NodeDrainer)
	nd.Profile = new(util.AWSProfile)
	cmd := cmdutil.NewRootCommand(nd)

	cmd.Use = "node-drainer"
	cmd.Short = "Node drainer utility."
	cmd.Long = `Drains selected kubernetes nodes while applying a NoSchedule taint. ` +
		`Nodes to be drained are selected by receiving AWS ASG lifecycle hook triggers over sqs.`

	return cmd
}
