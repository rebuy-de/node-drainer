package cmd

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"text/template"
	"time"

	"github.com/gobuffalo/packr/v2"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/webutil"

	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/ec2"
)

type Healthier interface {
	Healthy() bool
}

type HealthHandler struct {
	services map[string]Healthier
}

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

type Server struct {
	asg asg.Client
	ec2 ec2.Client

	mainloop *MainLoop
}

func (s *Server) Run(ctx context.Context) error {
	h := HealthHandler{
		services: map[string]Healthier{
			"ec2":      s.ec2,
			"asg":      s.asg,
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
	data := struct {
		ASGInstances []asg.Instance
		EC2Instances []ec2.Instance

		Combined aws.Instances
	}{}

	data.ASGInstances = s.asg.List()
	data.EC2Instances = s.ec2.List()
	data.Combined = aws.CombineInstances(
		data.ASGInstances, data.EC2Instances,
	).Select(aws.HasLifecycleMessage).
		Sort(aws.ByInstanceID).Sort(aws.ByLaunchTime).SortReverse(aws.ByTriggeredAt)

	s.respondTemplate(w, r, "status.html", data)
}

func (s *Server) respondTemplate(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	templateBox := packr.New("templates", "./templates")

	t := template.New("")

	t = t.Funcs(template.FuncMap{
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
			return t.Local().Format(format), nil
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
