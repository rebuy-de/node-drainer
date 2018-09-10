package util

import (
	"github.com/rebuy-de/rebuy-go-sdk/cmdutil"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func KubernetesClientset(kubeconfig string) *kubernetes.Clientset {
	var (
		config *rest.Config
		err    error
	)
	config, err = rest.InClusterConfig()
	if err != nil && kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		log.Error("failed to configure kubernetes client")
		cmdutil.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err)
		cmdutil.Exit(1)
	}
	return clientset
}
