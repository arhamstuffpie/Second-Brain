package temporal

import (
	"context"

	"github.com/arham/ai-second-brain/internal/service"
)

type Mock struct {
	Result service.TemporalAnalysis
	Err    error
}

func (m Mock) AnalyzeTemporal(context.Context, service.TemporalAnalysisInput) (service.TemporalAnalysis, error) {
	return m.Result, m.Err
}
func (Mock) Validate(context.Context) (service.ProviderMetadata, error) {
	return service.ProviderMetadata{Provider: "mock", Model: "deterministic-temporal-fixture", Version: "1"}, nil
}
func (Mock) Provider() string { return "mock" }
func (Mock) Model() string    { return "deterministic-temporal-fixture" }

var _ service.TemporalActivityAnalyzer = Mock{}
