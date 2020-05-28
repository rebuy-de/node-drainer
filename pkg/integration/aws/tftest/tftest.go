package tftest

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const (
	EnvEnable = "NODE_DRAINER_ACCEPTANCE_TEST"

	// EnvSkipDestroy is used to speed up test driven development.
	EnvSkipDestroy = "NODE_DRAINER_SKIP_DESTROY"
)

type Terraform interface {
	Create()
	Output(string) string
	Destroy()
}

type terraform struct {
	tb   testing.TB
	dir  string
	uuid uuid.UUID
}

func New(tb testing.TB, dir string) Terraform {
	if !hasTrueEnvVar(EnvEnable) {
		tb.Logf("Acceptance test disabled. Enable it by setting env var `%s` to `true`", EnvEnable)
		tb.SkipNow()
	}

	tf := new(terraform)
	tf.tb = tb
	tf.dir = dir
	tf.uuid = uuid.New()

	tf.run("init")

	return tf
}

func (tf *terraform) cmd(args ...string) *exec.Cmd {
	cmd := exec.Command("terraform", args...)
	cmd.Dir = tf.dir
	return cmd
}

func (tf *terraform) run(args ...string) {
	cmd := tf.cmd(args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		tf.tb.Fatal(err)
	}
}

func (tf *terraform) Create() {
	tf.run("apply", "-auto-approve")
}

func (tf *terraform) Output(name string) string {
	cmd := tf.cmd("output", name)
	bytes, err := cmd.Output()
	if err != nil {
		tf.tb.Fatal(err)
	}

	return strings.TrimSpace(string(bytes))
}

// Destroy deletes all Terraform resources. It should be used as deferrer and
// the deferrer should be defined before Create is called, so the resources get
// deleted, even if the creation fails.
func (tf *terraform) Destroy() {
	if hasTrueEnvVar(EnvSkipDestroy) {
		tf.tb.Logf("WARNING: Destroy has been skipped, because the %s env var is set", EnvSkipDestroy)
		return
	}

	tf.run("destroy", "-auto-approve")
}

func hasTrueEnvVar(name string) bool {
	return strings.ToLower(os.Getenv(name)) == "true"
}
