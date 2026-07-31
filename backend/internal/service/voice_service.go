package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type voiceService struct {
	repository      VoiceRepository
	transcriber     Transcriber
	store           AudioStore
	memograph       MemographClient
	episodeDuration time.Duration
	maxAttempts     int
}

func newVoiceService(
	repository VoiceRepository,
	transcriber Transcriber,
	store AudioStore,
	memograph MemographClient,
	voiceConfig config.VoiceConfig,
	workerConfig config.WorkerConfig,
) *voiceService {
	return &voiceService{
		repository: repository, transcriber: transcriber, store: store, memograph: memograph,
		episodeDuration: voiceConfig.EpisodeDuration, maxAttempts: workerConfig.MaxAttempts,
	}
}

func (s *voiceService) Ingest(ctx context.Context, input VoiceIngestInput) (VoiceRecording, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.GroupID = strings.TrimSpace(input.GroupID)
	input.MemoryID = strings.TrimSpace(input.MemoryID)
	input.FileName = filepath.Base(strings.TrimSpace(input.FileName))
	input.MediaType = strings.TrimSpace(input.MediaType)
	if input.OwnerUserID == "" {
		return VoiceRecording{}, validation("owner_user_id", "is required")
	}
	if input.SessionID == "" {
		return VoiceRecording{}, validation("session_id", "is required")
	}
	if input.GroupID == "" {
		input.GroupID = input.SessionID
	}
	if input.MemoryID == "" {
		return VoiceRecording{}, validation("memory_id", "is required")
	}
	if input.FileName == "" || input.FileName == "." {
		return VoiceRecording{}, validation("file", "must have a filename")
	}
	if input.Content == nil {
		return VoiceRecording{}, validation("file", "is required")
	}
	if input.StartOffset < 0 {
		return VoiceRecording{}, validation("start_time", "must not be negative")
	}
	if input.ChunkIndex != nil && *input.ChunkIndex < 0 {
		return VoiceRecording{}, validation("chunk_index", "must not be negative")
	}
	if input.DefaultConfidence != nil && (*input.DefaultConfidence < 0 || *input.DefaultConfidence > 1) {
		return VoiceRecording{}, validation("confidence", "must be between 0 and 1")
	}
	if !supportedAudio(input.FileName, input.MediaType) {
		return VoiceRecording{}, validation("file", "must be flac, mp3, mp4, mpeg, mpga, m4a, ogg, wav, or webm audio")
	}
	if input.ChunkIndex != nil {
		existing, found, err := s.repository.FindRecordingByChunk(
			ctx, input.OwnerUserID, input.SessionID, *input.ChunkIndex,
		)
		if err != nil {
			return VoiceRecording{}, err
		}
		if found {
			return existing, nil
		}
	}

	stored, err := s.store.Save(ctx, input.FileName, input.Content)
	if err != nil {
		return VoiceRecording{}, validation("file", err.Error())
	}
	recording, err := s.repository.CreateRecording(ctx, CreateRecordingInput{
		OwnerUserID: input.OwnerUserID, SessionID: input.SessionID, GroupID: input.GroupID,
		MemoryID: input.MemoryID, DeviceID: strings.TrimSpace(input.DeviceID),
		Location: strings.TrimSpace(input.Location), FileName: input.FileName,
		FilePath: stored.Path, MediaType: input.MediaType, SizeBytes: stored.SizeBytes,
		StartOffset: input.StartOffset, ChunkIndex: input.ChunkIndex, IsFinal: input.IsFinal,
		DefaultConfidence: input.DefaultConfidence,
	}, s.maxAttempts)
	if err != nil {
		if input.ChunkIndex != nil {
			existing, found, findErr := s.repository.FindRecordingByChunk(
				ctx, input.OwnerUserID, input.SessionID, *input.ChunkIndex,
			)
			if findErr == nil && found {
				_ = s.store.Delete(context.Background(), stored.Path)
				return existing, nil
			}
		}
		_ = s.store.Delete(context.Background(), stored.Path)
		return VoiceRecording{}, fmt.Errorf("persist voice ingestion: %w", err)
	}
	return recording, nil
}

func (s *voiceService) GetRecording(ctx context.Context, id, ownerUserID string) (VoiceRecordingDetail, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(ownerUserID) == "" {
		return VoiceRecordingDetail{}, validation("recording_id", "is required")
	}
	return s.repository.GetRecording(ctx, id, ownerUserID)
}

func (s *voiceService) StartRealtimeSession(
	ctx context.Context,
	input StartRealtimeSessionInput,
) (RealtimeVoiceSession, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.MemoryID = strings.TrimSpace(input.MemoryID)
	input.GroupID = strings.TrimSpace(input.GroupID)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.Location = strings.TrimSpace(input.Location)
	if input.OwnerUserID == "" {
		return RealtimeVoiceSession{}, validation("owner_user_id", "is required")
	}
	if input.MemoryID == "" {
		return RealtimeVoiceSession{}, validation("memory_id", "is required")
	}
	if input.ChunkDurationSeconds == 0 {
		input.ChunkDurationSeconds = 30
	}
	if input.ChunkDurationSeconds < 5 || input.ChunkDurationSeconds > 300 {
		return RealtimeVoiceSession{}, validation("chunk_duration_seconds", "must be between 5 and 300")
	}
	return s.repository.CreateRealtimeSession(ctx, input)
}

func (s *voiceService) IngestRealtimeChunk(
	ctx context.Context,
	input RealtimeChunkInput,
) (VoiceRecording, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	if input.OwnerUserID == "" || input.SessionID == "" {
		return VoiceRecording{}, validation("session_id", "session_id and authenticated owner are required")
	}
	if input.ChunkIndex < 0 {
		return VoiceRecording{}, validation("chunk_index", "must not be negative")
	}
	session, err := s.repository.GetRealtimeSession(ctx, input.SessionID, input.OwnerUserID)
	if err != nil {
		return VoiceRecording{}, err
	}
	if session.Status != "active" {
		return VoiceRecording{}, fmt.Errorf("%w: realtime voice session is stopped", ErrConflict)
	}
	chunkIndex := input.ChunkIndex
	recording, err := s.Ingest(ctx, VoiceIngestInput{
		OwnerUserID:       input.OwnerUserID,
		SessionID:         input.SessionID,
		GroupID:           session.GroupID,
		MemoryID:          session.MemoryID,
		DeviceID:          session.DeviceID,
		Location:          session.Location,
		FileName:          input.FileName,
		MediaType:         input.MediaType,
		StartOffset:       float64(input.ChunkIndex * session.ChunkDurationSeconds),
		ChunkIndex:        &chunkIndex,
		IsFinal:           input.IsFinal,
		DefaultConfidence: input.DefaultConfidence,
		Content:           input.Content,
	})
	if err != nil {
		return VoiceRecording{}, err
	}
	if input.IsFinal {
		if _, stopErr := s.repository.StopRealtimeSession(ctx, input.SessionID, input.OwnerUserID); stopErr != nil {
			return VoiceRecording{}, stopErr
		}
	}
	return recording, nil
}

func (s *voiceService) GetRealtimeSession(
	ctx context.Context,
	id, ownerUserID string,
) (RealtimeVoiceSessionDetail, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(ownerUserID) == "" {
		return RealtimeVoiceSessionDetail{}, validation("session_id", "is required")
	}
	return s.repository.GetRealtimeSession(ctx, id, ownerUserID)
}

func (s *voiceService) StopRealtimeSession(
	ctx context.Context,
	id, ownerUserID string,
) (RealtimeVoiceSession, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(ownerUserID) == "" {
		return RealtimeVoiceSession{}, validation("session_id", "is required")
	}
	return s.repository.StopRealtimeSession(ctx, id, ownerUserID)
}

func (s *voiceService) CreateMemory(ctx context.Context, projectID string, request MemoryCreateRequest) (json.RawMessage, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, validation("project_id", "is required")
	}
	if strings.TrimSpace(request.Name) == "" {
		return nil, validation("name", "is required")
	}
	request.MemoryType = "graph"
	if request.EmbeddingModel == "" {
		request.EmbeddingModel = "text-embedding-3-small"
	}
	hasConfidence := false
	for _, field := range request.CustomFields {
		if strings.EqualFold(strings.TrimSpace(field.Name), "confidence") {
			hasConfidence = true
			break
		}
	}
	if !hasConfidence {
		request.CustomFields = append(request.CustomFields, CustomField{
			Name: "confidence", Type: "float", Description: "Speech-to-text confidence",
		})
	}
	if err := validateGraphConfig(request.GraphConfig); err != nil {
		return nil, err
	}
	result, err := s.memograph.CreateMemory(ctx, projectID, request)
	if err != nil {
		return nil, &UnavailableError{Dependency: "memograph", Cause: err}
	}
	return result, nil
}

func (s *voiceService) Search(ctx context.Context, memoryID string, request MemorySearchRequest) (json.RawMessage, error) {
	if strings.TrimSpace(memoryID) == "" || strings.TrimSpace(request.Query) == "" {
		return nil, validation("query", "memory_id and query are required")
	}
	if request.Limit <= 0 {
		request.Limit = 10
	}
	result, err := s.memograph.Search(ctx, memoryID, request)
	if err != nil {
		return nil, &UnavailableError{Dependency: "memograph", Cause: err}
	}
	return result, nil
}

func (s *voiceService) Answer(ctx context.Context, memoryID string, request MemoryAnswerRequest) (json.RawMessage, error) {
	if strings.TrimSpace(memoryID) == "" || (strings.TrimSpace(request.Query) == "" && len(request.Messages) == 0) {
		return nil, validation("query", "memory_id and query or messages are required")
	}
	if request.Limit <= 0 {
		request.Limit = 10
	}
	result, err := s.memograph.Answer(ctx, memoryID, request)
	if err != nil {
		return nil, &UnavailableError{Dependency: "memograph", Cause: err}
	}
	return result, nil
}

func (s *voiceService) AnswerStream(
	ctx context.Context,
	memoryID string,
	request MemoryAnswerRequest,
) (MemoryAnswerStream, error) {
	if strings.TrimSpace(memoryID) == "" ||
		(strings.TrimSpace(request.Query) == "" && len(request.Messages) == 0) {
		return MemoryAnswerStream{}, validation(
			"query",
			"memory_id and query or messages are required",
		)
	}
	if request.Limit <= 0 {
		request.Limit = 10
	}
	request.Stream = true
	result, err := s.memograph.AnswerStream(ctx, memoryID, request)
	if err != nil {
		return MemoryAnswerStream{}, &UnavailableError{Dependency: "memograph", Cause: err}
	}
	return result, nil
}

func (s *voiceService) GetGraph(ctx context.Context, memoryID, groupID string) (json.RawMessage, error) {
	if strings.TrimSpace(memoryID) == "" {
		return nil, validation("memory_id", "is required")
	}
	result, err := s.memograph.GetGraph(ctx, memoryID, groupID)
	if err != nil {
		return nil, &UnavailableError{Dependency: "memograph", Cause: err}
	}
	return result, nil
}

func (s *voiceService) ProcessNextJob(ctx context.Context) (bool, error) {
	job, found, err := s.repository.ClaimJob(ctx)
	if err != nil || !found {
		return found, err
	}
	switch job.Kind {
	case "stt":
		err = s.processSTT(ctx, job)
	case "memograph":
		err = s.processMemograph(ctx, job)
	default:
		err = fmt.Errorf("unsupported voice job kind %q", job.Kind)
	}
	if err == nil {
		return true, nil
	}
	dead := job.Attempts >= job.MaxAttempts
	delay := retryDelay(job.Attempts)
	retryCtx := ctx
	var cancel context.CancelFunc
	if ctx.Err() != nil {
		retryCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	if retryErr := s.repository.RetryJob(retryCtx, job, err.Error(), time.Now().UTC().Add(delay), dead); retryErr != nil {
		return true, fmt.Errorf("%v; persist retry: %w", err, retryErr)
	}
	return true, err
}

func (s *voiceService) processSTT(ctx context.Context, job VoiceJob) error {
	audio, err := s.store.Open(ctx, job.FilePath)
	if err != nil {
		return err
	}
	defer audio.Close()
	transcript, err := s.transcriber.Transcribe(ctx, TranscriptionInput{
		FileName: job.FileName, MediaType: job.MediaType, Audio: audio,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(transcript.Text) == "" || len(transcript.Segments) == 0 {
		return fmt.Errorf("transcription returned no speech")
	}
	episodes := BuildAudioEpisodes(transcript, s.episodeDuration, job.StartOffset, job.SessionID, job.Location)
	if len(episodes) == 0 {
		return fmt.Errorf("transcription produced no episodes")
	}
	if err := s.repository.SaveTranscriptAndEpisodes(
		ctx, job, transcript, episodes, s.transcriber.Provider(), s.transcriber.Model(), s.maxAttempts,
	); err != nil {
		return err
	}
	_ = s.store.Delete(context.Background(), job.FilePath)
	return nil
}

func (s *voiceService) processMemograph(ctx context.Context, job VoiceJob) error {
	meta := map[string]any{
		"source": "audio", "source_description": "voice recording",
		"session_id": job.SessionID, "group_id": job.GroupID,
		"start_time": job.EpisodeStart, "end_time": job.EpisodeEnd,
	}
	if job.Location != "" {
		meta["location"] = job.Location
	}
	if job.DeviceID != "" {
		meta["device_id"] = job.DeviceID
	}
	custom := make(map[string]any)
	if job.Confidence != nil {
		custom["confidence"] = *job.Confidence
	}
	result, err := s.memograph.InsertEpisode(ctx, job.MemoryID, EpisodeInsertRequest{
		Data: job.Description, Meta: meta, CustomFields: custom,
	})
	if err != nil {
		return err
	}
	return s.repository.CompleteMemographEpisode(ctx, job, result)
}

// BuildAudioEpisodes merges timestamped speech into fixed-duration memory
// buckets. Timestamps in the returned episodes include the chunk offset.
func BuildAudioEpisodes(
	transcript Transcript,
	duration time.Duration,
	offset float64,
	sessionID, location string,
) []EpisodeDraft {
	bucketSeconds := duration.Seconds()
	if bucketSeconds <= 0 {
		bucketSeconds = 30
	}
	buckets := make(map[int][]TranscriptSegment)
	for _, segment := range transcript.Segments {
		if strings.TrimSpace(segment.Text) == "" {
			continue
		}
		index := int(math.Floor(math.Max(segment.StartTime, 0) / bucketSeconds))
		buckets[index] = append(buckets[index], segment)
	}
	indices := make([]int, 0, len(buckets))
	for index := range buckets {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	episodes := make([]EpisodeDraft, 0, len(indices))
	for _, index := range indices {
		segments := buckets[index]
		start := offset + float64(index)*bucketSeconds
		end := start
		parts := make([]string, 0, len(segments))
		confidenceTotal := 0.0
		confidenceCount := 0
		for _, segment := range segments {
			segmentEnd := offset + segment.EndTime
			if segmentEnd > end {
				end = segmentEnd
			}
			speaker := strings.TrimSpace(segment.Speaker)
			if speaker == "" {
				speaker = "speaker"
			}
			parts = append(parts, fmt.Sprintf("%s said: %q", speaker, strings.TrimSpace(segment.Text)))
			if segment.Confidence != nil {
				confidenceTotal += *segment.Confidence
				confidenceCount++
			}
		}
		if maximum := start + bucketSeconds; end > maximum {
			end = maximum
		}
		if end < start {
			end = start
		}
		contextText := fmt.Sprintf("Audio memory from session %s between %.2fs and %.2fs.", sessionID, start, end)
		if location != "" {
			contextText += " Recorded at " + location + "."
		}
		description := contextText + " " + strings.Join(parts, " ")
		var confidence *float64
		if confidenceCount > 0 {
			value := confidenceTotal / float64(confidenceCount)
			confidence = &value
		}
		episodes = append(episodes, EpisodeDraft{
			BucketIndex: index, StartTime: start, EndTime: end,
			Description: description, Confidence: confidence,
		})
	}
	return episodes
}

func supportedAudio(filename, mediaType string) bool {
	extensions := map[string]bool{
		".flac": true, ".mp3": true, ".mp4": true, ".mpeg": true, ".mpga": true,
		".m4a": true, ".ogg": true, ".wav": true, ".webm": true,
	}
	if extensions[strings.ToLower(filepath.Ext(filename))] {
		return true
	}
	return strings.HasPrefix(strings.ToLower(mediaType), "audio/")
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := math.Pow(2, float64(attempt-1))
	if seconds > 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

func validateGraphConfig(config GraphConfig) error {
	switch config.Mode {
	case "template":
		if strings.TrimSpace(config.Template) == "" {
			return validation("graph_config.template", "is required in template mode")
		}
	case "instruction":
		if strings.TrimSpace(config.Instruction) == "" {
			return validation("graph_config.instruction", "is required in instruction mode")
		}
	case "custom":
		if len(config.EntityTypes) == 0 || len(config.EdgeTypes) == 0 {
			return validation("graph_config", "custom mode requires entity_types and edge_types")
		}
		for name, description := range config.EntityTypes {
			if strings.TrimSpace(name) == "" || strings.TrimSpace(description) == "" {
				return validation("graph_config.entity_types", "names and descriptions must not be empty")
			}
		}
		for name, description := range config.EdgeTypes {
			if strings.TrimSpace(name) == "" || strings.TrimSpace(description) == "" {
				return validation("graph_config.edge_types", "names and descriptions must not be empty")
			}
		}
		colorPattern := regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
		for _, color := range config.EntityTypeColors {
			if !colorPattern.MatchString(color) {
				return validation("graph_config.entity_type_colors", "colors must use #RRGGBB")
			}
		}
	default:
		return validation("graph_config.mode", "must be template, instruction, or custom")
	}
	return nil
}

func validation(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

var _ VoiceService = (*voiceService)(nil)
