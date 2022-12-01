package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/cmdutil"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/kubeutil"
)

func main() {
	ctx := cmdutil.SignalRootContext()

	kubeInterface, err := new(kubeutil.Params).Client()
	cmdutil.Must(err)

	collector := pod.New(kubeInterface)

	go collector.Run(ctx)

	for !collector.Healthy() {
		time.Sleep(250 * time.Millisecond) // Wait for warmup
	}

	pods := collector.List(ctx)

	for _, pod := range pods {
		bytes, err := json.Marshal(pod)
		cmdutil.Must(err)
		fmt.Println(string(bytes))
	}

}
