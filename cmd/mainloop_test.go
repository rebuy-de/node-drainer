package cmd

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/testdata"
)

func TestMainLoopLifecycleCompletion(t *testing.T) {

	t.Run("NoComplete", func(t *testing.T) {
		b := testdata.NewBuilder()

		for _, ec2State := range testdata.AllEC2States {
			for _, asgState := range testdata.AllASGStates {
				if ec2State == testdata.EC2Running && asgState != testdata.ASGMissing {
					continue
				}

				b.AddInstance(1, testdata.InstanceTemplate{
					ASG:  asgState,
					EC2:  ec2State,
					Spot: testdata.SpotRunning,
					Node: testdata.NodeSchedulable,
					Name: fmt.Sprintf("%v-%v", ec2State, asgState),
				})
			}
		}

		collectors := testdata.GenerateCollectors(t, b.Build())
		collectors.ASG.(*testdata.ASGClientMock).On("Delete", mock.Anything, mock.Anything).Return(nil)

		ml := NewMainLoop(collectors)

		err := ml.runOnce(context.Background())
		require.NoError(t, err)

		collectors.ASG.(*testdata.ASGClientMock).AssertExpectations(t)
		collectors.ASG.(*testdata.ASGClientMock).AssertNotCalled(t, "Complete", mock.Anything, mock.Anything)
	})
}
