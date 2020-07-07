package testdata

import "github.com/rebuy-de/node-drainer/v2/pkg/collectors"

func Join(chunks []collectors.Instances) collectors.Instances {
	result := collectors.Instances{}

	for _, chunk := range chunks {
		result = append(result, chunk...)
	}

	return result
}
