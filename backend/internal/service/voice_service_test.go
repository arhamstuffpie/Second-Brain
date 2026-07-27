package service

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

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

type realtimeTestStore struct {
	saveCalls int
}

func (s *realtimeTestStore) Save(context.Context, string, io.Reader) (StoredAudio, error) {
	s.saveCalls++
	return StoredAudio{Path: "/tmp/chunk.webm", SizeBytes: 10}, nil
}
func (*realtimeTestStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (*realtimeTestStore) Delete(context.Context, string) error { return nil }

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
		repository, stubTranscriber{}, store, stubMemographClient{},
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
		repository, stubTranscriber{}, store, stubMemographClient{},
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
