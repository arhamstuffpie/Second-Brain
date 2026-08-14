package service

import "context"

type TemporalActivityAnalyzer interface {
	AnalyzeTemporal(ctx context.Context, input TemporalAnalysisInput) (TemporalAnalysis, error)
	Validate(ctx context.Context) (ProviderMetadata, error)
	Provider() string
	Model() string
}

type ActiveSpeakerDetector interface {
	DetectActiveSpeakers(ctx context.Context, input ActiveSpeakerInput) (ActiveSpeakerAnalysis, error)
	Validate(ctx context.Context) (ProviderMetadata, error)
	Provider() string
	Model() string
}

type ProviderMetadata struct {
	Provider string            `json:"provider"`
	Model    string            `json:"model"`
	Version  string            `json:"version"`
	Details  map[string]string `json:"details"`
}

type TemporalAnalysisInput struct {
	RecordingID       string       `json:"recording_id"`
	MediaAssetID      string       `json:"media_asset_id"`
	ProcessingVersion int          `json:"processing_version"`
	Frames            []VideoFrame `json:"frames"`
}

type TemporalPersonTrack struct {
	ID                 string   `json:"id"`
	StartTime          float64  `json:"start_time"`
	EndTime            float64  `json:"end_time"`
	TrackingConfidence float64  `json:"tracking_confidence"`
	EvidenceFrameIDs   []string `json:"evidence_frame_ids"`
	PhysicalPresence   bool     `json:"physical_presence"`
}

type TemporalObjectTrack struct {
	ID                 string   `json:"id"`
	Label              string   `json:"label"`
	StartTime          float64  `json:"start_time"`
	EndTime            float64  `json:"end_time"`
	TrackingConfidence float64  `json:"tracking_confidence"`
	EvidenceFrameIDs   []string `json:"evidence_frame_ids"`
}

type TemporalAnalysis struct {
	Provider     string                `json:"provider"`
	Model        string                `json:"model"`
	PersonTracks []TemporalPersonTrack `json:"person_tracks"`
	ObjectTracks []TemporalObjectTrack `json:"object_tracks"`
	Observations []TemporalObservation `json:"observations"`
	Warning      string                `json:"warning,omitempty"`
}

type ActiveSpeakerInput struct {
	RecordingID  string                `json:"recording_id"`
	Frames       []VideoFrame          `json:"frames"`
	PersonTracks []TemporalPersonTrack `json:"person_tracks"`
	Segments     []TranscriptSegment   `json:"segments"`
}

type ActiveSpeakerEvidence struct {
	PersonTrackID        string   `json:"person_track_id"`
	SegmentIDs           []string `json:"segment_ids"`
	Score                float64  `json:"score"`
	VisibleMouthCoverage float64  `json:"visible_mouth_coverage"`
	OverlappingConflict  bool     `json:"overlapping_conflict"`
	EvidenceFrameIDs     []string `json:"evidence_frame_ids"`
}

type ActiveSpeakerAnalysis struct {
	Provider string                  `json:"provider"`
	Model    string                  `json:"model"`
	Evidence []ActiveSpeakerEvidence `json:"evidence"`
	Warning  string                  `json:"warning,omitempty"`
}
