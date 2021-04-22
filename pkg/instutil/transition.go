package instutil

import (
	"context"

	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/logutil"
	"github.com/sirupsen/logrus"
)

type contextKeyTransitionCollector string

type Transition struct {
	Name     string
	From, To string
	Fields   logrus.Fields
}

func GetTransitionCollector(ctx context.Context, name string) *TransitionCollector {
	cache, ok := ctx.Value(contextKeyTransitionCollector(name)).(*map[string]string)
	if !ok {
		logutil.Get(ctx).Warnf("transition collector with name '%s' not found", name)
		return nil
	}

	return &TransitionCollector{

		cache: cache,
	}
}

type TransitionCollector struct {
	cache   *map[string]string
	changes []Transition
}

func NewTransitionCollector(ctx context.Context, name string) context.Context {
	cache := map[string]string{}
	return context.WithValue(ctx, contextKeyTransitionCollector(name), &cache)
}

func (c *TransitionCollector) Observe(name string, state string, fields logrus.Fields) {
	if c == nil {
		return
	}

	c.changes = append(c.changes, Transition{
		Name:   name,
		To:     state,
		Fields: fields,
	})
}

func (c *TransitionCollector) Finish() []Transition {
	result := []Transition{}

	if c == nil {
		return result
	}

	observed := map[string]struct{}{}

	for _, change := range c.changes {
		from := (*c.cache)[change.Name]
		(*c.cache)[change.Name] = change.To
		observed[change.Name] = struct{}{}

		if from == change.To {
			continue
		}

		change.From = from
		result = append(result, change)
	}

	for name, value := range *c.cache {
		_, found := observed[name]
		if found {
			continue
		}

		result = append(result, Transition{
			Name: name,
			From: value,
		})

		delete(*c.cache, name)

	}

	return result
}
