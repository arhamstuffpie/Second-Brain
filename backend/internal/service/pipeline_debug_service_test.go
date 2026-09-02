package service

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/arham/ai-second-brain/internal/config"
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
	debug := newPipelineDebugService(face, nil, nil, nil, nil, nil, nil, config.PersonTrackingConfig{})
	run, err := debug.AnalyzeFace(context.Background(), PipelineDebugFile{
		FileName: "mark.jpg", MediaType: "image/jpeg", Content: []byte("image"),
	})
	if err != nil {
		t.Fatalf("AnalyzeFace() error = %v", err)
	}
	if run.Stage != "face" || run.MemographCalled || face.input.FileName != "mark.jpg" {
		t.Fatalf("debug run = %+v, input = %+v", run, face.input)
	}
	if providers := debug.Providers().Providers; len(providers) != 7 || providers[6].Enabled {
		t.Fatalf("providers = %+v", providers)
	}
}

func TestDenseTrackMetricsExposeTrackingStability(t *testing.T) {
	track := PipelineDebugDenseTrack{StartTime: 0, EndTime: 1, Observations: []PipelineDebugDenseObservation{
		{Timestamp: 0, Box: FaceBox{Width: 20, Height: 20}, DetectionScore: .8,
			Pose: FacePose{Bucket: "frontal"}, MouthVisible: true, MouthActivity: .2,
			GallerySelected: true, Embedding: []float64{1, 0}},
		{Timestamp: .5, Box: FaceBox{X: 2, Width: 20, Height: 20}, DetectionScore: 1,
			Pose: FacePose{Bucket: "right_profile"}, MouthActivity: .6,
			GallerySelected: true, Embedding: []float64{.8, .6}},
	}}
	metrics := denseTrackMetrics(track)
	if metrics.DetectionMean != .9 || metrics.GalleryCoverage != 1 ||
		metrics.MouthVisibleCoverage != .5 || metrics.EmbeddingDimensions != 2 ||
		metrics.ObservationsPerSecond != 1 ||
		metrics.EmbeddingCosineMinimum == nil || *metrics.EmbeddingCosineMinimum != .8 ||
		metrics.PoseBuckets["frontal"] != 1 {
		t.Fatalf("dense metrics = %+v", metrics)
	}
}

func TestCropDebugFaceReturnsBoundedJPEG(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			frame.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var source bytes.Buffer
	if err := jpeg.Encode(&source, frame, nil); err != nil {
		t.Fatal(err)
	}
	cropped, err := cropDebugFace(source.Bytes(), FaceBox{X: 30, Y: 20, Width: 20, Height: 20})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(cropped))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() <= 20 || decoded.Bounds().Dx() >= 100 ||
		decoded.Bounds().Dy() <= 20 || decoded.Bounds().Dy() >= 80 {
		t.Fatalf("cropped bounds = %v", decoded.Bounds())
	}
}

func TestDisplayFaceBoxRotatesPhoneVideoCoordinates(t *testing.T) {
	box := displayFaceBox(
		FaceBox{X: 952, Y: 287, Width: 118, Height: 171},
		VideoOrientation{Width: 1280, Height: 720, Rotation: -90},
		720, 1280,
	)
	want := FaceBox{X: 262, Y: 952, Width: 171, Height: 118}
	if box != want {
		t.Fatalf("display face box = %+v, want %+v", box, want)
	}
}
