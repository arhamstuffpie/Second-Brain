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
	DeviceID            string          `json:"device_id,omitempty"`
	Location            string          `json:"location,omitempty"`
	STTProvider         string          `json:"stt_provider,omitempty"`
	STTModel            string          `json:"stt_model,omitempty"`
	SpeakerReferenceIDs []string        `json:"speaker_reference_ids"`
	VisualProvider      string          `json:"visual_provider,omitempty"`
	VisualModel         string          `json:"visual_model,omitempty"`
	Transcript          *Transcript     `json:"transcript,omitempty"`
	VisualAnalysis      *VisualAnalysis `json:"visual_analysis,omitempty"`
	Episodes            []VideoEpisode  `json:"episodes"`
	LastError           string          `json:"last_error,omitempty"`
	UpdatedAt           time.Time       `json:"updated_at"`
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
	Audio     io.ReadCloser
}

type VideoFrame struct {
	Timestamp float64
	FileName  string
	MediaType string
	Image     []byte
}

type VisualAnalysisInput struct {
	Frames         []VideoFrame
	WindowDuration float64
}

type DetectedObject struct {
	Name       string   `json:"name"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type DetectedText struct {
	Text       string   `json:"text"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type VideoObservation struct {
	StartTime     float64          `json:"start_time"`
	EndTime       float64          `json:"end_time"`
	Objects       []DetectedObject `json:"objects"`
	TextDetected  []DetectedText   `json:"text_detected"`
	Activity      string           `json:"activity"`
	LocationGuess string           `json:"location_guess"`
	Summary       string           `json:"summary"`
	Confidence    *float64         `json:"confidence,omitempty"`
}

type VisualAnalysis struct {
	Observations []VideoObservation `json:"observations"`
}

type VideoEpisodeDraft struct {
	BucketIndex       int                `json:"bucket_index"`
	StartTime         float64            `json:"start_time"`
	EndTime           float64            `json:"end_time"`
	Description       string             `json:"description"`
	VisualDescription string             `json:"visual_description"`
	SpeechDescription string             `json:"speech_description"`
	Location          string             `json:"location"`
	Confidence        *float64           `json:"confidence,omitempty"`
	Visual            []VideoObservation `json:"visual_observations"`
}

type VideoEpisode struct {
	ID                string          `json:"id"`
	BucketIndex       int             `json:"bucket_index"`
	StartTime         float64         `json:"start_time"`
	EndTime           float64         `json:"end_time"`
	Description       string          `json:"description"`
	VisualDescription string          `json:"visual_description,omitempty"`
	SpeechDescription string          `json:"speech_description,omitempty"`
	Location          string          `json:"location,omitempty"`
	Confidence        *float64        `json:"confidence,omitempty"`
	Status            string          `json:"status"`
	Response          json.RawMessage `json:"memograph_response,omitempty"`
	LastError         string          `json:"last_error,omitempty"`
}

type VideoJob struct {
	ID                int64
	Kind              string
	RecordingID       string
	OwnerUserID       string
	EpisodeID         string
	MemographSource   string
	Attempts          int
	MaxAttempts       int
	FilePath          string
	FileName          string
	MediaType         string
	SessionID         string
	GroupID           string
	MemoryID          string
	DeviceID          string
	Location          string
	ClientChunkID     string
	StartOffset       float64
	FrameInterval     float64
	Transcript        Transcript
	VisualAnalysis    VisualAnalysis
	Description       string
	VisualDescription string
	SpeechDescription string
	EpisodeStart      float64
	EpisodeEnd        float64
	Confidence        *float64
}
