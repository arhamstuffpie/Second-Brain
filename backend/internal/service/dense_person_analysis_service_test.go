package service

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type densePersonRepositoryStub struct {
	job       DensePersonAnalysisJob
	completed DensePersonAnalysis
	retries   int
}

func (r *densePersonRepositoryStub) ClaimDensePersonAnalysis(context.Context, string, time.Duration) (DensePersonAnalysisJob, bool, error) {
	return r.job, true, nil
}

func (r *densePersonRepositoryStub) CompleteDensePersonAnalysis(_ context.Context, _ DensePersonAnalysisJob, analysis DensePersonAnalysis) error {
	r.completed = analysis
	return nil
}

func (r *densePersonRepositoryStub) RetryDensePersonAnalysis(context.Context, DensePersonAnalysisJob, string, time.Time, bool) error {
	r.retries++
	return nil
}

type densePersonAnalyzerStub struct {
	input  DensePersonAnalysisInput
	result DensePersonAnalysis
}

func (a *densePersonAnalyzerStub) AnalyzePeople(_ context.Context, input DensePersonAnalysisInput) (DensePersonAnalysis, error) {
	a.input = input
	_, _ = io.Copy(io.Discard, input.Video)
	return a.result, nil
}

func (*densePersonAnalyzerStub) Validate(context.Context) (ModelProvenance, error) {
	return ModelProvenance{}, nil
}

type densePersonStoreStub struct{}

func (densePersonStoreStub) Save(context.Context, string, io.Reader) (StoredObject, error) {
	return StoredObject{}, nil
}

func (densePersonStoreStub) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewBufferString("video")), nil
}

func (densePersonStoreStub) Delete(context.Context, string) error { return nil }

func TestDensePersonAnalysisServiceProcessesDurableJob(t *testing.T) {
	repository := &densePersonRepositoryStub{job: DensePersonAnalysisJob{
		ID: 7, AnalysisRunID: "run-1", RecordingID: "recording-1", OwnerUserID: "owner-1",
		ProcessingVersion: 3, Attempts: 1, MaxAttempts: 5,
		FilePath: "recording.mp4", FileName: "recording.mp4", MediaType: "video/mp4",
	}}
	analyzer := &densePersonAnalyzerStub{result: DensePersonAnalysis{
		RecordingID: "recording-1", ProcessingVersion: 3, DurationSeconds: 2, AnalyzedFPS: 8,
		Tracks: []DensePersonTrack{{ID: "track-1"}, {ID: "track-2"}},
	}}
	configured, err := newDensePersonAnalysisService(
		repository, analyzer, densePersonStoreStub{},
		config.PersonTrackingConfig{
			DetectorModel: "opencv/yunet-2023mar", EmbeddingModel: "opencv/sface-2021dec",
			Timeout: time.Minute,
			Profile: config.PersonTrackingProfile{
				FPS: 8, ConfirmationDetections: 3, ConfirmationWindowFrames: 5,
				LostTimeoutSeconds: 1, ReidentificationWindowSeconds: 10,
				HighConfidenceThreshold: 0.8, LowConfidenceThreshold: 0.35,
				IOUThreshold: 0.2, AppearanceThreshold: 0.35, MaxGallerySamples: 5,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	processed, err := configured.ProcessNextDensePersonAnalysis(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processed || repository.retries != 0 || len(repository.completed.Tracks) != 2 {
		t.Fatalf("processed=%t retries=%d completed tracks=%d", processed, repository.retries, len(repository.completed.Tracks))
	}
	if analyzer.input.RecordingID != "recording-1" || analyzer.input.Profile.FPS != 8 ||
		analyzer.input.DetectorModel != "opencv/yunet-2023mar" || analyzer.input.EmbeddingModel != "opencv/sface-2021dec" {
		t.Fatalf("analyzer input = %+v", analyzer.input)
	}
}
