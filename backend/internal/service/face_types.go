package service

import "context"

type FaceRecognizer interface {
	Recognize(ctx context.Context, input FaceRecognitionInput) (FaceRecognition, error)
	Validate(ctx context.Context) (FaceProviderMetadata, error)
	Provider() string
	Model() string
}

type FaceRecognitionInput struct {
	FileName   string
	MediaType  string
	Image      []byte
	SingleFace bool
}

type FaceBox struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type FaceQuality struct {
	Usable  bool     `json:"usable"`
	Reasons []string `json:"reasons"`
	Score   float64  `json:"score"`
}

type FacePose struct {
	Yaw    float64 `json:"yaw"`
	Pitch  float64 `json:"pitch"`
	Roll   float64 `json:"roll"`
	Bucket string  `json:"bucket"`
}

type DetectedFace struct {
	Box            FaceBox     `json:"box"`
	Landmarks      [][]float64 `json:"landmarks"`
	DetectionScore float64     `json:"detection_score"`
	Quality        FaceQuality `json:"quality"`
	Pose           FacePose    `json:"pose"`
	Embedding      []float64   `json:"embedding,omitempty"`
}

type FaceRecognition struct {
	Provider   string
	Detector   string
	Model      string
	Dimensions int
	Faces      []DetectedFace
}

type FaceProviderMetadata struct {
	Provider       string `json:"provider"`
	Detector       string `json:"detector"`
	DetectorSHA256 string `json:"detector_sha256"`
	Model          string `json:"model"`
	ModelSHA256    string `json:"model_sha256"`
	Dimensions     int    `json:"dimensions"`
}
