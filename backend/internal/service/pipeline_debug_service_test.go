package service

import (
	"context"
	"testing"
)

type pipelineDebugFaceStub struct{ input FaceRecognitionInput }

func (s *pipelineDebugFaceStub) Recognize(_ context.Context, input FaceRecognitionInput) (FaceRecognition, error) {
	s.input = input
	return FaceRecognition{Provider: "opencv", Detector: "yunet", Model: "sface", Dimensions: 2}, nil
}
func (*pipelineDebugFaceStub) Validate(context.Context) (FaceProviderMetadata, error) {
	return FaceProviderMetadata{}, nil
}
func (*pipelineDebugFaceStub) Provider() string { return "opencv" }
func (*pipelineDebugFaceStub) Model() string    { return "sface" }

func TestPipelineDebugFaceRunStaysLocal(t *testing.T) {
	face := &pipelineDebugFaceStub{}
	debug := newPipelineDebugService(face, nil, nil)
	run, err := debug.AnalyzeFace(context.Background(), PipelineDebugFile{
		FileName: "mark.jpg", MediaType: "image/jpeg", Content: []byte("image"),
	})
	if err != nil {
		t.Fatalf("AnalyzeFace() error = %v", err)
	}
	if run.Stage != "face" || run.MemographCalled || face.input.FileName != "mark.jpg" {
		t.Fatalf("debug run = %+v, input = %+v", run, face.input)
	}
	if providers := debug.Providers().Providers; len(providers) != 6 || providers[5].Enabled {
		t.Fatalf("providers = %+v", providers)
	}
}
