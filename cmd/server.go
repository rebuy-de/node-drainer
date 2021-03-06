package cmd

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gobuffalo/packr/v2"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/webutil"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
)

// useUTC is used for showing dates to the user. On tests it should always use
// UTC, but generally it should use the servers time zone. Using the servers
// time zone is bad for generating golden files, because then the tests would
// depend on the developers machine settings.
var useUTC bool

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

	s.respondTemplate(w, r, "status.html", data)
}

func (s *Server) respondTemplate(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	templateBox := packr.New("templates", "./templates")

	t := template.New("")

	t = t.Funcs(template.FuncMap{
		"StringTitle": strings.Title,
		"PrettyTime": func(value interface{}) (string, error) {
			tPtr, ok := value.(*time.Time)
			if ok {
				if tPtr == nil {
					return "N/A", nil
				}
				value = *tPtr
			}

			t, ok := value.(time.Time)
			if !ok {
				return "", errors.Errorf("unexpected type")
			}

			if t.IsZero() {
				return "N/A", nil
			}

			format := "Mon, 2 Jan 15:04:05"

			if useUTC {
				t = t.UTC()
			} else {
				t = t.Local()
			}

			return t.Format(format), nil
		},
	})

	err := templateBox.Walk(func(name string, file packr.File) error {
		var err error
		t = t.New(name)
		t, err = t.Parse(file.String())
		return err
	})
	if webutil.RespondError(w, err) {
		return
	}

	w.Header().Set("Content-Type", "text/html")
	webutil.RespondError(w, t.ExecuteTemplate(w, name, data))
}
