package active_speaker

import (
	"context"

	"github.com/arham/ai-second-brain/internal/service"
)

type Mock struct {
	Result service.ActiveSpeakerAnalysis
	Err    error
}

func (m Mock) DetectActiveSpeakers(context.Context, service.ActiveSpeakerInput) (service.ActiveSpeakerAnalysis, error) {
	return m.Result, m.Err
}
func (Mock) Validate(context.Context) (service.ProviderMetadata, error) {
	return service.ProviderMetadata{Provider: "mock", Model: "deterministic-active-speaker-fixture", Version: "1"}, nil
}
func (Mock) Provider() string { return "mock" }
func (Mock) Model() string    { return "deterministic-active-speaker-fixture" }

var _ service.ActiveSpeakerDetector = Mock{}
