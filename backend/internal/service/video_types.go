package service

import (
	"encoding/json"
	"errors"
	"io"
	"time"
)

var ErrNoAudioTrack = errors.New("video has no audio track")

type VideoIngestInput struct {
	OwnerUserID       string
	SessionID         string
	GroupID           string
	MemoryID          string
	DeviceID          string
	Location          string
	FileName          string
	MediaType         string
	StartOffset       float64
	DefaultConfidence *float64
	Content           io.Reader
}

type CreateVideoRecordingInput struct {
	OwnerUserID       string
	SessionID         string
	GroupID           string
	MemoryID          string
	DeviceID          string
	Location          string
	FileName          string
	FilePath          string
	StorageProvider   string
	StorageBucket     string
	SHA256            string
	MediaType         string
	SizeBytes         int64
	StartOffset       float64
	FrameInterval     float64
	DefaultConfidence *float64
}

type CreateRealtimeVideoChunkInput struct {
	OwnerUserID       string
	RealtimeSessionID string
	ClientChunkID     string
	FileName          string
	FilePath          string
	StorageProvider   string
	StorageBucket     string
	SHA256            string
	MediaType         string
	SizeBytes         int64
	IsFinal           bool
	DefaultConfidence *float64
}

type VideoRecording struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"session_id"`
	GroupID       string    `json:"group_id"`
	MemoryID      string    `json:"memory_id"`
	Status        string    `json:"status"`
	AudioStatus   string    `json:"audio_status"`
	VisualStatus  string    `json:"visual_status"`
	MergeStatus   string    `json:"merge_status"`
	FileName      string    `json:"file_name"`
	MediaType     string    `json:"media_type"`
	SizeBytes     int64     `json:"size_bytes"`
	ClientChunkID string    `json:"chunk_id,omitempty"`
	ChunkIndex    *int      `json:"chunk_index,omitempty"`
	StartTime     float64   `json:"start_time"`
	IsFinal       bool      `json:"is_final,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type VideoRecordingDetail struct {
	VideoRecording
	DeviceID            string               `json:"device_id,omitempty"`
	Location            string               `json:"location,omitempty"`
	STTProvider         string               `json:"stt_provider,omitempty"`
	STTModel            string               `json:"stt_model,omitempty"`
	SpeakerReferenceIDs []string             `json:"speaker_reference_ids"`
	VisualProvider      string               `json:"visual_provider,omitempty"`
	VisualModel         string               `json:"visual_model,omitempty"`
	Transcript          *Transcript          `json:"transcript,omitempty"`
	VisualAnalysis      *VisualAnalysis      `json:"visual_analysis,omitempty"`
	Episodes            []VideoEpisode       `json:"episodes"`
	LastError           string               `json:"last_error,omitempty"`
	UpdatedAt           time.Time            `json:"updated_at"`
	MediaAssetID        string               `json:"media_asset_id,omitempty"`
	ActualDuration      float64              `json:"actual_duration_seconds,omitempty"`
	ProcessingVersion   int                  `json:"processing_version"`
	AnalysisBatches     []VideoAnalysisBatch `json:"analysis_batches"`
}

type EvidencePlayback struct {
	MediaAssetID string    `json:"media_asset_id"`
	Timestamp    float64   `json:"timestamp"`
	URL          string    `json:"url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type StartVideoRealtimeSessionInput struct {
	OwnerUserID          string `json:"-"`
	MemoryID             string `json:"memory_id"`
	GroupID              string `json:"group_id,omitempty"`
	DeviceID             string `json:"device_id,omitempty"`
	Location             string `json:"location,omitempty"`
	ChunkDurationSeconds int    `json:"chunk_duration_seconds,omitempty"`
	FrameIntervalSeconds int    `json:"frame_interval_seconds,omitempty"`
}

type RealtimeVideoChunkInput struct {
	OwnerUserID       string
	SessionID         string
	ClientChunkID     string
	IsFinal           bool
	FileName          string
	MediaType         string
	DefaultConfidence *float64
	Content           io.Reader
}

type RealtimeVideoSession struct {
	ID                   string     `json:"id"`
	MemoryID             string     `json:"memory_id"`
	GroupID              string     `json:"group_id"`
	DeviceID             string     `json:"device_id,omitempty"`
	Location             string     `json:"location,omitempty"`
	ChunkDurationSeconds int        `json:"chunk_duration_seconds"`
	FrameIntervalSeconds int        `json:"frame_interval_seconds"`
	NextChunkIndex       int        `json:"next_chunk_index"`
	Status               string     `json:"status"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	StoppedAt            *time.Time `json:"stopped_at,omitempty"`
}

type RealtimeVideoSessionDetail struct {
	RealtimeVideoSession
	Progress RealtimeSessionProgress `json:"progress"`
	Chunks   []VideoRecording        `json:"chunks"`
}

type ExtractedAudio struct {
	FileName  string
	MediaType string
	Path      string
	Audio     io.ReadCloser
}

type VideoFrame struct {
	FrameID          string  `json:"frame_id"`
	Timestamp        float64 `json:"timestamp"`
	SelectionReason  string  `json:"selection_reason"`
	ImageQuality     float64 `json:"image_quality"`
	SourceAssetID    string  `json:"source_asset_id"`
	DerivedObjectKey string  `json:"derived_object_key,omitempty"`
	FileName         string  `json:"file_name,omitempty"`
	MediaType        string  `json:"media_type,omitempty"`
	Image            []byte  `json:"-"`
}

type FrameExtraction struct {
	DurationSeconds float64
	Frames          []VideoFrame
}

type VisualAnalysisInput struct {
	Frames         []VideoFrame
	WindowDuration float64
}

type DetectedObject struct {
	ObjectID   string   `json:"object_id"`
	Name       string   `json:"name"`
	Attributes []string `json:"attributes"`
	Position   string   `json:"position"`
	State      string   `json:"state"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type DetectedText struct {
	Text          string   `json:"text"`
	Surface       string   `json:"surface"`
	Region        string   `json:"region"`
	ReadingStatus string   `json:"reading_status"`
	Confidence    *float64 `json:"confidence,omitempty"`
}

type VisualScene struct {
	Setting    string `json:"setting"`
	Lighting   string `json:"lighting"`
	Foreground string `json:"foreground"`
	Background string `json:"background"`
}

type VisualPerson struct {
	VisualLabel          string   `json:"visual_label"`
	PersonTrackID        string   `json:"person_track_id,omitempty"`
	FaceIdentityOutcome  string   `json:"face_identity_outcome,omitempty"`
	Appearance           string   `json:"appearance"`
	Position             string   `json:"position"`
	Action               string   `json:"action"`
	PhysicalPresence     bool     `json:"physical_presence"`
	FaceVisible          bool     `json:"face_visible"`
	PersonProfileID      string   `json:"person_profile_id,omitempty"`
	PersonIdentityStatus string   `json:"person_identity_status,omitempty"`
	PersonName           string   `json:"person_name,omitempty"`
	FaceMatchConfidence  *float64 `json:"face_match_confidence,omitempty"`
	Confidence           *float64 `json:"confidence,omitempty"`
}

type VisualRelation struct {
	Source     string   `json:"source"`
	Predicate  string   `json:"predicate"`
	Target     string   `json:"target"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type VideoObservation struct {
	ObservationID    string           `json:"observation_id"`
	FrameID          string           `json:"frame_id"`
	StartTime        float64          `json:"start_time"`
	EndTime          float64          `json:"end_time"`
	SelectionReason  string           `json:"selection_reason"`
	DerivedObjectKey string           `json:"derived_object_key,omitempty"`
	Scene            VisualScene      `json:"scene"`
	People           []VisualPerson   `json:"people"`
	Objects          []DetectedObject `json:"objects"`
	TextDetected     []DetectedText   `json:"text_detected"`
	Relations        []VisualRelation `json:"relations"`
	Activity         string           `json:"activity"`
	LocationGuess    string           `json:"location_guess"`
	Summary          string           `json:"summary"`
	Confidence       *float64         `json:"confidence,omitempty"`
}

type VisualAnalysis struct {
	Observations      []VideoObservation `json:"observations"`
	Warning           string             `json:"warning,omitempty"`
	Provider          string             `json:"provider,omitempty"`
	Model             string             `json:"model,omitempty"`
	ProcessingVersion int                `json:"processing_version"`
}

type VideoAnalysisBatch struct {
	ID                string         `json:"id"`
	RecordingID       string         `json:"recording_id"`
	BatchIndex        int            `json:"batch_index"`
	StartTime         float64        `json:"start_time"`
	EndTime           float64        `json:"end_time"`
	Frames            []VideoFrame   `json:"frames"`
	Status            string         `json:"status"`
	Attempts          int            `json:"attempts"`
	Provider          string         `json:"provider,omitempty"`
	Model             string         `json:"model,omitempty"`
	Result            VisualAnalysis `json:"result,omitempty"`
	LastError         string         `json:"last_error,omitempty"`
	ProcessingVersion int            `json:"processing_version"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type VideoEpisodeDraft struct {
	BucketIndex          int                `json:"bucket_index"`
	StartTime            float64            `json:"start_time"`
	EndTime              float64            `json:"end_time"`
	Description          string             `json:"description"`
	VisualDescription    string             `json:"visual_description"`
	SpeechDescription    string             `json:"speech_description"`
	Location             string             `json:"location"`
	Confidence           *float64           `json:"confidence,omitempty"`
	Visual               []VideoObservation `json:"visual_observations"`
	EvidenceKind         string             `json:"evidence_kind"`
	SourceIdentity       string             `json:"source_identity"`
	ProcessingVersion    int                `json:"processing_version"`
	MediaAssetID         string             `json:"media_asset_id"`
	ObservationIDs       []string           `json:"observation_ids"`
	FrameIDs             []string           `json:"frame_ids"`
	SupportingEpisodeIDs []string           `json:"supporting_episode_ids"`
}

type VideoEpisode struct {
	ID                   string          `json:"id"`
	BucketIndex          int             `json:"bucket_index"`
	StartTime            float64         `json:"start_time"`
	EndTime              float64         `json:"end_time"`
	Description          string          `json:"description"`
	VisualDescription    string          `json:"visual_description,omitempty"`
	SpeechDescription    string          `json:"speech_description,omitempty"`
	Location             string          `json:"location,omitempty"`
	Confidence           *float64        `json:"confidence,omitempty"`
	Status               string          `json:"status"`
	Response             json.RawMessage `json:"memograph_response,omitempty"`
	LastError            string          `json:"last_error,omitempty"`
	EvidenceKind         string          `json:"evidence_kind"`
	ProcessingVersion    int             `json:"processing_version"`
	MediaAssetID         string          `json:"media_asset_id,omitempty"`
	ObservationIDs       []string        `json:"observation_ids"`
	FrameIDs             []string        `json:"frame_ids"`
	SupportingEpisodeIDs []string        `json:"supporting_episode_ids"`
}

type VideoJob struct {
	ID                   int64
	Kind                 string
	RecordingID          string
	OwnerUserID          string
	EpisodeID            string
	MemographSource      string
	SourceIdentity       string
	Attempts             int
	MaxAttempts          int
	FilePath             string
	FileName             string
	MediaType            string
	STTProvider          string
	STTModel             string
	VisualProvider       string
	VisualModel          string
	SessionID            string
	GroupID              string
	MemoryID             string
	DeviceID             string
	Location             string
	ClientChunkID        string
	MediaAssetID         string
	ActualDuration       float64
	StartOffset          float64
	FrameInterval        float64
	Transcript           Transcript
	VisualAnalysis       VisualAnalysis
	EpisodeVisual        []VideoObservation
	ObservationIDs       []string
	FrameIDs             []string
	SupportingEpisodeIDs []string
	Description          string
	VisualDescription    string
	SpeechDescription    string
	EpisodeStart         float64
	EpisodeEnd           float64
	Confidence           *float64
	GraphRevision        int64
	ProcessingVersion    int
}
