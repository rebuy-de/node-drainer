package test_util

type MockDrainer struct {
	WasDrainCalled bool
	Names          []string
}

func NewMockDrainer() *MockDrainer {
	m := &MockDrainer{}
	m.WasDrainCalled = false
	return m
}

func (md *MockDrainer) Drain(nodeName string) error {
	md.WasDrainCalled = true
	md.Names = append(md.Names, nodeName)
	return nil
}
