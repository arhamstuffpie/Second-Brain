package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

func TestBuildVideoEpisodesMergesSpeechAndVisualContext(t *testing.T) {
	speechConfidence := 0.8
	visionConfidence := 0.9
	objectConfidence := 1.0
	transcript := Transcript{Segments: []TranscriptSegment{{
		StartTime: 2, EndTime: 7, Speaker: "A",
		Text: "Update the contract before Friday.", Confidence: &speechConfidence,
	}}}
	analysis := VisualAnalysis{Observations: []VideoObservation{{
		StartTime: 0, EndTime: 5,
		Objects:      []DetectedObject{{Name: "printed contract", Confidence: &objectConfidence}},
		TextDetected: []DetectedText{{Text: "Employment Agreement", Confidence: &visionConfidence}},
		Activity:     "reviewing a document", LocationGuess: "office meeting room",
		Summary: "Two people are reviewing a document.", Confidence: &visionConfidence,
	}}}

	episodes := BuildVideoEpisodes(
		transcript, analysis, 30*time.Second, 60, "session-1", "",
	)

	if len(episodes) != 1 {
		t.Fatalf("len(episodes) = %d, want 1", len(episodes))
	}
	episode := episodes[0]
	if episode.StartTime != 60 || episode.EndTime != 67 {
		t.Fatalf("episode times = %.2f..%.2f, want 60..67", episode.StartTime, episode.EndTime)
	}
	for _, expected := range []string{
		"Two people are reviewing a document.",
		"Objects visible: printed contract.",
		"Readable text: Employment Agreement.",
		`A said: "Update the contract before Friday."`,
	} {
		if !strings.Contains(episode.Description, expected) {
			t.Fatalf("description %q does not contain %q", episode.Description, expected)
		}
	}
	if episode.Location != "office meeting room" {
		t.Fatalf("location = %q, want office meeting room", episode.Location)
	}
	if len(episode.Visual) != 1 || episode.Visual[0].StartTime != 60 {
		t.Fatalf("absolute visual observations = %+v", episode.Visual)
	}
	if episode.Confidence == nil || *episode.Confidence <= 0.8 {
		t.Fatalf("confidence = %v, want merged confidence", episode.Confidence)
	}
}

func TestBuildVideoEpisodesSupportsVisualOnlyVideo(t *testing.T) {
	analysis := VisualAnalysis{Observations: []VideoObservation{{
		StartTime: 0, EndTime: 5, Activity: "walking",
		Summary: "A person walks through a hallway.",
	}}}
	episodes := BuildVideoEpisodes(
		Transcript{}, analysis, 30*time.Second, 0, "session-1", "office",
	)
	if len(episodes) != 1 {
		t.Fatalf("len(episodes) = %d, want 1", len(episodes))
	}
	if !strings.Contains(episodes[0].Description, "No speech was detected") {
		t.Fatalf("description = %q", episodes[0].Description)
	}
}

type videoRealtimeRepository struct {
	stubVideoRepository
	session       RealtimeVideoSessionDetail
	existing      VideoRecording
	existingFound bool
	createInput   CreateRealtimeVideoChunkInput
	createCalls   int
}

func (r *videoRealtimeRepository) FindVideoRecordingByClientChunk(
	context.Context, string, string, string,
) (VideoRecording, bool, error) {
	return r.existing, r.existingFound, nil
}

func (r *videoRealtimeRepository) GetVideoRealtimeSession(
	context.Context, string, string,
) (RealtimeVideoSessionDetail, error) {
	return r.session, nil
}

func (r *videoRealtimeRepository) CreateRealtimeVideoChunk(
	_ context.Context,
	input CreateRealtimeVideoChunkInput,
	_ int,
) (VideoRecording, error) {
	r.createInput = input
	r.createCalls++
	index := 3
	return VideoRecording{
		ID: "video-1", SessionID: input.RealtimeSessionID,
		ClientChunkID: input.ClientChunkID, ChunkIndex: &index, StartTime: 90,
	}, nil
}

func TestVideoRealtimeChunkUsesClientIDAndBackendAssignedIndex(t *testing.T) {
	repository := &videoRealtimeRepository{session: RealtimeVideoSessionDetail{
		RealtimeVideoSession: RealtimeVideoSession{
			ID: "session-1", Status: "active", ChunkDurationSeconds: 30,
		},
	}}
	store := &realtimeTestStore{}
	video := newVideoService(
		repository, stubTranscriber{}, store, noAudioExtractor{},
		stubVisualAnalyzer{}, stubMemographClient{},
		config.VideoConfig{
			EpisodeDuration: 30 * time.Second, FrameInterval: 5 * time.Second, MaxFrames: 6,
		},
		config.WorkerConfig{MaxAttempts: 5},
	)
	chunkID := "550e8400-e29b-41d4-a716-446655440000"
	result, err := video.IngestVideoRealtimeChunk(
		context.Background(),
		RealtimeVideoChunkInput{
			OwnerUserID: "owner-1", SessionID: "session-1", ClientChunkID: chunkID,
			FileName: "chunk.webm", MediaType: "video/webm",
			Content: strings.NewReader("video"),
		},
	)
	if err != nil {
		t.Fatalf("IngestVideoRealtimeChunk() error = %v", err)
	}
	if repository.createCalls != 1 || repository.createInput.ClientChunkID != chunkID {
		t.Fatalf("create calls/input = %d/%+v", repository.createCalls, repository.createInput)
	}
	if result.ChunkIndex == nil || *result.ChunkIndex != 3 || result.StartTime != 90 {
		t.Fatalf("result = %+v, want backend-assigned index 3 and start 90", result)
	}
}

func TestVideoRealtimeChunkRetryReturnsExistingRecording(t *testing.T) {
	existing := VideoRecording{ID: "existing", ClientChunkID: "550e8400-e29b-41d4-a716-446655440000"}
	repository := &videoRealtimeRepository{existing: existing, existingFound: true}
	store := &realtimeTestStore{}
	video := newVideoService(
		repository, stubTranscriber{}, store, noAudioExtractor{},
		stubVisualAnalyzer{}, stubMemographClient{},
		config.VideoConfig{
			EpisodeDuration: 30 * time.Second, FrameInterval: 5 * time.Second, MaxFrames: 6,
		},
		config.WorkerConfig{MaxAttempts: 5},
	)
	result, err := video.IngestVideoRealtimeChunk(context.Background(), RealtimeVideoChunkInput{
		OwnerUserID: "owner-1", SessionID: "session-1",
		ClientChunkID: "550e8400-e29b-41d4-a716-446655440000",
	})
	if err != nil {
		t.Fatalf("IngestVideoRealtimeChunk() error = %v", err)
	}
	if result.ID != "existing" || store.saveCalls != 0 || repository.createCalls != 0 {
		t.Fatalf("retry result/store/create = %+v/%d/%d", result, store.saveCalls, repository.createCalls)
	}
}

type noAudioExtractor struct{}

func (noAudioExtractor) ExtractAudio(context.Context, string) (ExtractedAudio, error) {
	return ExtractedAudio{}, ErrNoAudioTrack
}
func (noAudioExtractor) ExtractFrames(context.Context, string, time.Duration, int) ([]VideoFrame, error) {
	return nil, errors.New("not used")
}

type transcriptCaptureRepository struct {
	stubVideoRepository
	saved      bool
	transcript Transcript
}

func (r *transcriptCaptureRepository) SaveVideoTranscript(
	_ context.Context,
	_ VideoJob,
	transcript Transcript,
	_, _ string,
	_ int,
) error {
	r.saved = true
	r.transcript = transcript
	return nil
}

func TestVideoAudioJobAllowsVideoWithoutAudioTrack(t *testing.T) {
	repository := &transcriptCaptureRepository{}
	video := newVideoService(
		repository, stubTranscriber{}, stubAudioStore{}, noAudioExtractor{},
		stubVisualAnalyzer{}, stubMemographClient{},
		config.VideoConfig{
			EpisodeDuration: 30 * time.Second, FrameInterval: 5 * time.Second, MaxFrames: 6,
		},
		config.WorkerConfig{MaxAttempts: 5},
	)
	err := video.processVideoAudio(context.Background(), VideoJob{RecordingID: "video-1"})
	if err != nil {
		t.Fatalf("processVideoAudio() error = %v", err)
	}
	if !repository.saved || repository.transcript.Segments == nil ||
		len(repository.transcript.Segments) != 0 {
		t.Fatalf("saved transcript = %+v", repository.transcript)
	}
	if repository.transcript.AudioTrackPresent == nil ||
		*repository.transcript.AudioTrackPresent ||
		repository.transcript.Warning != "video contains no audio track" {
		t.Fatalf("audio-track diagnostic = %+v", repository.transcript)
	}
}

func TestVideoAudioJobReportsAudioWithNoDetectedSpeech(t *testing.T) {
	repository := &transcriptCaptureRepository{}
	video := newVideoService(
		repository, stubTranscriber{}, stubAudioStore{}, stubMediaExtractor{},
		stubVisualAnalyzer{}, stubMemographClient{},
		config.VideoConfig{
			EpisodeDuration: 30 * time.Second, FrameInterval: 5 * time.Second, MaxFrames: 6,
		},
		config.WorkerConfig{MaxAttempts: 5},
	)
	err := video.processVideoAudio(context.Background(), VideoJob{RecordingID: "video-1"})
	if err != nil {
		t.Fatalf("processVideoAudio() error = %v", err)
	}
	if repository.transcript.AudioTrackPresent == nil ||
		!*repository.transcript.AudioTrackPresent ||
		repository.transcript.Warning != "audio track was present, but no speech was detected" {
		t.Fatalf("audio-track diagnostic = %+v", repository.transcript)
	}
}
