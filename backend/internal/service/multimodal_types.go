package service

import (
	"context"
	"io"
)

const EvidenceProcessingVersion = 3

type ModelProvenance struct {
	DetectorModel       string         `json:"detector_model,omitempty"`
	DetectorChecksum    string         `json:"detector_checksum,omitempty"`
	EmbeddingModel      string         `json:"embedding_model,omitempty"`
	EmbeddingChecksum   string         `json:"embedding_checksum,omitempty"`
	DiarizationModel    string         `json:"diarization_model,omitempty"`
	DiarizationChecksum string         `json:"diarization_checksum,omitempty"`
	SeparationModel     string         `json:"separation_model,omitempty"`
	SeparationChecksum  string         `json:"separation_checksum,omitempty"`
	Tracker             string         `json:"tracker,omitempty"`
	RuntimeVersion      string         `json:"runtime_version"`
	Configuration       map[string]any `json:"configuration_profile"`
	Device              string         `json:"device"`
}

type DensePersonAnalysisInput struct {
	RecordingID       string
	ProcessingVersion int
	FileName          string
	MediaType         string
	Video             io.Reader
	DetectorModel     string
	EmbeddingModel    string
	Profile           PersonTrackingProfile
}

type PersonTrackingProfile struct {
	FPS                           float64 `json:"fps"`
	ConfirmationDetections        int     `json:"confirmation_detections"`
	ConfirmationWindowFrames      int     `json:"confirmation_window_frames"`
	LostTimeoutSeconds            float64 `json:"lost_timeout_seconds"`
	ReidentificationWindowSeconds float64 `json:"reidentification_window_seconds"`
	HighConfidenceThreshold       float64 `json:"high_confidence_threshold"`
	LowConfidenceThreshold        float64 `json:"low_confidence_threshold"`
	IOUThreshold                  float64 `json:"iou_threshold"`
	AppearanceThreshold           float64 `json:"appearance_threshold"`
	MaxGallerySamples             int     `json:"max_gallery_samples"`
}

type DenseFaceObservation struct {
	ObservationID  string      `json:"observation_id"`
	FrameIndex     int         `json:"frame_index"`
	Timestamp      float64     `json:"timestamp"`
	Box            FaceBox     `json:"box"`
	Landmarks      [][]float64 `json:"landmarks"`
	DetectionScore float64     `json:"detection_score"`
	Quality        FaceQuality `json:"quality"`
	Pose           FacePose    `json:"pose"`
	Embedding      []float64   `json:"embedding,omitempty"`
	MouthVisible   bool        `json:"mouth_visible"`
	MouthActivity  float64     `json:"mouth_activity"`
}

type PersonTrackQuality struct {
	Mean               float64 `json:"mean"`
	Maximum            float64 `json:"maximum"`
	UsableObservations int     `json:"usable_observations"`
}

type DensePersonTrack struct {
	ID                     string                 `json:"id"`
	ProviderTrackReference string                 `json:"provider_track_reference"`
	LifecycleStatus        string                 `json:"lifecycle_status"`
	FirstFrame             int                    `json:"first_frame"`
	LastFrame              int                    `json:"last_frame"`
	StartTime              float64                `json:"start_time"`
	EndTime                float64                `json:"end_time"`
	ObservationCount       int                    `json:"observation_count"`
	TrackingConfidence     float64                `json:"tracking_confidence"`
	Quality                PersonTrackQuality     `json:"quality"`
	GalleryObservationIDs  []string               `json:"gallery_observation_ids"`
	Observations           []DenseFaceObservation `json:"observations"`
}

type DensePersonAnalysis struct {
	RecordingID       string             `json:"recording_id"`
	ProcessingVersion int                `json:"processing_version"`
	DurationSeconds   float64            `json:"duration_seconds"`
	AnalyzedFPS       float64            `json:"analyzed_fps"`
	Tracks            []DensePersonTrack `json:"tracks"`
	Provenance        ModelProvenance    `json:"provenance"`
	Warnings          []string           `json:"warnings"`
}

type DensePersonAnalyzer interface {
	AnalyzePeople(ctx context.Context, input DensePersonAnalysisInput) (DensePersonAnalysis, error)
	Validate(ctx context.Context) (ModelProvenance, error)
}

type DensePersonAnalysisJob struct {
	ID                int64
	AnalysisRunID     string
	RecordingID       string
	OwnerUserID       string
	ProcessingVersion int
	Attempts          int
	MaxAttempts       int
	FilePath          string
	FileName          string
	MediaType         string
}

type AudioAnalysisInput struct {
	RecordingID       string
	ProcessingVersion int
	FileName          string
	MediaType         string
	Audio             io.Reader
	Profile           AudioAnalysisProfile
}

type AudioAnalysisProfile struct {
	MaximumSpeakers             int     `json:"maximum_speakers"`
	MaximumOverlapWindowSeconds float64 `json:"maximum_overlap_window_seconds"`
	SeparationBudgetSeconds     float64 `json:"separation_budget_seconds"`
	SpeakerMatchThreshold       float64 `json:"speaker_match_threshold"`
	SpeakerMatchMargin          float64 `json:"speaker_match_margin"`
}

type AudioRegion struct {
	ID                     string   `json:"id"`
	StartTime              float64  `json:"start_time"`
	EndTime                float64  `json:"end_time"`
	Kind                   string   `json:"kind"`
	ActiveSpeakerLabels    []string `json:"active_speaker_labels"`
	ConcurrentSpeakerCount int      `json:"concurrent_speaker_count"`
	Overlap                bool     `json:"overlap"`
	DiarizationConfidence  *float64 `json:"diarization_confidence,omitempty"`
	Status                 string   `json:"status"`
}

type AnalyzedAudioSource struct {
	ID                   string   `json:"id"`
	AudioRegionID        string   `json:"audio_region_id"`
	SourceIndex          int      `json:"source_index"`
	DiarizationClusterID string   `json:"diarization_cluster_id,omitempty"`
	SeparationStatus     string   `json:"separation_status"`
	SeparationConfidence *float64 `json:"separation_confidence,omitempty"`
	ReconstructionScore  *float64 `json:"reconstruction_score,omitempty"`
	SpeakerMatchScore    *float64 `json:"speaker_match_score,omitempty"`
	SpeakerRunnerUpScore *float64 `json:"speaker_runner_up_score,omitempty"`
	AudioBase64          string   `json:"audio_base64,omitempty"`
}

type AudioAnalysis struct {
	RecordingID       string                `json:"recording_id"`
	ProcessingVersion int                   `json:"processing_version"`
	DurationSeconds   float64               `json:"duration_seconds"`
	Regions           []AudioRegion         `json:"regions"`
	Sources           []AnalyzedAudioSource `json:"sources"`
	Provenance        ModelProvenance       `json:"provenance"`
	Warnings          []string              `json:"warnings"`
}

type OverlapAudioAnalyzer interface {
	AnalyzeAudio(ctx context.Context, input AudioAnalysisInput) (AudioAnalysis, error)
	Validate(ctx context.Context) (ModelProvenance, error)
}

func enforceCompatibleSpeakerFields(transcript *Transcript) {
	if transcript == nil {
		return
	}
	for index := range transcript.Segments {
		segment := &transcript.Segments[index]
		if !segment.Overlap {
			if segment.SpeakerProfileID != "" && len(segment.SpeakerProfileIDs) == 0 {
				segment.SpeakerProfileIDs = []string{segment.SpeakerProfileID}
			}
			continue
		}
		unambiguous := segment.SeparationStatus == "accepted" &&
			len(segment.SpeakerProfileIDs) == 1 && len(segment.AmbiguityReasons) == 0
		if unambiguous {
			segment.SpeakerProfileID = segment.SpeakerProfileIDs[0]
			continue
		}
		segment.SpeakerRole = "unknown"
		segment.SpeakerProfileID = ""
		segment.PersonProfileID = ""
		segment.SpeakerName = ""
		segment.SpeakerRelationship = ""
		segment.SpeakerIdentityStatus = ""
	}
}
