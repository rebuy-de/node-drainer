package cmd

import (
	"context"
	"net/http"
	"text/template"
	"time"

	"github.com/gobuffalo/packr/v2"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/webutil"

	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/ec2"
)

type Server struct {
	asgHandler asg.Handler
	ec2Store   *ec2.Store
}

func (s *Server) Run(ctx context.Context) error {
	router := httprouter.New()
	router.GET("/", s.handleStatus)
	router.GET("/-/ready", webutil.HandleHealth)
	router.Handler("GET", "/metrics", promhttp.Handler())

	return webutil.ListenAndServerWithContext(
		ctx, ":8080", router)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	data := struct {
		ASGInstances []asg.Instance
		EC2Instances []ec2.Instance
	}{
		ASGInstances: s.asgHandler.List(),
		EC2Instances: s.ec2Store.List(),
	}

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

			format := "Mon, 2 Jan 15:04:05"
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
