package cmd

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/testutil"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/webutil"
	"github.com/stretchr/testify/require"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
)

func init() {
	// Tests should always use UTC, because using the hosts time zone is bad
	// for generating golden files, since the tests would depend on the
	// developers' machine settings.
	os.Setenv("TZ", "")
}

type testUnique map[string]struct{}

func (u *testUnique) Check(t *testing.T, value string) {
	t.Helper()

	_, ok := (*u)[value]
	if ok {
		t.Errorf("%s appears more than once", value)
		return
	}

	(*u)[value] = struct{}{}
}

func TestServerRender(t *testing.T) {
	contents, err := ioutil.ReadFile("test-fixtures/status.json")
	require.NoError(t, err)

	lists := collectors.Lists{}
	err = json.Unmarshal(contents, &lists)
	require.NoError(t, err)

	t.Run("TestData", func(t *testing.T) {
		// Some additional checks to make sure the testdata makes sense. This
		// should make it less worrisome to add new data.

		var (
			uniqueEC2InstanceID  = testUnique{}
			uniqueEC2NodeName    = testUnique{}
			uniqueASGInstanceID  = testUnique{}
			uniqueSpotInstanceID = testUnique{}
			uniqueSpotRequest    = testUnique{}
			uniqueKubeInstanceID = testUnique{}
			uniqueKubeNodeName   = testUnique{}
			uniquePodName        = testUnique{}
		)

		for _, instance := range lists.ASG {
			uniqueASGInstanceID.Check(t, instance.ID)
		}

		for _, instance := range lists.EC2 {
			uniqueEC2InstanceID.Check(t, instance.InstanceID)
			uniqueEC2NodeName.Check(t, instance.NodeName)
		}

		for _, instance := range lists.Spot {
			uniqueSpotInstanceID.Check(t, instance.InstanceID)
			uniqueSpotRequest.Check(t, instance.RequestID)
		}

		for _, instance := range lists.Nodes {
			uniqueKubeInstanceID.Check(t, instance.InstanceID)
			uniqueKubeNodeName.Check(t, instance.NodeName)
		}

		for _, pod := range lists.Pods {
			uniquePodName.Check(t, path.Join(pod.Name, pod.Namespace))
		}

	})

	t.Run("StatusPage", func(t *testing.T) {
		server := new(Server)
		server.renderer = webutil.NewTemplateRenderer(&templates)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)

		server.renderStatus(w, r, lists)
		response := w.Result()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("got %s", response.Status)
		}

		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(response.Body)
		require.NoError(t, err)

		testutil.AssertGolden(t, "test-fixtures/status-golden.html", buf.Bytes())
	})
}
