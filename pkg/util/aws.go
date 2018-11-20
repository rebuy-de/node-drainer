package util

import (
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
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
	if !c.Shared() && !c.Static() {
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
		cfg.WithCredentials(credentials.NewCredentials(&ec2rolecreds.EC2RoleProvider{}))
		return nil
	}

	return errors.New("No valid AWS credentials found.")
}

func GetQueueURL(cp client.ConfigProvider, queueName string, awsRegion string) string {
	stsClient := sts.New(cp)
	identity, err := stsClient.GetCallerIdentity(nil)
	if err != nil {
		log.Error(err)
		cmdutil.Exit(1)
	}
	return "https://sqs." + awsRegion + ".amazonaws.com/" + *identity.Account + "/" + queueName
}
