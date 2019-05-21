package fake

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
)

const (
	DrainStateInProgress = "in-progress"
	DrainStateDone       = "done"
)

type Drainer struct {
	LastDrained   string
	DrainCount    int
	DrainDuration time.Duration
	Clock         clock.Clock

	States map[string]string
}

func (d *Drainer) Drain(id string) (int, error) {
	d.States[id] = DrainStateInProgress
	d.Clock.Sleep(d.DrainDuration)
	d.States[id] = DrainStateDone
	return 1, nil
}

func (d *Drainer) assertState(t *testing.T, wantState string, want string) {
	t.Helper()

	haveIDs := []string{}
	for id, state := range d.States {
		if wantState == state {
			haveIDs = append(haveIDs, id)
		}
	}

	sort.Strings(haveIDs)

	have := strings.Join(haveIDs, ",")

	if want != have {
		t.Errorf("Got wrong IDs for given state '%s'. Want: %s. Have: %s", wantState, want, have)
	}
}

// Assert expects a comma-separated list of instance IDs for the inProgressIDs
// and doneIDs args. It checks whether all instance are in the wanted state.
// The IDs are sorted.
func (d *Drainer) Assert(t *testing.T, inProgressIDs string, doneIDs string) {
	t.Helper()
	d.assertState(t, DrainStateInProgress, inProgressIDs)
	d.assertState(t, DrainStateDone, doneIDs)
}
