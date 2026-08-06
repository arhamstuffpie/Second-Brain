package stt

import (
	"context"
	"fmt"
	"io"

	"github.com/arham/ai-second-brain/internal/service"
)

// MockTranscriber makes the whole queue pipeline runnable without external
// credentials. It treats UTF-8 file content as a transcript.
type MockTranscriber struct{}

func NewMock() *MockTranscriber           { return &MockTranscriber{} }
func (*MockTranscriber) Provider() string { return "mock" }
func (*MockTranscriber) Model() string    { return "mock-text-audio" }

func (*MockTranscriber) Transcribe(ctx context.Context, input service.TranscriptionInput) (service.Transcript, error) {
	content, err := io.ReadAll(input.Audio)
	if err != nil {
		return service.Transcript{}, fmt.Errorf("read mock audio: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return service.Transcript{}, err
	}
	text := string(content)
	return service.Transcript{
		Text: text, Duration: 1, Provider: "mock", Model: "mock-text-audio",
		Segments: []service.TranscriptSegment{{
			StartTime: 0, EndTime: 1, Speaker: "speaker", Text: text,
		}},
	}, nil
}

var _ service.Transcriber = (*MockTranscriber)(nil)
