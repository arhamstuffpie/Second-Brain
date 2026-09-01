package service

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type retentionVoiceRepository struct {
	stubVoiceRepository
	saved bool
}

func (r *retentionVoiceRepository) SaveTranscriptAndQueueAssembly(context.Context, VoiceJob, Transcript, []string, string, string, int) error {
	r.saved = true
	return nil
}

func TestVoiceProcessingRetainsOriginal(t *testing.T) {
	repository := &retentionVoiceRepository{}
	store := &realtimeTestStore{}
	voice := newVoiceService(
		repository, stubTranscriber{}, stubSpeakerAttributor{}, store, store,
		stubAudioInspector{}, stubMemographClient{}, config.VoiceConfig{},
		config.WorkerConfig{MaxAttempts: 5},
	)
	if err := voice.processSTT(context.Background(), VoiceJob{
		ID: 1, RecordingID: "recording-1", OwnerUserID: "owner-1",
		FilePath: "original.wav", FileName: "original.wav", MediaType: "audio/wav",
	}); err != nil {
		t.Fatal(err)
	}
	if !repository.saved || store.deleteCalls != 0 {
		t.Fatalf("saved = %v, original delete calls = %d", repository.saved, store.deleteCalls)
	}
}

func TestBuildAudioEpisodesBucketsSegmentsAndAppliesOffset(t *testing.T) {
	confidenceOne := 0.8
	confidenceTwo := 1.0
	transcript := Transcript{
		Text: "Hello. Ship it.",
		Segments: []TranscriptSegment{
			{StartTime: 1, EndTime: 3, Speaker: "A", Text: "Hello.", Confidence: &confidenceOne},
			{StartTime: 31, EndTime: 35, Speaker: "B", Text: "Ship it.", Confidence: &confidenceTwo},
		},
	}

	episodes := BuildAudioEpisodes(transcript, 30*time.Second, 60, "session-1", "office")

	if len(episodes) != 2 {
		t.Fatalf("len(episodes) = %d, want 2", len(episodes))
	}
	if episodes[0].StartTime != 60 || episodes[0].EndTime != 63 {
		t.Fatalf("first episode times = %.2f..%.2f, want 60..63", episodes[0].StartTime, episodes[0].EndTime)
	}
	if episodes[1].StartTime != 90 || episodes[1].EndTime != 95 {
		t.Fatalf("second episode times = %.2f..%.2f, want 90..95", episodes[1].StartTime, episodes[1].EndTime)
	}
	if !strings.Contains(episodes[0].Description, `A said: "Hello."`) ||
		!strings.Contains(episodes[0].Description, "Recorded at office") {
		t.Fatalf("first description = %q", episodes[0].Description)
	}
	if episodes[0].Confidence == nil || *episodes[0].Confidence != confidenceOne {
		t.Fatalf("first confidence = %v, want %.1f", episodes[0].Confidence, confidenceOne)
	}
}

func TestBuildConversationEpisodesBuffersActiveTailAndSeparatesAttribution(t *testing.T) {
	chunkZero, chunkOne := 0, 1
	snapshot := VoiceAssemblySnapshot{
		SessionID: "session-1", Location: "office", Watermark: 60,
		Recordings: []AssemblyRecording{
			{ID: "recording-0", ChunkIndex: &chunkZero, Transcript: Transcript{
				Duration: 30, Segments: []TranscriptSegment{{
					StartTime: 2, EndTime: 5, Speaker: "owner_ref",
					SpeakerRole: "owner", Text: "I prefer tea.",
				}},
			}},
			{ID: "recording-1", StartOffset: 30, ChunkIndex: &chunkOne, Transcript: Transcript{
				Duration: 5, Segments: []TranscriptSegment{{
					StartTime: 2, EndTime: 4, Speaker: "A",
					SpeakerRole: "other", Text: "I prefer coffee.",
				}},
			}},
		},
	}
	episodes := BuildConversationEpisodes(snapshot, 8*time.Second, 2*time.Minute)
	if len(episodes) != 1 {
		t.Fatalf("len(episodes) = %d, want 1 finalized episode", len(episodes))
	}
	if episodes[0].OwnerUtteranceCount != 1 || episodes[0].OtherUtteranceCount != 0 {
		t.Fatalf("attribution counts = %+v", episodes[0])
	}
	if !strings.Contains(episodes[0].Description, "Owner-attributed utterances") ||
		strings.Contains(episodes[0].Description, "I prefer coffee") {
		t.Fatalf("description = %q", episodes[0].Description)
	}

	snapshot.Closed = true
	snapshot.AllSTTTerminal = true
	episodes = BuildConversationEpisodes(snapshot, 8*time.Second, 2*time.Minute)
	if len(episodes) != 2 {
		t.Fatalf("closed len(episodes) = %d, want 2", len(episodes))
	}
	if episodes[1].OwnerUtteranceCount != 0 || episodes[1].OtherUtteranceCount != 1 ||
		!strings.Contains(episodes[1].Description, "must not be treated as owner statements") {
		t.Fatalf("non-owner episode = %+v", episodes[1])
	}
}

func TestBuildConversationEpisodesCombinesAcrossChunkBoundary(t *testing.T) {
	chunkZero, chunkOne := 0, 1
	snapshot := VoiceAssemblySnapshot{
		SessionID: "session-1", Closed: true, AllSTTTerminal: true,
		Recordings: []AssemblyRecording{
			{ID: "recording-0", ChunkIndex: &chunkZero, Transcript: Transcript{
				Duration: 30, Segments: []TranscriptSegment{{
					StartTime: 28, EndTime: 29.5, Speaker: "owner_ref",
					SpeakerRole: "owner", Text: "Let's ship it.",
				}},
			}},
			{ID: "recording-1", StartOffset: 30, ChunkIndex: &chunkOne, Transcript: Transcript{
				Duration: 30, Segments: []TranscriptSegment{{
					StartTime: 1, EndTime: 3, Speaker: "A",
					SpeakerRole: "other", Text: "Agreed.",
				}},
			}},
		},
	}
	episodes := BuildConversationEpisodes(snapshot, 8*time.Second, 2*time.Minute)
	if len(episodes) != 1 || len(episodes[0].SourceRecordingIDs) != 2 {
		t.Fatalf("episodes = %+v", episodes)
	}
	if episodes[0].StartTime != 28 || episodes[0].EndTime != 33 {
		t.Fatalf("episode times = %.1f..%.1f", episodes[0].StartTime, episodes[0].EndTime)
	}
}

func TestValidateGraphConfigModes(t *testing.T) {
	tests := []struct {
		name    string
		config  GraphConfig
		wantErr bool
	}{
		{name: "template", config: GraphConfig{Mode: "template", Template: "voice"}},
		{name: "instruction", config: GraphConfig{Mode: "instruction", Instruction: "Extract people and activities."}},
		{name: "custom", config: GraphConfig{
			Mode:             "custom",
			EntityTypes:      map[string]string{"Person": "A human"},
			EntityTypeColors: map[string]string{"Person": "#3B82F6"},
			EdgeTypes:        map[string]string{"MENTIONED": "Mention relationship"},
		}},
		{name: "bad color", config: GraphConfig{
			Mode:             "custom",
			EntityTypes:      map[string]string{"Person": "A human"},
			EntityTypeColors: map[string]string{"Person": "blue"},
			EdgeTypes:        map[string]string{"MENTIONED": "Mention relationship"},
		}, wantErr: true},
		{name: "missing instruction", config: GraphConfig{Mode: "instruction"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateGraphConfig(test.config)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateGraphConfig() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

type realtimeTestRepository struct {
	stubVoiceRepository
	session       RealtimeVoiceSessionDetail
	existing      VoiceRecording
	existingFound bool
	createInput   CreateRecordingInput
	createCalls   int
	stopped       bool
}

func (r *realtimeTestRepository) GetRealtimeSession(context.Context, string, string) (RealtimeVoiceSessionDetail, error) {
	return r.session, nil
}

func (r *realtimeTestRepository) FindRecordingByChunk(context.Context, string, string, int) (VoiceRecording, bool, error) {
	return r.existing, r.existingFound, nil
}

func (r *realtimeTestRepository) CreateRecording(_ context.Context, input CreateRecordingInput, _ int) (VoiceRecording, error) {
	r.createInput = input
	r.createCalls++
	return VoiceRecording{
		ID: "recording-1", SessionID: input.SessionID, GroupID: input.GroupID,
		MemoryID: input.MemoryID, ChunkIndex: input.ChunkIndex, IsFinal: input.IsFinal,
	}, nil
}

func (r *realtimeTestRepository) StopRealtimeSession(context.Context, string, string) (RealtimeVoiceSession, error) {
	r.stopped = true
	return RealtimeVoiceSession{Status: "stopped"}, nil
}

func TestVoiceIngestDefaultsToStableAccountGraphGroup(t *testing.T) {
	repository := &realtimeTestRepository{}
	store := &realtimeTestStore{}
	voice := newVoiceService(
		repository, stubTranscriber{}, stubSpeakerAttributor{}, store, store,
		stubAudioInspector{}, stubMemographClient{}, config.VoiceConfig{},
		config.WorkerConfig{MaxAttempts: 5},
	)
	_, err := voice.Ingest(context.Background(), VoiceIngestInput{
		OwnerUserID: "owner-1", SessionID: "session-1", MemoryID: "memory-1",
		FileName: "capture.wav", MediaType: "audio/wav",
		Content: strings.NewReader("audio"),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if repository.createInput.GroupID != "account-owner:owner-1" {
		t.Fatalf("default group = %q", repository.createInput.GroupID)
	}
}

type realtimeTestStore struct {
	saveCalls   int
	deleteCalls int
}

func (s *realtimeTestStore) Save(context.Context, string, io.Reader) (StoredAudio, error) {
	s.saveCalls++
	return StoredAudio{Path: "/tmp/chunk.webm", SizeBytes: 10}, nil
}
func (*realtimeTestStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (s *realtimeTestStore) Delete(context.Context, string) error {
	s.deleteCalls++
	return nil
}

type enrollmentTestRepository struct {
	stubVoiceRepository
	created   CreateEnrollmentSampleInput
	createErr error
}

func (r *enrollmentTestRepository) CreateEnrollmentSample(
	_ context.Context,
	input CreateEnrollmentSampleInput,
) (VoiceEnrollmentRecord, error) {
	r.created = input
	if r.createErr != nil {
		return VoiceEnrollmentRecord{}, r.createErr
	}
	return VoiceEnrollmentRecord{
		ID: "sample-1", FileName: input.FileName, MediaType: input.MediaType,
		SizeBytes: input.SizeBytes, DurationSeconds: input.DurationSeconds,
		CreatedAt: time.Unix(1, 0).UTC(), FilePath: input.FilePath,
	}, nil
}

type enrollmentInspector struct{ duration float64 }

func (i enrollmentInspector) Duration(context.Context, string) (float64, error) {
	return i.duration, nil
}

func TestEnrollVoiceValidatesDurationAndPersistsOpaqueReference(t *testing.T) {
	repository := &enrollmentTestRepository{}
	store := &realtimeTestStore{}
	voice := newVoiceService(
		repository, stubTranscriber{}, stubSpeakerAttributor{}, store, store,
		enrollmentInspector{duration: 5}, stubMemographClient{},
		config.VoiceConfig{
			EnrollmentMinDuration: 2 * time.Second,
			EnrollmentMaxDuration: 10 * time.Second,
		},
		config.WorkerConfig{MaxAttempts: 5},
	)
	result, err := voice.EnrollVoice(context.Background(), VoiceEnrollmentInput{
		OwnerUserID: "owner-1", FileName: "owner.wav",
		MediaType: "application/octet-stream", Content: strings.NewReader("audio"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "sample-1" || repository.created.DurationSeconds != 5 ||
		repository.created.MediaType != "audio/wav" ||
		!strings.HasPrefix(repository.created.ProviderLabel, "owner_") {
		t.Fatalf("result/input = %+v / %+v", result, repository.created)
	}
	if store.deleteCalls != 0 {
		t.Fatalf("valid sample was deleted %d times", store.deleteCalls)
	}
}

func TestEnrollVoiceDeletesRejectedSample(t *testing.T) {
	repository := &enrollmentTestRepository{}
	store := &realtimeTestStore{}
	voice := newVoiceService(
		repository, stubTranscriber{}, stubSpeakerAttributor{}, store, store,
		enrollmentInspector{duration: 1}, stubMemographClient{},
		config.VoiceConfig{
			EnrollmentMinDuration: 2 * time.Second,
			EnrollmentMaxDuration: 10 * time.Second,
		},
		config.WorkerConfig{MaxAttempts: 5},
	)
	_, err := voice.EnrollVoice(context.Background(), VoiceEnrollmentInput{
		OwnerUserID: "owner-1", FileName: "owner.wav",
		MediaType: "audio/wav", Content: strings.NewReader("audio"),
	})
	if err == nil || store.deleteCalls != 1 {
		t.Fatalf("error = %v, deleteCalls = %d", err, store.deleteCalls)
	}
}

func TestIngestRealtimeChunkUsesSessionAndDerivedOffset(t *testing.T) {
	repository := &realtimeTestRepository{session: RealtimeVoiceSessionDetail{
		RealtimeVoiceSession: RealtimeVoiceSession{
			ID: "session-1", MemoryID: "memory-1", GroupID: "user-1",
			DeviceID: "browser-mic", Location: "office",
			ChunkDurationSeconds: 30, Status: "active",
		},
	}}
	store := &realtimeTestStore{}
	voice := newVoiceService(
		repository, stubTranscriber{}, stubSpeakerAttributor{},
		store, store, stubAudioInspector{}, stubMemographClient{},
		config.VoiceConfig{EpisodeDuration: 30 * time.Second},
		config.WorkerConfig{MaxAttempts: 5},
	)

	result, err := voice.IngestRealtimeChunk(context.Background(), RealtimeChunkInput{
		OwnerUserID: "owner-1", SessionID: "session-1", ChunkIndex: 2,
		IsFinal: true, FileName: "chunk.webm", MediaType: "audio/webm",
		Content: strings.NewReader("audio"),
	})
	if err != nil {
		t.Fatalf("IngestRealtimeChunk() error = %v", err)
	}
	if repository.createInput.StartOffset != 60 {
		t.Fatalf("StartOffset = %.0f, want 60", repository.createInput.StartOffset)
	}
	if repository.createInput.GroupID != "user-1" ||
		repository.createInput.MemoryID != "memory-1" ||
		repository.createInput.DeviceID != "browser-mic" {
		t.Fatalf("CreateRecordingInput = %+v", repository.createInput)
	}
	if result.ChunkIndex == nil || *result.ChunkIndex != 2 || !result.IsFinal {
		t.Fatalf("result = %+v", result)
	}
	if !repository.stopped {
		t.Fatal("final chunk did not stop realtime session")
	}
}

func TestIngestRealtimeChunkIsIdempotent(t *testing.T) {
	index := 4
	repository := &realtimeTestRepository{
		session: RealtimeVoiceSessionDetail{RealtimeVoiceSession: RealtimeVoiceSession{
			ID: "session-1", MemoryID: "memory-1", GroupID: "session-1",
			ChunkDurationSeconds: 30, Status: "active",
		}},
		existing:      VoiceRecording{ID: "existing", ChunkIndex: &index},
		existingFound: true,
	}
	store := &realtimeTestStore{}
	voice := newVoiceService(
		repository, stubTranscriber{}, stubSpeakerAttributor{},
		store, store, stubAudioInspector{}, stubMemographClient{},
		config.VoiceConfig{EpisodeDuration: 30 * time.Second},
		config.WorkerConfig{MaxAttempts: 5},
	)

	result, err := voice.IngestRealtimeChunk(context.Background(), RealtimeChunkInput{
		OwnerUserID: "owner-1", SessionID: "session-1", ChunkIndex: index,
		FileName: "chunk.webm", MediaType: "audio/webm", Content: strings.NewReader("audio"),
	})
	if err != nil {
		t.Fatalf("IngestRealtimeChunk() error = %v", err)
	}
	if result.ID != "existing" || repository.createCalls != 0 || store.saveCalls != 0 {
		t.Fatalf("idempotent result = %+v, createCalls=%d saveCalls=%d", result, repository.createCalls, store.saveCalls)
	}
}

func TestEvidenceFirstFiltersPreservesExplicitSource(t *testing.T) {
	defaults := evidenceFirstFilters(nil)
	if len(defaults["source"].([]string)) != 2 {
		t.Fatalf("default filters = %+v", defaults)
	}
	explicit := evidenceFirstFilters(map[string]any{"source": "context_summary"})
	if explicit["source"] != "context_summary" {
		t.Fatalf("explicit source was replaced: %+v", explicit)
	}
}
