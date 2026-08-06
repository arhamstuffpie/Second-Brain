package service

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

type HealthRepository interface {
	Ping(ctx context.Context) error
}

type HealthService interface {
	Check(ctx context.Context) (Health, error)
}

type UserRepository interface {
	Create(ctx context.Context, email, passwordHash string) (StoredUser, bool, error)
	FindByEmail(ctx context.Context, email string) (StoredUser, bool, error)
}

type ModelProfileRepository interface {
	Get(ctx context.Context, ownerUserID, task string) (StoredModelProfile, bool, error)
	Upsert(ctx context.Context, profile StoredModelProfile) (StoredModelProfile, error)
	Delete(ctx context.Context, ownerUserID, task string) error
}

type CredentialCipher interface {
	Seal(plaintext string) (string, error)
	Open(ciphertext string) (string, error)
}

type AuthService interface {
	Signup(ctx context.Context, email, password string) (AuthResult, error)
	Login(ctx context.Context, email, password string) (AuthResult, error)
}

type ModelProfileService interface {
	GetTranscription(ctx context.Context, ownerUserID string) (ModelProfile, error)
	SaveTranscription(ctx context.Context, ownerUserID string, input ModelProfileInput) (ModelProfile, error)
	ResetTranscription(ctx context.Context, ownerUserID string) (ModelProfile, error)
}

type VoiceRepository interface {
	CreateEnrollmentSample(ctx context.Context, input CreateEnrollmentSampleInput) (VoiceEnrollmentRecord, error)
	ListEnrollmentSamples(ctx context.Context, ownerUserID string) ([]VoiceEnrollmentRecord, error)
	GetEnrollmentSample(ctx context.Context, id, ownerUserID string) (VoiceEnrollmentRecord, error)
	DeleteEnrollmentSample(ctx context.Context, id, ownerUserID string) (VoiceEnrollmentRecord, error)
	CreateRecording(ctx context.Context, input CreateRecordingInput, maxAttempts int) (VoiceRecording, error)
	FindRecordingByChunk(ctx context.Context, ownerUserID, sessionID string, chunkIndex int) (VoiceRecording, bool, error)
	GetRecording(ctx context.Context, id, ownerUserID string) (VoiceRecordingDetail, error)
	CreateRealtimeSession(ctx context.Context, input StartRealtimeSessionInput) (RealtimeVoiceSession, error)
	GetRealtimeSession(ctx context.Context, id, ownerUserID string) (RealtimeVoiceSessionDetail, error)
	StopRealtimeSession(ctx context.Context, id, ownerUserID string) (RealtimeVoiceSession, error)
	ClaimJob(ctx context.Context) (VoiceJob, bool, error)
	SaveTranscriptAndQueueAssembly(ctx context.Context, job VoiceJob, transcript Transcript, referenceIDs []string, provider, model string, maxAttempts int) error
	LoadAssembly(ctx context.Context, job VoiceJob) (VoiceAssemblySnapshot, error)
	SaveAssembledEpisodes(ctx context.Context, job VoiceJob, snapshot VoiceAssemblySnapshot, episodes []EpisodeDraft, maxAttempts int) error
	CompleteMemographEpisode(ctx context.Context, job VoiceJob, response json.RawMessage) error
	RetryJob(ctx context.Context, job VoiceJob, cause string, runAt time.Time, dead bool) error
}

type Transcriber interface {
	Transcribe(ctx context.Context, input TranscriptionInput) (Transcript, error)
	Provider() string
	Model() string
}

type SpeakerAttributor interface {
	Attribute(ctx context.Context, input SpeakerAttributionInput) (Transcript, error)
}

type AudioInspector interface {
	Duration(ctx context.Context, path string) (float64, error)
}

type AudioStore interface {
	Save(ctx context.Context, filename string, content io.Reader) (StoredAudio, error)
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
}

type VideoStore interface {
	Save(ctx context.Context, filename string, content io.Reader) (StoredAudio, error)
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
}

type VideoRepository interface {
	CreateVideoRecording(ctx context.Context, input CreateVideoRecordingInput, maxAttempts int) (VideoRecording, error)
	CreateRealtimeVideoChunk(ctx context.Context, input CreateRealtimeVideoChunkInput, maxAttempts int) (VideoRecording, error)
	FindVideoRecordingByClientChunk(ctx context.Context, ownerUserID, sessionID, clientChunkID string) (VideoRecording, bool, error)
	GetVideoRecording(ctx context.Context, id, ownerUserID string) (VideoRecordingDetail, error)
	CreateVideoRealtimeSession(ctx context.Context, input StartVideoRealtimeSessionInput) (RealtimeVideoSession, error)
	GetVideoRealtimeSession(ctx context.Context, id, ownerUserID string) (RealtimeVideoSessionDetail, error)
	StopVideoRealtimeSession(ctx context.Context, id, ownerUserID string) (RealtimeVideoSession, error)
	ClaimVideoJob(ctx context.Context) (VideoJob, bool, error)
	SaveVideoTranscript(ctx context.Context, job VideoJob, transcript Transcript, referenceIDs []string, provider, model string, maxAttempts int) error
	SaveVideoAnalysis(ctx context.Context, job VideoJob, analysis VisualAnalysis, provider, model string, maxAttempts int) error
	SaveVideoEpisodes(ctx context.Context, job VideoJob, episodes []VideoEpisodeDraft, maxAttempts int) error
	CompleteVideoMemographBranch(ctx context.Context, job VideoJob, response json.RawMessage) error
	RetryVideoJob(ctx context.Context, job VideoJob, cause string, runAt time.Time, dead bool) error
}

type MediaExtractor interface {
	ExtractAudio(ctx context.Context, videoPath string) (ExtractedAudio, error)
	ExtractFrames(ctx context.Context, videoPath string, interval time.Duration, maxFrames int) ([]VideoFrame, error)
}

type VisualAnalyzer interface {
	Analyze(ctx context.Context, input VisualAnalysisInput) (VisualAnalysis, error)
	Provider() string
	Model() string
}

type MemographClient interface {
	CreateMemory(ctx context.Context, projectID string, request MemoryCreateRequest) (json.RawMessage, error)
	InsertEpisode(ctx context.Context, memoryID string, request EpisodeInsertRequest) (json.RawMessage, error)
	Search(ctx context.Context, memoryID string, request MemorySearchRequest) (json.RawMessage, error)
	Answer(ctx context.Context, memoryID string, request MemoryAnswerRequest) (json.RawMessage, error)
	AnswerStream(ctx context.Context, memoryID string, request MemoryAnswerRequest) (MemoryAnswerStream, error)
	GetGraph(ctx context.Context, memoryID, groupID string) (json.RawMessage, error)
}

type VoiceService interface {
	EnrollVoice(ctx context.Context, input VoiceEnrollmentInput) (VoiceEnrollmentSample, error)
	ListVoiceEnrollments(ctx context.Context, ownerUserID string) ([]VoiceEnrollmentSample, error)
	DeleteVoiceEnrollment(ctx context.Context, id, ownerUserID string) error
	Ingest(ctx context.Context, input VoiceIngestInput) (VoiceRecording, error)
	GetRecording(ctx context.Context, id, ownerUserID string) (VoiceRecordingDetail, error)
	StartRealtimeSession(ctx context.Context, input StartRealtimeSessionInput) (RealtimeVoiceSession, error)
	IngestRealtimeChunk(ctx context.Context, input RealtimeChunkInput) (VoiceRecording, error)
	GetRealtimeSession(ctx context.Context, id, ownerUserID string) (RealtimeVoiceSessionDetail, error)
	StopRealtimeSession(ctx context.Context, id, ownerUserID string) (RealtimeVoiceSession, error)
	CreateMemory(ctx context.Context, projectID string, request MemoryCreateRequest) (json.RawMessage, error)
	Search(ctx context.Context, memoryID string, request MemorySearchRequest) (json.RawMessage, error)
	Answer(ctx context.Context, memoryID string, request MemoryAnswerRequest) (json.RawMessage, error)
	AnswerStream(ctx context.Context, memoryID string, request MemoryAnswerRequest) (MemoryAnswerStream, error)
	GetGraph(ctx context.Context, memoryID, groupID string) (json.RawMessage, error)
	ProcessNextJob(ctx context.Context) (bool, error)
}

type VideoService interface {
	IngestVideo(ctx context.Context, input VideoIngestInput) (VideoRecording, error)
	GetVideoRecording(ctx context.Context, id, ownerUserID string) (VideoRecordingDetail, error)
	StartVideoRealtimeSession(ctx context.Context, input StartVideoRealtimeSessionInput) (RealtimeVideoSession, error)
	IngestVideoRealtimeChunk(ctx context.Context, input RealtimeVideoChunkInput) (VideoRecording, error)
	GetVideoRealtimeSession(ctx context.Context, id, ownerUserID string) (RealtimeVideoSessionDetail, error)
	StopVideoRealtimeSession(ctx context.Context, id, ownerUserID string) (RealtimeVideoSession, error)
	ProcessNextVideoJob(ctx context.Context) (bool, error)
}

type Health struct {
	Status    string    `json:"status"`
	Database  string    `json:"database"`
	CheckedAt time.Time `json:"checked_at"`
}

type StoredUser struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AuthResult struct {
	User        User      `json:"user"`
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type StoredAudio struct {
	Path      string
	SizeBytes int64
}

type VoiceIngestInput struct {
	OwnerUserID       string
	SessionID         string
	GroupID           string
	MemoryID          string
	DeviceID          string
	Location          string
	FileName          string
	MediaType         string
	StartOffset       float64
	ChunkIndex        *int
	IsFinal           bool
	DefaultConfidence *float64
	Content           io.Reader
	BatchID           string
	BatchClosed       bool
}

type VoiceEnrollmentInput struct {
	OwnerUserID string
	FileName    string
	MediaType   string
	Content     io.Reader
}

type CreateEnrollmentSampleInput struct {
	OwnerUserID     string
	ProviderLabel   string
	FileName        string
	FilePath        string
	MediaType       string
	SizeBytes       int64
	DurationSeconds float64
}

type VoiceEnrollmentRecord struct {
	ID              string
	OwnerUserID     string
	Slot            int
	ProviderLabel   string
	FileName        string
	FilePath        string
	MediaType       string
	SizeBytes       int64
	DurationSeconds float64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type VoiceEnrollmentSample struct {
	ID              string    `json:"id"`
	FileName        string    `json:"file_name"`
	MediaType       string    `json:"media_type"`
	SizeBytes       int64     `json:"size_bytes"`
	DurationSeconds float64   `json:"duration_seconds"`
	CreatedAt       time.Time `json:"created_at"`
}

type CreateRecordingInput struct {
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
	ChunkIndex        *int
	IsFinal           bool
	DefaultConfidence *float64
	BatchID           string
	BatchClosed       bool
}

type VoiceRecording struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	GroupID    string    `json:"group_id"`
	MemoryID   string    `json:"memory_id"`
	Status     string    `json:"status"`
	FileName   string    `json:"file_name"`
	MediaType  string    `json:"media_type"`
	SizeBytes  int64     `json:"size_bytes"`
	ChunkIndex *int      `json:"chunk_index,omitempty"`
	IsFinal    bool      `json:"is_final,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type VoiceRecordingDetail struct {
	VoiceRecording
	DeviceID            string         `json:"device_id,omitempty"`
	Location            string         `json:"location,omitempty"`
	BatchID             string         `json:"batch_id"`
	STTProvider         string         `json:"stt_provider,omitempty"`
	STTModel            string         `json:"stt_model,omitempty"`
	SpeakerReferenceIDs []string       `json:"speaker_reference_ids"`
	Transcript          *Transcript    `json:"transcript,omitempty"`
	Episodes            []VoiceEpisode `json:"episodes"`
	LastError           string         `json:"last_error,omitempty"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type VoiceEpisode struct {
	ID                    string           `json:"id"`
	BucketIndex           int              `json:"bucket_index"`
	StartTime             float64          `json:"start_time"`
	EndTime               float64          `json:"end_time"`
	Description           string           `json:"description"`
	Confidence            *float64         `json:"confidence,omitempty"`
	Status                string           `json:"status"`
	Response              json.RawMessage  `json:"memograph_response,omitempty"`
	LastError             string           `json:"last_error,omitempty"`
	EpisodeIndex          int              `json:"episode_index"`
	Segments              []EpisodeSegment `json:"segments"`
	SourceRecordingIDs    []string         `json:"source_recording_ids"`
	OwnerUtteranceCount   int              `json:"owner_utterance_count"`
	OtherUtteranceCount   int              `json:"other_utterance_count"`
	UnknownUtteranceCount int              `json:"unknown_utterance_count"`
}

type TranscriptSegment struct {
	ID          string   `json:"id,omitempty"`
	StartTime   float64  `json:"start_time"`
	EndTime     float64  `json:"end_time"`
	Speaker     string   `json:"speaker"`
	SpeakerRole string   `json:"speaker_role"`
	Text        string   `json:"text"`
	Confidence  *float64 `json:"confidence,omitempty"`
}

type Transcript struct {
	Text              string              `json:"text"`
	Language          string              `json:"language,omitempty"`
	Duration          float64             `json:"duration"`
	Segments          []TranscriptSegment `json:"segments"`
	AudioTrackPresent *bool               `json:"audio_track_present,omitempty"`
	Warning           string              `json:"warning,omitempty"`
	Provider          string              `json:"-"`
	Model             string              `json:"-"`
}

type TranscriptionInput struct {
	OwnerUserID   string
	FileName      string
	MediaType     string
	Audio         io.Reader
	KnownSpeakers []KnownSpeakerReference
}

type StoredModelProfile struct {
	ID               string
	OwnerUserID      string
	Task             string
	Provider         string
	BaseURL          string
	Model            string
	APIKeyCiphertext string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ModelProfileInput struct {
	Provider    string `json:"provider"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	APIKey      string `json:"api_key"`
	ClearAPIKey bool   `json:"clear_api_key,omitempty"`
}

type ModelProfile struct {
	Task             string     `json:"task"`
	Provider         string     `json:"provider"`
	BaseURL          string     `json:"base_url"`
	Model            string     `json:"model"`
	APIKeyConfigured bool       `json:"api_key_configured"`
	Source           string     `json:"source"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
}

type KnownSpeakerReference struct {
	ID            string
	ProviderLabel string
	MediaType     string
	Audio         []byte
}

type SpeakerAttributionInput struct {
	Transcript Transcript
	References []KnownSpeakerReference
}

type EpisodeSegment struct {
	ID          string   `json:"id,omitempty"`
	RecordingID string   `json:"recording_id"`
	StartTime   float64  `json:"start_time"`
	EndTime     float64  `json:"end_time"`
	Speaker     string   `json:"speaker"`
	SpeakerRole string   `json:"speaker_role"`
	Text        string   `json:"text"`
	Confidence  *float64 `json:"confidence,omitempty"`
}

type EpisodeDraft struct {
	BucketIndex           int              `json:"bucket_index"`
	EpisodeIndex          int              `json:"episode_index"`
	StartTime             float64          `json:"start_time"`
	EndTime               float64          `json:"end_time"`
	Description           string           `json:"description"`
	Confidence            *float64         `json:"confidence,omitempty"`
	Segments              []EpisodeSegment `json:"segments"`
	SourceRecordingIDs    []string         `json:"source_recording_ids"`
	OwnerUtteranceCount   int              `json:"owner_utterance_count"`
	OtherUtteranceCount   int              `json:"other_utterance_count"`
	UnknownUtteranceCount int              `json:"unknown_utterance_count"`
}

type VoiceJob struct {
	ID                    int64
	Kind                  string
	RecordingID           string
	EpisodeID             string
	Attempts              int
	MaxAttempts           int
	FilePath              string
	FileName              string
	MediaType             string
	SessionID             string
	GroupID               string
	MemoryID              string
	DeviceID              string
	Location              string
	StartOffset           float64
	Description           string
	EpisodeStart          float64
	EpisodeEnd            float64
	Confidence            *float64
	BatchID               string
	OwnerUserID           string
	BatchClosed           bool
	EpisodeSegments       []EpisodeSegment
	SourceRecordingIDs    []string
	OwnerUtteranceCount   int
	OtherUtteranceCount   int
	UnknownUtteranceCount int
}

type StartRealtimeSessionInput struct {
	OwnerUserID          string `json:"-"`
	MemoryID             string `json:"memory_id"`
	GroupID              string `json:"group_id,omitempty"`
	DeviceID             string `json:"device_id,omitempty"`
	Location             string `json:"location,omitempty"`
	ChunkDurationSeconds int    `json:"chunk_duration_seconds,omitempty"`
}

type RealtimeChunkInput struct {
	OwnerUserID       string
	SessionID         string
	ChunkIndex        int
	IsFinal           bool
	FileName          string
	MediaType         string
	DefaultConfidence *float64
	Content           io.Reader
}

type RealtimeVoiceSession struct {
	ID                   string     `json:"id"`
	MemoryID             string     `json:"memory_id"`
	GroupID              string     `json:"group_id"`
	DeviceID             string     `json:"device_id,omitempty"`
	Location             string     `json:"location,omitempty"`
	ChunkDurationSeconds int        `json:"chunk_duration_seconds"`
	Status               string     `json:"status"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	StoppedAt            *time.Time `json:"stopped_at,omitempty"`
	BatchID              string     `json:"-"`
}

type RealtimeSessionProgress struct {
	Total            int `json:"total"`
	Queued           int `json:"queued"`
	Processing       int `json:"processing"`
	Completed        int `json:"completed"`
	Failed           int `json:"failed"`
	LatestChunkIndex int `json:"latest_chunk_index"`
}

type RealtimeVoiceSessionDetail struct {
	RealtimeVoiceSession
	Progress RealtimeSessionProgress `json:"progress"`
	Chunks   []VoiceRecording        `json:"chunks"`
	Episodes []VoiceEpisode          `json:"episodes"`
}

type AssemblyRecording struct {
	ID          string
	StartOffset float64
	ChunkIndex  *int
	Status      string
	Transcript  Transcript
}

type VoiceAssemblySnapshot struct {
	BatchID            string
	OwnerUserID        string
	SessionID          string
	GroupID            string
	MemoryID           string
	Location           string
	DeviceID           string
	Closed             bool
	AllSTTTerminal     bool
	TranscriptRevision int64
	Watermark          float64
	Recordings         []AssemblyRecording
}

type GraphConfig struct {
	Mode             string              `json:"mode"`
	Template         string              `json:"template,omitempty"`
	Instruction      string              `json:"instruction,omitempty"`
	EntityTypes      map[string]string   `json:"entity_types,omitempty"`
	EntityTypeColors map[string]string   `json:"entity_type_colors,omitempty"`
	EdgeTypes        map[string]string   `json:"edge_types,omitempty"`
	EdgeTypeMap      map[string][]string `json:"edge_type_map,omitempty"`
}

type CustomField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type MemoryCreateRequest struct {
	Name           string        `json:"name"`
	MemoryType     string        `json:"memory_type"`
	EmbeddingModel string        `json:"embedding_model,omitempty"`
	SecretID       string        `json:"secret_id,omitempty"`
	CustomFields   []CustomField `json:"custom_fields,omitempty"`
	GraphConfig    GraphConfig   `json:"graph_config"`
}

type EpisodeInsertRequest struct {
	Data            string           `json:"data"`
	Meta            map[string]any   `json:"meta"`
	StructuredGraph *StructuredGraph `json:"structured_graph,omitempty"`
	CustomFields    map[string]any   `json:"-"`
	IdempotencyKey  string           `json:"-"`
}

// StructuredGraph is Memograph's grounded episode contract. Canonical entity
// IDs make identity deterministic and bypass probabilistic entity extraction.
type StructuredGraph struct {
	EpisodeID  string                `json:"episode_id"`
	SceneID    string                `json:"scene_id,omitempty"`
	StartTime  float64               `json:"start_time"`
	EndTime    float64               `json:"end_time"`
	Summary    string                `json:"summary"`
	Location   string                `json:"location,omitempty"`
	Entities   []StructuredEntity    `json:"entities"`
	Relations  []StructuredRelation  `json:"relations"`
	Utterances []StructuredUtterance `json:"utterances"`
}

type StructuredEntity struct {
	CanonicalID string   `json:"canonical_id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Confidence  *float64 `json:"confidence,omitempty"`
}

type StructuredRelation struct {
	Source     string   `json:"source"`
	Predicate  string   `json:"predicate"`
	Target     string   `json:"target"`
	Fact       string   `json:"fact"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type StructuredUtterance struct {
	SpeakerID  string   `json:"speaker_id"`
	Speaker    string   `json:"speaker"`
	Text       string   `json:"text"`
	StartTime  float64  `json:"start_time"`
	EndTime    float64  `json:"end_time"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type MemorySearchRequest struct {
	Query   string         `json:"query"`
	Limit   int            `json:"limit,omitempty"`
	GroupID string         `json:"group_id,omitempty"`
	Filters map[string]any `json:"filters,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type MemoryAnswerRequest struct {
	Query    string         `json:"query,omitempty"`
	Messages []ChatMessage  `json:"messages,omitempty"`
	Stream   bool           `json:"stream,omitempty"`
	Limit    int            `json:"limit,omitempty"`
	Model    string         `json:"model,omitempty"`
	GroupID  string         `json:"group_id,omitempty"`
	Filters  map[string]any `json:"filters,omitempty"`
}

type MemoryAnswerStream struct {
	Body        io.ReadCloser
	ContentType string
}
