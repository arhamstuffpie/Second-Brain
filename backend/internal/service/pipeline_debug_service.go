package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type PipelineDebugService interface {
	Providers() PipelineDebugStatus
	Owners(context.Context) ([]PipelineDebugOwner, error)
	AnalysisOverview(context.Context, string) (PipelineDebugAnalysisOverview, error)
	AnalyzeFace(context.Context, PipelineDebugFile) (PipelineDebugRun, error)
	EmbedSpeaker(context.Context, PipelineDebugFile) (PipelineDebugRun, error)
	DetectActiveSpeaker(context.Context, PipelineDebugActiveSpeakerInput) (PipelineDebugRun, error)
	DenseOverview(context.Context, string) (PipelineDebugDenseOverview, error)
	DenseRecording(context.Context, string, string, int) (PipelineDebugDenseRecordingDetail, error)
	DenseFace(context.Context, string, string, string, string, int) (PipelineDebugImage, error)
}

type PipelineDebugRepository interface {
	PipelineDebugOwners(context.Context) ([]PipelineDebugOwner, error)
	PipelineDebugAnalysisOverview(context.Context, string) (PipelineDebugAnalysisOverview, error)
	DensePipelineDebugOverview(context.Context, string) (PipelineDebugDenseOverview, error)
	DensePipelineDebugRecording(context.Context, string, string, int) (PipelineDebugDenseRecordingDetail, error)
	DensePipelineDebugFaceSource(context.Context, string, string, string, string, int) (PipelineDebugDenseFaceSource, error)
}

type PipelineDebugFile struct {
	FileName  string
	MediaType string
	Content   []byte
}

type PipelineDebugActiveSpeakerInput struct {
	File         PipelineDebugFile
	RecordingID  string
	PersonTracks []TemporalPersonTrack
	Segments     []TranscriptSegment
}

type PipelineDebugProvider struct {
	Stage       string `json:"stage"`
	Enabled     bool   `json:"enabled"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	CostProfile string `json:"cost_profile"`
}

type PipelineDebugStatus struct {
	MemographCalled bool                    `json:"memograph_called"`
	Providers       []PipelineDebugProvider `json:"providers"`
}

type PipelineDebugOwner struct {
	ID             string     `json:"id"`
	Email          string     `json:"email"`
	RecordingCount int        `json:"recording_count"`
	RunCount       int        `json:"run_count"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
}

type PipelineDebugAnalysisStage struct {
	Stage            string         `json:"stage"`
	Required         bool           `json:"required"`
	Status           string         `json:"status"`
	Attempts         int            `json:"attempts"`
	MaxAttempts      int            `json:"max_attempts"`
	DependsOn        []string       `json:"depends_on"`
	Checkpoint       map[string]any `json:"checkpoint"`
	ResultProvenance map[string]any `json:"result_provenance"`
	LastError        string         `json:"last_error"`
	RunAt            time.Time      `json:"run_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type PipelineDebugAnalysisRun struct {
	ID                   string                       `json:"id"`
	RecordingID          string                       `json:"recording_id"`
	FileName             string                       `json:"file_name"`
	ProcessingVersion    int                          `json:"processing_version"`
	Status               string                       `json:"status"`
	Active               bool                         `json:"active"`
	ConfigurationProfile string                       `json:"configuration_profile"`
	LastError            string                       `json:"last_error"`
	CreatedAt            time.Time                    `json:"created_at"`
	UpdatedAt            time.Time                    `json:"updated_at"`
	Stages               []PipelineDebugAnalysisStage `json:"stages"`
}

type PipelineDebugAnalysisOverview struct {
	OwnerID string                     `json:"owner_id"`
	Runs    []PipelineDebugAnalysisRun `json:"runs"`
}

type PipelineDebugRun struct {
	RunID           string    `json:"run_id"`
	Stage           string    `json:"stage"`
	StartedAt       time.Time `json:"started_at"`
	DurationMS      int64     `json:"duration_ms"`
	MemographCalled bool      `json:"memograph_called"`
	Request         any       `json:"request"`
	Response        any       `json:"response"`
}

type PipelineDebugDenseWorker struct {
	Enabled         bool                      `json:"enabled"`
	Provider        string                    `json:"provider"`
	DetectorModel   string                    `json:"detector_model"`
	EmbeddingModel  string                    `json:"embedding_model"`
	Profile         PipelineDebugDenseProfile `json:"profile"`
	Jobs            map[string]int            `json:"jobs"`
	OldestQueuedAt  *time.Time                `json:"oldest_queued_at,omitempty"`
	LastCompletedAt *time.Time                `json:"last_completed_at,omitempty"`
}

type PipelineDebugDenseProfile struct {
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

type PipelineDebugDenseRecording struct {
	RecordingID       string         `json:"recording_id"`
	FileName          string         `json:"file_name"`
	ProcessingVersion int            `json:"processing_version"`
	RunStatus         string         `json:"run_status"`
	StageStatus       string         `json:"stage_status"`
	Attempts          int            `json:"attempts"`
	MaxAttempts       int            `json:"max_attempts"`
	LastError         string         `json:"last_error"`
	Checkpoint        map[string]any `json:"checkpoint"`
	ResultProvenance  map[string]any `json:"result_provenance"`
	TrackCount        int            `json:"track_count"`
	ObservationCount  int            `json:"observation_count"`
	GalleryCount      int            `json:"gallery_count"`
	EmbeddingCount    int            `json:"embedding_count"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type PipelineDebugDenseOverview struct {
	Worker     PipelineDebugDenseWorker      `json:"worker"`
	Recordings []PipelineDebugDenseRecording `json:"recordings"`
}

type PipelineDebugDenseTrackMetrics struct {
	DurationSeconds        float64        `json:"duration_seconds"`
	ObservationsPerSecond  float64        `json:"observations_per_second"`
	DetectionMinimum       float64        `json:"detection_minimum"`
	DetectionMean          float64        `json:"detection_mean"`
	DetectionMaximum       float64        `json:"detection_maximum"`
	GalleryCoverage        float64        `json:"gallery_coverage"`
	MouthVisibleCoverage   float64        `json:"mouth_visible_coverage"`
	MouthActivityMean      float64        `json:"mouth_activity_mean"`
	MaximumObservationGap  float64        `json:"maximum_observation_gap_seconds"`
	MeanConsecutiveBoxIOU  float64        `json:"mean_consecutive_box_iou"`
	EmbeddingCount         int            `json:"embedding_count"`
	EmbeddingDimensions    int            `json:"embedding_dimensions"`
	EmbeddingNormMean      float64        `json:"embedding_norm_mean"`
	EmbeddingCosineMinimum *float64       `json:"embedding_cosine_minimum,omitempty"`
	EmbeddingCosineMean    *float64       `json:"embedding_cosine_mean,omitempty"`
	PoseBuckets            map[string]int `json:"pose_buckets"`
}

type PipelineDebugDenseObservation struct {
	ObservationID       string      `json:"observation_id"`
	FrameIndex          int         `json:"frame_index"`
	Timestamp           float64     `json:"timestamp"`
	Box                 FaceBox     `json:"box"`
	Landmarks           [][]float64 `json:"landmarks"`
	DetectionScore      float64     `json:"detection_score"`
	Quality             FaceQuality `json:"quality"`
	Pose                FacePose    `json:"pose"`
	EmbeddingReference  string      `json:"embedding_reference,omitempty"`
	EmbeddingModel      string      `json:"embedding_model"`
	EmbeddingDimensions int         `json:"embedding_dimensions"`
	Embedding           []float64   `json:"embedding"`
	MouthVisible        bool        `json:"mouth_visible"`
	MouthActivity       float64     `json:"mouth_activity"`
	GallerySelected     bool        `json:"gallery_selected"`
	CreatedAt           time.Time   `json:"created_at"`
}

type PipelineDebugDenseTrack struct {
	ID                      string                          `json:"id"`
	ProviderTrackReference  string                          `json:"provider_track_reference"`
	TemporaryVisualLabel    string                          `json:"temporary_visual_label"`
	ResolvedPersonProfileID string                          `json:"resolved_person_profile_id,omitempty"`
	ResolvedPersonName      string                          `json:"resolved_person_name,omitempty"`
	ResolvedPersonStatus    string                          `json:"resolved_person_status,omitempty"`
	LifecycleStatus         string                          `json:"lifecycle_status"`
	FirstFrame              int                             `json:"first_frame"`
	LastFrame               int                             `json:"last_frame"`
	StartTime               float64                         `json:"start_time"`
	EndTime                 float64                         `json:"end_time"`
	ObservationCount        int                             `json:"observation_count"`
	TrackingConfidence      float64                         `json:"tracking_confidence"`
	Quality                 PersonTrackQuality              `json:"quality"`
	EvidenceFrameIDs        []string                        `json:"evidence_frame_ids"`
	ModelProvenance         ModelProvenance                 `json:"model_provenance"`
	Metrics                 PipelineDebugDenseTrackMetrics  `json:"metrics"`
	Observations            []PipelineDebugDenseObservation `json:"observations"`
	CreatedAt               time.Time                       `json:"created_at"`
	UpdatedAt               time.Time                       `json:"updated_at"`
}

type PipelineDebugFusionEvidence struct {
	ID                       string         `json:"id"`
	SegmentID                string         `json:"segment_id"`
	SegmentStartTime         float64        `json:"segment_start_time"`
	SegmentEndTime           float64        `json:"segment_end_time"`
	KnownVoiceName           string         `json:"known_voice_name"`
	VoiceSpeakerProfileID    string         `json:"voice_speaker_profile_id"`
	CanonicalPersonProfileID string         `json:"canonical_person_profile_id"`
	PersonTrackID            string         `json:"person_track_id,omitempty"`
	VoiceConfidence          float64        `json:"voice_confidence"`
	ActiveSpeakerScore       float64        `json:"active_speaker_score"`
	RunnerUpScore            float64        `json:"runner_up_score"`
	DecisionMargin           float64        `json:"decision_margin"`
	TemporalCoverage         float64        `json:"temporal_coverage"`
	MouthVisibleCoverage     float64        `json:"mouth_visible_coverage"`
	MouthActivity            float64        `json:"mouth_activity"`
	CombinedScore            float64        `json:"combined_score"`
	SupportingSegmentCount   int            `json:"supporting_segment_count"`
	Decision                 string         `json:"decision"`
	ConflictReasons          []string       `json:"conflict_reasons"`
	ModelProvenance          map[string]any `json:"model_provenance"`
	RawEvidence              map[string]any `json:"raw_evidence"`
	CreatedAt                time.Time      `json:"created_at"`
}

type PipelineDebugDenseRecordingDetail struct {
	Recording      PipelineDebugDenseRecording   `json:"recording"`
	VisualAnalysis VisualAnalysis                `json:"visual_analysis"`
	Tracks         []PipelineDebugDenseTrack     `json:"tracks"`
	FusionEvidence []PipelineDebugFusionEvidence `json:"fusion_evidence"`
}

type PipelineDebugDenseFaceSource struct {
	FilePath  string
	FileName  string
	MediaType string
	Timestamp float64
	Box       FaceBox
}

type PipelineDebugImage struct {
	MediaType string
	Content   []byte
}

type VideoOrientation struct {
	Width    int
	Height   int
	Rotation int
}

type pipelineDebugService struct {
	face          FaceRecognizer
	speaker       SpeakerEmbedder
	activeSpeaker ActiveSpeakerDetector
	dense         DensePersonAnalyzer
	repository    PipelineDebugRepository
	videoStore    VideoStore
	extractor     MediaExtractor
	denseConfig   config.PersonTrackingConfig
}

func newPipelineDebugService(
	face FaceRecognizer,
	speaker SpeakerEmbedder,
	activeSpeaker ActiveSpeakerDetector,
	dense DensePersonAnalyzer,
	repository PipelineDebugRepository,
	videoStore VideoStore,
	extractor MediaExtractor,
	denseConfig config.PersonTrackingConfig,
) *pipelineDebugService {
	return &pipelineDebugService{
		face: face, speaker: speaker, activeSpeaker: activeSpeaker, dense: dense,
		repository: repository, videoStore: videoStore, extractor: extractor, denseConfig: denseConfig,
	}
}

func (s *pipelineDebugService) Providers() PipelineDebugStatus {
	providers := []PipelineDebugProvider{
		{Stage: "face", Enabled: s.face != nil, CostProfile: "local"},
		{Stage: "dense_person_tracking", Enabled: s.dense != nil, CostProfile: "local"},
		{Stage: "speaker", Enabled: s.speaker != nil, CostProfile: "local"},
		{Stage: "active_speaker", Enabled: s.activeSpeaker != nil, CostProfile: "local"},
		{Stage: "stt", Enabled: false, Provider: "blocked in debug mode", CostProfile: "paid"},
		{Stage: "vision", Enabled: false, Provider: "blocked in debug mode", CostProfile: "paid"},
		{Stage: "memograph", Enabled: false, Provider: "never called by debug routes", CostProfile: "paid"},
	}
	if s.face != nil {
		providers[0].Provider, providers[0].Model = s.face.Provider(), s.face.Model()
	}
	if s.dense != nil {
		providers[1].Provider = s.denseConfig.Provider
		providers[1].Model = s.denseConfig.EmbeddingModel
	}
	if s.speaker != nil {
		providers[2].Provider = "speaker-embedding-http"
		if metadata, ok := s.speaker.(interface {
			Provider() string
			Model() string
		}); ok {
			providers[2].Provider, providers[2].Model = metadata.Provider(), metadata.Model()
		}
	}
	if s.activeSpeaker != nil {
		providers[3].Provider, providers[3].Model = s.activeSpeaker.Provider(), s.activeSpeaker.Model()
	}
	return PipelineDebugStatus{Providers: providers}
}

func (s *pipelineDebugService) Owners(ctx context.Context) ([]PipelineDebugOwner, error) {
	if s.repository == nil {
		return nil, fmt.Errorf("pipeline debug repository is unavailable")
	}
	return s.repository.PipelineDebugOwners(ctx)
}

func (s *pipelineDebugService) AnalysisOverview(ctx context.Context, ownerUserID string) (PipelineDebugAnalysisOverview, error) {
	if s.repository == nil {
		return PipelineDebugAnalysisOverview{}, fmt.Errorf("pipeline debug repository is unavailable")
	}
	return s.repository.PipelineDebugAnalysisOverview(ctx, ownerUserID)
}

func (s *pipelineDebugService) DenseOverview(ctx context.Context, ownerUserID string) (PipelineDebugDenseOverview, error) {
	if s.repository == nil {
		return PipelineDebugDenseOverview{}, fmt.Errorf("dense debug repository is unavailable")
	}
	overview, err := s.repository.DensePipelineDebugOverview(ctx, ownerUserID)
	if err != nil {
		return PipelineDebugDenseOverview{}, err
	}
	overview.Worker.Enabled = s.dense != nil
	overview.Worker.Provider = s.denseConfig.Provider
	overview.Worker.DetectorModel = s.denseConfig.DetectorModel
	overview.Worker.EmbeddingModel = s.denseConfig.EmbeddingModel
	overview.Worker.Profile = PipelineDebugDenseProfile{
		FPS:                           s.denseConfig.Profile.FPS,
		ConfirmationDetections:        s.denseConfig.Profile.ConfirmationDetections,
		ConfirmationWindowFrames:      s.denseConfig.Profile.ConfirmationWindowFrames,
		LostTimeoutSeconds:            s.denseConfig.Profile.LostTimeoutSeconds,
		ReidentificationWindowSeconds: s.denseConfig.Profile.ReidentificationWindowSeconds,
		HighConfidenceThreshold:       s.denseConfig.Profile.HighConfidenceThreshold,
		LowConfidenceThreshold:        s.denseConfig.Profile.LowConfidenceThreshold,
		IOUThreshold:                  s.denseConfig.Profile.IOUThreshold,
		AppearanceThreshold:           s.denseConfig.Profile.AppearanceThreshold,
		MaxGallerySamples:             s.denseConfig.Profile.MaxGallerySamples,
	}
	return overview, nil
}

func (s *pipelineDebugService) DenseRecording(
	ctx context.Context,
	ownerUserID, recordingID string,
	processingVersion int,
) (PipelineDebugDenseRecordingDetail, error) {
	if s.repository == nil {
		return PipelineDebugDenseRecordingDetail{}, fmt.Errorf("dense debug repository is unavailable")
	}
	detail, err := s.repository.DensePipelineDebugRecording(ctx, ownerUserID, recordingID, processingVersion)
	if err != nil {
		return PipelineDebugDenseRecordingDetail{}, err
	}
	for index := range detail.Tracks {
		detail.Tracks[index].Metrics = denseTrackMetrics(detail.Tracks[index])
	}
	return detail, nil
}

func (s *pipelineDebugService) DenseFace(
	ctx context.Context,
	ownerUserID, recordingID, trackID, observationID string,
	processingVersion int,
) (PipelineDebugImage, error) {
	if s.repository == nil || s.videoStore == nil || s.extractor == nil {
		return PipelineDebugImage{}, fmt.Errorf("dense face preview dependencies are unavailable")
	}
	source, err := s.repository.DensePipelineDebugFaceSource(
		ctx, ownerUserID, recordingID, trackID, observationID, processingVersion,
	)
	if err != nil {
		return PipelineDebugImage{}, err
	}
	path, cleanup, err := materializeObject(ctx, s.videoStore, source.FilePath, source.FileName)
	if err != nil {
		return PipelineDebugImage{}, err
	}
	defer cleanup()
	frames, err := s.extractor.ExtractFramesAt(ctx, path, []VideoFrame{{
		FrameID: observationID, Timestamp: source.Timestamp,
	}})
	if err != nil || len(frames) != 1 {
		if err == nil {
			err = fmt.Errorf("face preview extraction returned no frame")
		}
		return PipelineDebugImage{}, fmt.Errorf("extract dense face preview: %w", err)
	}
	orientation := VideoOrientation{}
	if probe, ok := s.extractor.(interface {
		ProbeVideoOrientation(context.Context, string) (VideoOrientation, error)
	}); ok {
		orientation, _ = probe.ProbeVideoOrientation(ctx, path)
	}
	content, err := cropDebugFaceOriented(frames[0].Image, source.Box, orientation)
	if err != nil {
		return PipelineDebugImage{}, err
	}
	return PipelineDebugImage{MediaType: "image/jpeg", Content: content}, nil
}

func denseTrackMetrics(track PipelineDebugDenseTrack) PipelineDebugDenseTrackMetrics {
	metrics := PipelineDebugDenseTrackMetrics{
		DurationSeconds: math.Max(0, track.EndTime-track.StartTime),
		PoseBuckets:     make(map[string]int), DetectionMinimum: 1,
	}
	if len(track.Observations) == 0 {
		metrics.DetectionMinimum = 0
		return metrics
	}
	var detectionTotal, mouthTotal, gapMaximum, iouTotal, normTotal float64
	var mouthVisible, galleryCount, iouCount int
	var previousEmbedding []float64
	var cosineMinimum, cosineTotal float64
	var cosineCount int
	for index, observation := range track.Observations {
		metrics.DetectionMinimum = math.Min(metrics.DetectionMinimum, observation.DetectionScore)
		metrics.DetectionMaximum = math.Max(metrics.DetectionMaximum, observation.DetectionScore)
		detectionTotal += observation.DetectionScore
		mouthTotal += observation.MouthActivity
		metrics.PoseBuckets[observation.Pose.Bucket]++
		if observation.MouthVisible {
			mouthVisible++
		}
		if observation.GallerySelected {
			galleryCount++
		}
		if len(observation.Embedding) > 0 {
			metrics.EmbeddingCount++
			metrics.EmbeddingDimensions = len(observation.Embedding)
			var squared float64
			for _, value := range observation.Embedding {
				squared += value * value
			}
			normTotal += math.Sqrt(squared)
			if len(previousEmbedding) == len(observation.Embedding) {
				score := cosine(previousEmbedding, observation.Embedding)
				if cosineCount == 0 || score < cosineMinimum {
					cosineMinimum = score
				}
				cosineTotal += score
				cosineCount++
			}
			previousEmbedding = observation.Embedding
		}
		if index > 0 {
			previous := track.Observations[index-1]
			gapMaximum = math.Max(gapMaximum, observation.Timestamp-previous.Timestamp)
			iouTotal += boxIOU(previous.Box, observation.Box)
			iouCount++
		}
	}
	count := float64(len(track.Observations))
	if len(track.Observations) > 1 {
		metrics.ObservationsPerSecond = float64(len(track.Observations)-1) / math.Max(metrics.DurationSeconds, 0.125)
	}
	metrics.DetectionMean = detectionTotal / count
	metrics.GalleryCoverage = float64(galleryCount) / count
	metrics.MouthVisibleCoverage = float64(mouthVisible) / count
	metrics.MouthActivityMean = mouthTotal / count
	metrics.MaximumObservationGap = gapMaximum
	if iouCount > 0 {
		metrics.MeanConsecutiveBoxIOU = iouTotal / float64(iouCount)
	}
	if metrics.EmbeddingCount > 0 {
		metrics.EmbeddingNormMean = normTotal / float64(metrics.EmbeddingCount)
	}
	if cosineCount > 0 {
		minimum, mean := cosineMinimum, cosineTotal/float64(cosineCount)
		metrics.EmbeddingCosineMinimum, metrics.EmbeddingCosineMean = &minimum, &mean
	}
	return metrics
}

func cropDebugFace(content []byte, box FaceBox) ([]byte, error) {
	return cropDebugFaceOriented(content, box, VideoOrientation{})
}

func cropDebugFaceOriented(content []byte, box FaceBox, orientation VideoOrientation) ([]byte, error) {
	decoded, err := jpeg.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("decode dense face preview: %w", err)
	}
	box = displayFaceBox(box, orientation, decoded.Bounds().Dx(), decoded.Bounds().Dy())
	padding := max(4, max(box.Width, box.Height)/8)
	region := image.Rect(
		box.X-padding, box.Y-padding,
		box.X+box.Width+padding, box.Y+box.Height+padding,
	).Intersect(decoded.Bounds())
	if region.Empty() {
		return nil, fmt.Errorf("dense face box is outside the extracted frame")
	}
	cropped := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(cropped, cropped.Bounds(), decoded, region.Min, draw.Src)
	var output bytes.Buffer
	if err := jpeg.Encode(&output, cropped, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode dense face preview: %w", err)
	}
	return output.Bytes(), nil
}

func displayFaceBox(box FaceBox, orientation VideoOrientation, displayWidth, displayHeight int) FaceBox {
	if orientation.Width < 1 || orientation.Height < 1 || displayWidth < 1 || displayHeight < 1 {
		return box
	}
	rotation := ((orientation.Rotation % 360) + 360) % 360
	expectedWidth, expectedHeight := orientation.Width, orientation.Height
	switch rotation {
	case 90:
		box = FaceBox{
			X: box.Y, Y: orientation.Width - box.X - box.Width,
			Width: box.Height, Height: box.Width,
		}
		expectedWidth, expectedHeight = orientation.Height, orientation.Width
	case 180:
		box.X = orientation.Width - box.X - box.Width
		box.Y = orientation.Height - box.Y - box.Height
	case 270:
		box = FaceBox{
			X: orientation.Height - box.Y - box.Height, Y: box.X,
			Width: box.Height, Height: box.Width,
		}
		expectedWidth, expectedHeight = orientation.Height, orientation.Width
	}
	return FaceBox{
		X:      int(math.Round(float64(box.X) * float64(displayWidth) / float64(expectedWidth))),
		Y:      int(math.Round(float64(box.Y) * float64(displayHeight) / float64(expectedHeight))),
		Width:  int(math.Round(float64(box.Width) * float64(displayWidth) / float64(expectedWidth))),
		Height: int(math.Round(float64(box.Height) * float64(displayHeight) / float64(expectedHeight))),
	}
}

func (s *pipelineDebugService) AnalyzeFace(ctx context.Context, file PipelineDebugFile) (PipelineDebugRun, error) {
	if s.face == nil {
		return PipelineDebugRun{}, fmt.Errorf("face recognition provider is disabled")
	}
	return debugRun("face", file, func() (any, error) {
		result, err := s.face.Recognize(ctx, FaceRecognitionInput{
			FileName: file.FileName, MediaType: file.MediaType, Image: file.Content,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"provider": result.Provider, "detector": result.Detector, "model": result.Model,
			"dimensions": result.Dimensions, "faces": result.Faces,
		}, nil
	})
}

func (s *pipelineDebugService) EmbedSpeaker(ctx context.Context, file PipelineDebugFile) (PipelineDebugRun, error) {
	if s.speaker == nil {
		return PipelineDebugRun{}, fmt.Errorf("speaker embedding provider is disabled")
	}
	return debugRun("speaker", file, func() (any, error) {
		result, err := s.speaker.Embed(ctx, SpeakerEmbeddingInput{
			FileName: file.FileName, MediaType: file.MediaType, Audio: file.Content,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"model": result.Model, "dimensions": len(result.Vector), "embedding": result.Vector,
		}, nil
	})
}

func (s *pipelineDebugService) DetectActiveSpeaker(ctx context.Context, input PipelineDebugActiveSpeakerInput) (PipelineDebugRun, error) {
	if s.activeSpeaker == nil {
		return PipelineDebugRun{}, fmt.Errorf("active-speaker provider is disabled")
	}
	if strings.TrimSpace(input.RecordingID) == "" || len(input.PersonTracks) == 0 || len(input.Segments) == 0 {
		return PipelineDebugRun{}, fmt.Errorf("recording_id, person_tracks, and segments are required")
	}
	ext := filepath.Ext(input.File.FileName)
	temporary, err := os.CreateTemp("", "pipeline-debug-*"+ext)
	if err != nil {
		return PipelineDebugRun{}, fmt.Errorf("create temporary video: %w", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if _, err = temporary.Write(input.File.Content); err == nil {
		err = temporary.Close()
	}
	if err != nil {
		_ = temporary.Close()
		return PipelineDebugRun{}, fmt.Errorf("store temporary video: %w", err)
	}
	started := time.Now().UTC()
	result, err := s.activeSpeaker.DetectActiveSpeakers(ctx, ActiveSpeakerInput{
		RecordingID: input.RecordingID, VideoPath: path, FileName: input.File.FileName,
		MediaType: input.File.MediaType, PersonTracks: input.PersonTracks, Segments: input.Segments,
	})
	if err != nil {
		return PipelineDebugRun{}, err
	}
	return PipelineDebugRun{
		RunID: debugRunID(), Stage: "active_speaker", StartedAt: started,
		DurationMS: time.Since(started).Milliseconds(),
		Request: map[string]any{
			"file": debugFileSummary(input.File), "recording_id": input.RecordingID,
			"person_tracks": input.PersonTracks, "segments": input.Segments,
		},
		Response: result,
	}, nil
}

func debugRun(stage string, file PipelineDebugFile, call func() (any, error)) (PipelineDebugRun, error) {
	started := time.Now().UTC()
	result, err := call()
	if err != nil {
		return PipelineDebugRun{}, err
	}
	return PipelineDebugRun{
		RunID: debugRunID(), Stage: stage, StartedAt: started,
		DurationMS: time.Since(started).Milliseconds(), Request: debugFileSummary(file), Response: result,
	}, nil
}

func debugFileSummary(file PipelineDebugFile) map[string]any {
	return map[string]any{"file_name": file.FileName, "media_type": file.MediaType, "size_bytes": len(file.Content)}
}

func debugRunID() string {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + hex.EncodeToString(bytes)
}

var _ PipelineDebugService = (*pipelineDebugService)(nil)
