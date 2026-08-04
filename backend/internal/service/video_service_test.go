package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
		"[62.00s-67.00s] Unknown speaker A: Update the contract before Friday.",
	} {
		if !strings.Contains(episode.Description, expected) {
			t.Fatalf("description %q does not contain %q", episode.Description, expected)
		}
	}
	if episode.VisualDescription != "Two people are reviewing a document." {
		t.Fatalf("visual description = %q", episode.VisualDescription)
	}
	if strings.Contains(episode.VisualDescription, "Objects visible:") ||
		strings.Contains(episode.VisualDescription, "Visible context:") {
		t.Fatalf("visual Memograph data contains a wrapper label: %q", episode.VisualDescription)
	}
	if episode.SpeechDescription != "[62.00s-67.00s] Unknown speaker A: Update the contract before Friday." {
		t.Fatalf("speech description = %q", episode.SpeechDescription)
	}
	if strings.Contains(episode.SpeechDescription, "Speech:") ||
		strings.Contains(episode.SpeechDescription, `"`) {
		t.Fatalf("speech Memograph data contains a wrapper label: %q", episode.SpeechDescription)
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
		repository, stubVoiceRepository{}, stubTranscriber{}, stubSpeakerAttributor{},
		stubAudioStore{}, store, noAudioExtractor{},
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
		repository, stubVoiceRepository{}, stubTranscriber{}, stubSpeakerAttributor{},
		stubAudioStore{}, store, noAudioExtractor{},
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
	saved        bool
	transcript   Transcript
	referenceIDs []string
}

func (r *transcriptCaptureRepository) SaveVideoTranscript(
	_ context.Context,
	_ VideoJob,
	transcript Transcript,
	referenceIDs []string,
	_, _ string,
	_ int,
) error {
	r.saved = true
	r.transcript = transcript
	r.referenceIDs = referenceIDs
	return nil
}

type videoEnrollmentRepository struct {
	stubVoiceRepository
	samples []VoiceEnrollmentRecord
}

func (r videoEnrollmentRepository) ListEnrollmentSamples(
	context.Context, string,
) ([]VoiceEnrollmentRecord, error) {
	return r.samples, nil
}

type videoEnrollmentStore struct{ stubAudioStore }

func (videoEnrollmentStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("voice")), nil
}

type videoOwnerTranscriber struct{ input TranscriptionInput }

func (t *videoOwnerTranscriber) Transcribe(
	_ context.Context, input TranscriptionInput,
) (Transcript, error) {
	t.input = input
	return Transcript{Segments: []TranscriptSegment{
		{StartTime: 0, EndTime: 2, Speaker: "owner_ref", Text: "What time is the meeting?"},
		{StartTime: 2, EndTime: 4, Speaker: "A", Text: "It starts at three."},
	}}, nil
}
func (*videoOwnerTranscriber) Provider() string { return "test" }
func (*videoOwnerTranscriber) Model() string    { return "test-diarize" }

type videoOwnerAttributor struct{ called bool }

func (a *videoOwnerAttributor) Attribute(
	_ context.Context, input SpeakerAttributionInput,
) (Transcript, error) {
	a.called = true
	for index := range input.Transcript.Segments {
		if input.Transcript.Segments[index].Speaker == input.References[0].ProviderLabel {
			input.Transcript.Segments[index].SpeakerRole = "owner"
		} else {
			input.Transcript.Segments[index].SpeakerRole = "other"
		}
	}
	return input.Transcript, nil
}

func TestVideoAudioJobUsesOwnerEnrollmentAndPersistsAttribution(t *testing.T) {
	repository := &transcriptCaptureRepository{}
	enrollments := videoEnrollmentRepository{samples: []VoiceEnrollmentRecord{{
		ID: "sample-1", ProviderLabel: "owner_ref", FilePath: "owner.m4a",
		MediaType: "audio/mp4", SizeBytes: 5,
	}}}
	transcriber := &videoOwnerTranscriber{}
	attributor := &videoOwnerAttributor{}
	video := newVideoService(
		repository, enrollments, transcriber, attributor,
		videoEnrollmentStore{}, stubAudioStore{}, stubMediaExtractor{},
		stubVisualAnalyzer{}, stubMemographClient{},
		config.VideoConfig{
			EpisodeDuration: 30 * time.Second, FrameInterval: 5 * time.Second, MaxFrames: 6,
		},
		config.WorkerConfig{MaxAttempts: 5},
	)

	err := video.processVideoAudio(context.Background(), VideoJob{
		RecordingID: "video-1", OwnerUserID: "owner-1",
	})
	if err != nil {
		t.Fatalf("processVideoAudio() error = %v", err)
	}
	if len(transcriber.input.KnownSpeakers) != 1 ||
		transcriber.input.KnownSpeakers[0].ProviderLabel != "owner_ref" {
		t.Fatalf("known speakers = %+v", transcriber.input.KnownSpeakers)
	}
	if !attributor.called {
		t.Fatal("speaker attributor was not called")
	}
	if len(repository.referenceIDs) != 1 || repository.referenceIDs[0] != "sample-1" {
		t.Fatalf("saved reference IDs = %+v", repository.referenceIDs)
	}
	if got := repository.transcript.Segments; len(got) != 2 ||
		got[0].SpeakerRole != "owner" || got[1].SpeakerRole != "other" {
		t.Fatalf("attributed transcript = %+v", got)
	}
}

func TestBuildVideoEpisodesLabelsOwnerAndOtherSpeech(t *testing.T) {
	episodes := BuildVideoEpisodes(Transcript{Segments: []TranscriptSegment{
		{StartTime: 1, EndTime: 2, Speaker: "owner_ref", SpeakerRole: "owner", Text: "Can you send the notes?"},
		{StartTime: 2, EndTime: 3, Speaker: "A", SpeakerRole: "other", Text: "Yes, I will."},
	}}, VisualAnalysis{}, 30*time.Second, 10, "session-1", "")
	if len(episodes) != 1 {
		t.Fatalf("len(episodes) = %d, want 1", len(episodes))
	}
	for _, expected := range []string{
		"[11.00s-12.00s] Owner: Can you send the notes?",
		"[12.00s-13.00s] Other speaker A: Yes, I will.",
	} {
		if !strings.Contains(episodes[0].SpeechDescription, expected) {
			t.Fatalf("speech description %q does not contain %q", episodes[0].SpeechDescription, expected)
		}
	}
}

func TestVideoAudioJobAllowsVideoWithoutAudioTrack(t *testing.T) {
	repository := &transcriptCaptureRepository{}
	video := newVideoService(
		repository, stubVoiceRepository{}, stubTranscriber{}, stubSpeakerAttributor{},
		stubAudioStore{}, stubAudioStore{}, noAudioExtractor{},
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
		repository, stubVoiceRepository{}, stubTranscriber{}, stubSpeakerAttributor{},
		stubAudioStore{}, stubAudioStore{}, stubMediaExtractor{},
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

type memographCompletionRepository struct {
	stubVideoRepository
	response json.RawMessage
}

func (r *memographCompletionRepository) CompleteVideoMemographEpisode(
	_ context.Context,
	_ VideoJob,
	response json.RawMessage,
) error {
	r.response = response
	return nil
}

type parallelMemographClient struct {
	stubMemographClient
	started  chan struct{}
	release  chan struct{}
	requests chan EpisodeInsertRequest
}

func (c *parallelMemographClient) InsertEpisode(
	ctx context.Context,
	_ string,
	request EpisodeInsertRequest,
) (json.RawMessage, error) {
	select {
	case c.started <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-c.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.requests <- request
	source, _ := request.Meta["source"].(string)
	return json.RawMessage(`{"source":"` + source + `"}`), nil
}

func TestVideoMemographWritesVisualAndSpeechInParallel(t *testing.T) {
	repository := &memographCompletionRepository{}
	client := &parallelMemographClient{
		started: make(chan struct{}, 2), release: make(chan struct{}),
		requests: make(chan EpisodeInsertRequest, 2),
	}
	video := newVideoService(
		repository, stubVoiceRepository{}, stubTranscriber{}, stubSpeakerAttributor{},
		stubAudioStore{}, stubAudioStore{}, stubMediaExtractor{},
		stubVisualAnalyzer{}, client,
		config.VideoConfig{
			EpisodeDuration: 30 * time.Second, FrameInterval: 5 * time.Second, MaxFrames: 6,
		},
		config.WorkerConfig{MaxAttempts: 5},
	)

	done := make(chan error, 1)
	go func() {
		done <- video.processVideoMemograph(context.Background(), VideoJob{
			MemoryID: "memory-1", SessionID: "session-1", GroupID: "group-1",
			VisualDescription: "View over a lake with a dock.",
			SpeechDescription: "A Said : you",
			EpisodeStart:      30, EpisodeEnd: 60,
		})
	}()

	for index := 0; index < 2; index++ {
		select {
		case <-client.started:
		case <-time.After(time.Second):
			t.Fatal("visual and speech Memograph calls did not overlap")
		}
	}
	close(client.release)
	if err := <-done; err != nil {
		t.Fatalf("processVideoMemograph() error = %v", err)
	}

	dataBySource := make(map[string]string, 2)
	for index := 0; index < 2; index++ {
		request := <-client.requests
		source, _ := request.Meta["source"].(string)
		dataBySource[source] = request.Data
	}
	if dataBySource["visual"] != "View over a lake with a dock." ||
		dataBySource["speech"] != "A Said : you" {
		t.Fatalf("Memograph branch data = %+v", dataBySource)
	}
	for _, data := range dataBySource {
		for _, forbidden := range []string{
			"Audio and video memory from session",
			"Visible context:",
			"Objects visible:",
			"Speech:",
		} {
			if strings.Contains(data, forbidden) {
				t.Fatalf("Memograph data %q contains %q", data, forbidden)
			}
		}
	}
	var responses map[string]json.RawMessage
	if err := json.Unmarshal(repository.response, &responses); err != nil {
		t.Fatalf("combined Memograph response = %s: %v", repository.response, err)
	}
	if len(responses) != 2 {
		t.Fatalf("combined Memograph responses = %s", repository.response)
	}
}

func TestLegacyVideoMemographDataRemovesCombinedEpisodeWrappers(t *testing.T) {
	visual, speech := legacyVideoMemographData(
		"Audio and video memory from session session-1 between 30.00s and 60.00s. " +
			"The scene was at or appeared to be home-4. " +
			"Visible context: View over a lake. A hand is holding a cup. " +
			"Activities: Looking over a lake. Objects visible: lake, hand, cup. " +
			`Speech: A said: "you"`,
	)
	if visual != "View over a lake. A hand is holding a cup." {
		t.Fatalf("legacy visual data = %q", visual)
	}
	if speech != "A Said : you" {
		t.Fatalf("legacy speech data = %q", speech)
	}
}
