package testdata

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"runtime"
	"testing"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
)

func Default(t *testing.T) collectors.Lists {
	file := relpath(t, "fixtures", "default.json")
	content, err := ioutil.ReadFile(file)
	if err != nil {
		t.Fatalf("failed to read dummy data: %v", err)
	}

	result := collectors.Lists{}

	err = json.Unmarshal(content, &result)
	if err != nil {
		t.Fatalf("failed to unmarshal dummy data: %v", err)
	}

	return result
}

func relpath(t *testing.T, paths ...string) string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to determine relative path")
	}

	return path.Join(
		path.Dir(filename),
		path.Join(paths...),
	)
}
