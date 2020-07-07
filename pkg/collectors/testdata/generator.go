package testdata

import (
	"fmt"
	"time"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
)

type TB interface {
}

type Option func(TB, collectors.Instance) collectors.Instance

type generator struct {
	n int
}

func (g *generator) next() int {
	n := g.n
	g.n++
	return n
}

func (g *generator) nextIntFmt(text string) string {
	return fmt.Sprintf(text, g.next())
}

func (g *generator) nextTime() time.Time {
	return time.
		Date(2020, time.July, 6, 16, 19, 0, 0, time.UTC).
		Add(time.Second * time.Duration(g.next()))
}
