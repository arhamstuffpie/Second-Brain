package vision

import (
	"context"
	"fmt"

	"github.com/arham/ai-second-brain/internal/service"
)

type MockAnalyzer struct{}

func NewMock() *MockAnalyzer { return &MockAnalyzer{} }

func (m *MockAnalyzer) Provider() string { return "mock" }
func (m *MockAnalyzer) Model() string    { return "mock-vision-v1" }

func (m *MockAnalyzer) Analyze(
	_ context.Context,
	input service.VisualAnalysisInput,
) (service.VisualAnalysis, error) {
	if len(input.Frames) == 0 {
		return service.VisualAnalysis{}, fmt.Errorf("at least one video frame is required")
	}
	confidence := 0.5
	result := service.VisualAnalysis{
		Observations: make([]service.VideoObservation, 0, len(input.Frames)),
	}
	for index, frame := range input.Frames {
		end := frame.Timestamp + input.WindowDuration
		if index+1 < len(input.Frames) {
			end = input.Frames[index+1].Timestamp
		}
		if end <= frame.Timestamp {
			end = frame.Timestamp + 5
		}
		result.Observations = append(result.Observations, service.VideoObservation{
			StartTime:    frame.Timestamp,
			EndTime:      end,
			Objects:      []service.DetectedObject{},
			TextDetected: []service.DetectedText{},
			Activity:     "mock visual activity",
			Summary:      "A video frame was captured for local mock analysis.",
			Confidence:   &confidence,
		})
	}
	return result, nil
}

var _ service.VisualAnalyzer = (*MockAnalyzer)(nil)
