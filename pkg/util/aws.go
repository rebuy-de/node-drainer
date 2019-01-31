package util

import (
	"errors"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/rebuy-de/rebuy-go-sdk/cmdutil"
	log "github.com/sirupsen/logrus"
)

type AWSProfile struct {
	Profile         string
	AccessKeyID     string
	SecretAccessKey string
	EC2RoleProvider bool
	SessionToken    string
}

type ASGMessage struct {
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

type SpotMessage struct {
	Version    *string
	Id         *string
	DetailType *string `json:"detail-type"`
	Source     *string
	Time       *string
	Region     *string
	Resources  []*string
	Detail     struct {
		InstanceId     *string `json:"instance-id"`
		InstanceAction *string `json:"instance-action"`
	} `json:"detail"`
}

func (c *AWSProfile) BuildSession(region string) *session.Session {
	cfg := aws.NewConfig()
	cfg.WithRegion(region)
	err := AttachAWSCredentials(cfg, c)
	if err != nil {
		log.Error(err)
		cmdutil.Exit(1)
	}
	awsSession, err := session.NewSession(cfg)
	if err != nil {
		log.Error(err)
		cmdutil.Exit(1)
	}
	return awsSession
}

func (c *AWSProfile) IsValid() bool {
	if !c.Shared() && !c.Static() && !c.EC2RoleProvider {
		return false
	}
	return true
}

func (c *AWSProfile) Shared() bool {
	if c.Profile != "" {
		return true
	}
	return false
}

func (c *AWSProfile) Static() bool {
	if c.AccessKeyID != "" && c.SecretAccessKey != "" {
		return true
	}
	return false
}

func AttachAWSCredentials(cfg *aws.Config, profile *AWSProfile) error {
	if profile.Static() {
		cfg.WithCredentials(credentials.NewStaticCredentials(profile.AccessKeyID, profile.SecretAccessKey, profile.SessionToken))
		return nil
	} else if profile.Shared() {
		cfg.WithCredentials(credentials.NewSharedCredentials("", profile.Profile))
		return nil
	} else if profile.EC2RoleProvider {
		metadata := ec2metadata.New(session.New(), &aws.Config{
			HTTPClient: http.DefaultClient,
		})

		creds := credentials.NewCredentials(&ec2rolecreds.EC2RoleProvider{Client: metadata})
		if _, err := creds.Get(); err != nil {
			return err
		}

		cfg.WithCredentials(creds)
		return nil
	}

	return errors.New("No valid AWS credentials found.")
}

func GetQueueURL(cp client.ConfigProvider, queueName string, awsRegion string, profile *AWSProfile) string {
	accountID := ""

	if profile.Shared() || profile.Static() {
		stsClient := sts.New(cp)
		identity, err := stsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
		if err != nil {
			log.Error(err)
			cmdutil.Exit(1)
		}
		accountID = *identity.Account
	} else if profile.EC2RoleProvider {
		metadataClient := ec2metadata.New(cp)
		info, err := metadataClient.IAMInfo()
		if err != nil {
			log.Error(err)
			cmdutil.Exit(1)
		}

		arn, err := arn.Parse(info.InstanceProfileArn)
		if err != nil {
			log.Error(err)
			cmdutil.Exit(1)
		}
		accountID = arn.AccountID
	}

	return "https://sqs." + awsRegion + ".amazonaws.com/" + accountID + "/" + queueName
}
