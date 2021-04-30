package vaultutil

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/logutil"
)

const (
	RenewIntervalSeconds = 1800
)

type Manager struct {
	ctx    context.Context
	params Params
	client *api.Client
}

func Init(ctx context.Context, params Params) (*Manager, error) {
	ctx = logutil.Start(ctx, "vault-manager")

	conf := api.DefaultConfig()
	conf.Address = params.Address

	client, err := api.NewClient(conf)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if params.Token == "" {
		secret, err := KubernetesToken(client, params.Role)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		client.SetToken(secret.Auth.ClientToken)
	} else {
		client.SetToken(params.Token)
	}

	secret, err := client.Auth().Token().RenewSelf(RenewIntervalSeconds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	client.SetToken(secret.Auth.ClientToken)

	logutil.Get(ctx).
		WithField("secret-data", prettyPrintSecret(secret)).
		Debugf("got initial secret")

	watcher, err := client.NewLifetimeWatcher(&api.LifetimeWatcherInput{
		Secret:      secret,
		RenewBuffer: 3,
		Increment:   RenewIntervalSeconds,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	go watcher.Start()

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for ctx.Err() == nil {
			select {
			case out := <-watcher.RenewCh():
				logutil.Get(ctx).
					WithField("secret-data", prettyPrintSecret(out.Secret)).
					Debugf("renewed secret")

			case err := <-watcher.DoneCh():
				logutil.Get(ctx).
					WithError(errors.WithStack(err)).
					Errorf("renewal stopped")
				cancel()

			case <-ctx.Done():
				logutil.Get(ctx).
					Warnf("renewal canceled")
				cancel()
			}
		}

		logutil.Get(ctx).
			WithError(errors.WithStack(err)).
			Errorf("shutting down vault manager")
		watcher.Stop()
		err := client.Auth().Token().RevokeSelf("")
		if err != nil {
			logutil.Get(ctx).
				WithError(errors.WithStack(err)).
				Errorf("revoking own token failed")
		} else {
			logutil.Get(ctx).
				WithError(errors.WithStack(err)).
				Debugf("revoking own token succeeded")
		}
	}()

	return &Manager{
		ctx:    ctx,
		params: params,
		client: client,
	}, nil
}

func (m *Manager) AWSSession() (*session.Session, error) {
	var (
		provider = m.AWSCredentialsProvider()
		creds    = credentials.NewCredentials(provider)
		conf     = &aws.Config{Credentials: creds}
	)

	sess, err := session.NewSession(conf)
	return sess, errors.WithStack(err)
}

func (m *Manager) AWSCredentialsProvider() credentials.Provider {
	return &awsCredentialsProvider{
		manager: m,
	}
}
