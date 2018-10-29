package cmd

import (
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	awssqs "github.com/aws/aws-sdk-go/service/sqs"
	"github.com/rebuy-de/node-drainer/pkg/drainer"
	"github.com/rebuy-de/node-drainer/pkg/prom"
	"github.com/rebuy-de/node-drainer/pkg/sqs"
	"github.com/rebuy-de/node-drainer/pkg/util"
	"github.com/rebuy-de/rebuy-go-sdk/cmdutil"
	log "github.com/sirupsen/logrus"
	cobra "github.com/spf13/cobra"
)

type NodeDrainer struct {
	Kubeconfig  string
	Profile     *util.AWSProfile
	LogLevel    string
	QueueURL    string
	AWSRegion   string
	SQSWait     int
	MetricsPort string
}

func (nd *NodeDrainer) Run(cmd *cobra.Command, args []string) {
	if !nd.Profile.IsValid() {
		log.Error("incorrect AWS credentials, exiting...")
		cmdutil.Exit(1)
	}

	prom.Run(nd.MetricsPort)

	logLevel, err := log.ParseLevel(nd.LogLevel)
	if err != nil {
		log.Error("incorrect log level set, exiting...\n" + err.Error())
		cmdutil.Exit(1)
	}
	log.SetLevel(logLevel)

	if nd.QueueURL == "" {
		log.Error("no SQS url specified, exiting...")
		cmdutil.Exit(1)
	}
	url := nd.QueueURL

	if nd.AWSRegion == "" {
		log.Error("no AWS region specified, exiting...")
		cmdutil.Exit(1)
	}

	drainer := drainer.NewDrainer(util.KubernetesClientset(nd.Kubeconfig))
	session := nd.Profile.BuildSession(nd.AWSRegion)
	svcAutoscaling := autoscaling.New(session)
	svcSqs := awssqs.New(session)
	svcEc2 := ec2.New(session)
	queueUrl := util.GetQueueURL(session, url, nd.AWSRegion)
	sqs := sqs.NewMessageHandler(drainer, &queueUrl, nd.SQSWait, svcAutoscaling, svcSqs, svcEc2)
	sqs.Run()
}

func (nd *NodeDrainer) Bind(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(
		&nd.Kubeconfig, "kubeconfig", "k", "",
		"Location of the kubeconfig file for local deployment.")

	cmd.PersistentFlags().StringVarP(
		&nd.Profile.Profile, "profile", "p", "",
		"Name of the AWS profile name for accessing the AWS API. "+
			"Cannot be used together with --access-key-id, --secret-access-key "+
			"and --session-token.")

	cmd.PersistentFlags().StringVar(
		&nd.Profile.AccessKeyID, "access-key-id", "",
		"AWS access key ID for accessing the AWS API. "+
			"Must be used together with --secret-access-key."+
			"Cannot be used together with --profile.")

	cmd.PersistentFlags().StringVar(
		&nd.Profile.SecretAccessKey, "secret-access-key", "",
		"AWS secret access key for accessing the AWS API. "+
			"Must be used together with --access-key-id."+
			"Cannot be used together with --profile.")

	cmd.PersistentFlags().StringVar(
		&nd.Profile.SessionToken, "session-token", "",
		"AWS session token for accessing the AWS API. "+
			"Must be used together with --access-key-id and --secret-access-key."+
			"Cannot be used together with --profile.")

	cmd.PersistentFlags().StringVarP(
		&nd.LogLevel, "log-level", "l", "info",
		"Log level. Defults to info.")

	cmd.PersistentFlags().StringVarP(
		&nd.QueueURL, "queue-name", "q", "",
		"The name of the sqs Queue, used to generate the queue address. This argument is mandatory.")

	cmd.PersistentFlags().StringVarP(
		&nd.AWSRegion, "region", "r", "",
		"AWS region. This argument is mandatory.")

	cmd.PersistentFlags().IntVarP(
		&nd.SQSWait, "sqs-wait-interval", "w", 10,
		"Time to wait between successive SQS polling calls, values must be between 0 and 20 (seconds).")

	cmd.PersistentFlags().StringVarP(
		&nd.MetricsPort, "metrics-port", "m", "8080",
		"Port on which prometheus `/metrics` will be exposed.")
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
