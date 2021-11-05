package cmd

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"sort"

	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/webutil"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
)

// The '*' below is needed to include files starting with '.' or '_'
// Might change with Go 1.18 or later to `//go:embed all:templates`
// https://github.com/golang/go/issues/42328#issuecomment-725579848

//go:embed templates/*
var templates embed.FS

// Healthier is a simple interface, that can easily be implemented by all
// critical services. It is used to indicate their health statuses.
type Healthier interface {
	Healthy() bool
}

// HealthHandler is a http.Handler that is used for the lifeness probe.
type HealthHandler struct {
	services map[string]Healthier
}

// ServeHTTP reponds 200, when all services are healhy. Otherwise it responds
// with 503.
func (h HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	unhealthy := []string{}
	for name, service := range h.services {
		if !service.Healthy() {
			unhealthy = append(unhealthy, name)
		}
	}

	sort.Strings(unhealthy)

	if len(unhealthy) == 0 {
		fmt.Fprintln(w, "HEALTHY")
		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
	fmt.Fprintln(w, "UNHEALTHY:")
	for _, name := range unhealthy {
		fmt.Fprintf(w, "- %s ERRORED\n", name)
	}

}

// Server is the HTTP server, which is used for the status page, metrics and
// healthyness.
type Server struct {
	collectors collectors.Collectors

	mainloop *MainLoop
	renderer *webutil.TemplateRenderer
}

// Run starts the actual HTTP server.
func (s *Server) Run(ctx context.Context) error {
	h := HealthHandler{
		services: map[string]Healthier{
			"ec2":   s.collectors.EC2,
			"asg":   s.collectors.ASG,
			"spot":  s.collectors.Spot,
			"nodes": s.collectors.Node,
			"pods":  s.collectors.Pod,

			"mainloop": s.mainloop,
		},
	}

	router := httprouter.New()
	router.GET("/", s.handleStatus)
	router.GET("/-/ready", webutil.HandleHealth)
	router.Handler("GET", "/-/healthy", h)
	router.Handler("GET", "/metrics", promhttp.Handler())

	return webutil.ListenAndServerWithContext(
		ctx, ":8080", router)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	lists := s.collectors.List(r.Context())
	s.renderStatus(w, r, lists)
}

func (s *Server) renderStatus(w http.ResponseWriter, r *http.Request, lists collectors.Lists) {
	data := struct {
		Lists             collectors.Lists
		CombinedInstances collectors.Instances
		CombinedPods      collectors.Pods
	}{}

	data.Lists = lists

	instances, pods := collectors.Combine(data.Lists)

	SortInstances(instances)
	SortPods(pods)

	data.CombinedInstances = instances.Select(collectors.HasEC2Data)
	data.CombinedPods = pods

	s.renderer.RespondHTML(w, r, "status.html", data)
}
