package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/testutil"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/testdata"
)

func TestServerRender(t *testing.T) {
	lists := testdata.Default()
	server := new(Server)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)

	server.renderStatus(w, r, lists)
	response := w.Result()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("got %s", response.Status)
	}

	buf := new(bytes.Buffer)

	_, err := buf.ReadFrom(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	testutil.AssertGolden(t, "test-fixtures/status-golden.html", buf.Bytes())
}
