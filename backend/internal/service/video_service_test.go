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

type retentionVideoRepository struct {
	stubVideoRepository
	saved bool
}

type videoFaceIdentifierStub struct {
	identities map[string]VideoFaceIdentity
	err        error
	frames     []VideoFrame
}

func (s *videoFaceIdentifierStub) Identify(
	_ context.Context, _, _ string, _ int, frames []VideoFrame,
) (map[string]VideoFaceIdentity, error) {
	s.frames = append([]VideoFrame(nil), frames...)
	return s.identities, s.err
}

func TestIdentifyVisualFacesEnrichesOnlyOnePhysicalVisiblePersonAndFailsOpen(t *testing.T) {
	identifier := &videoFaceIdentifierStub{
		identities: map[string]VideoFaceIdentity{"frame-1": {
			PersonProfileID: "person-1", IdentityStatus: "confirmed", Outcome: "attached",
			DisplayName: "Mark", TrackID: "face-track-1",
		}},
		err: errors.New("one later frame failed"),
	}
	video := &videoService{faceIdentifier: identifier}
	analysis := VisualAnalysis{Observations: []VideoObservation{
		{FrameID: "frame-1", People: []VisualPerson{{PhysicalPresence: true, FaceVisible: true}}},
		{FrameID: "frame-2", People: []VisualPerson{{PhysicalPresence: true, FaceVisible: false}}},
		{FrameID: "frame-3", People: []VisualPerson{{PhysicalPresence: true, FaceVisible: true}, {PhysicalPresence: true, FaceVisible: true}}},
	}}
	frames := []VideoFrame{{FrameID: "frame-1"}, {FrameID: "frame-2"}, {FrameID: "frame-3"}}
	video.identifyVisualFaces(context.Background(), "owner-1", "recording-1", 1, &analysis, frames)
	person := analysis.Observations[0].People[0]
	if len(identifier.frames) != 1 || identifier.frames[0].FrameID != "frame-1" ||
		person.PersonProfileID != "person-1" || person.PersonName != "Mark" ||
		person.PersonTrackID != "face-track-1" || person.FaceIdentityOutcome != "attached" ||
		!strings.Contains(analysis.Warning, "face identification was unavailable") {
		t.Fatalf("eligible = %+v, person = %+v, warning = %q", identifier.frames, person, analysis.Warning)
	}
}

func TestIdentifyVisualFacesSkipsSampledPathWhenDenseIdentityIsEnabled(t *testing.T) {
	identifier := &videoFaceIdentifierStub{}
	video := &videoService{faceIdentifier: identifier, denseIdentityEnabled: true}
	analysis := VisualAnalysis{Observations: []VideoObservation{{
		FrameID: "frame-1", People: []VisualPerson{{PhysicalPresence: true, FaceVisible: true}},
	}}}
	video.identifyVisualFaces(
		context.Background(), "owner-1", "recording-1", EvidenceProcessingVersion,
		&analysis, []VideoFrame{{FrameID: "frame-1"}},
	)
	if len(identifier.frames) != 0 || analysis.Observations[0].People[0].PersonTrackID != "" {
		t.Fatalf("sampled identifier frames = %+v, analysis = %+v", identifier.frames, analysis)
	}
}

func (r *retentionVideoRepository) SaveVideoEpisodes(context.Context, VideoJob, []VideoEpisodeDraft, int) error {
	r.saved = true
	return nil
}

func TestVideoMergeRetainsOriginal(t *testing.T) {
	repository := &retentionVideoRepository{}
	store := &realtimeTestStore{}
	video := newVideoService(
		repository, stubVoiceRepository{}, stubTranscriber{}, stubSpeakerAttributor{},
		stubAudioStore{}, store, stubMediaExtractor{}, stubVisualAnalyzer{},
		stubMemographClient{}, config.VideoConfig{EpisodeDuration: 30 * time.Second},
		config.WorkerConfig{MaxAttempts: 5},
	)
	err := video.processVideoMerge(context.Background(), VideoJob{
		ID: 1, RecordingID: "recording-1", FilePath: "original.webm",
		VisualAnalysis: VisualAnalysis{Observations: []VideoObservation{{
			StartTime: 0, EndTime: 5, Summary: "A person is visible.",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !repository.saved || store.deleteCalls != 0 {
		t.Fatalf("saved = %v, original delete calls = %d", repository.saved, store.deleteCalls)
	}
}

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

type videoIngestRepository struct {
	stubVideoRepository
	input CreateVideoRecordingInput
}

func (r *videoIngestRepository) CreateVideoRecording(
	_ context.Context, input CreateVideoRecordingInput, _ int,
) (VideoRecording, error) {
	r.input = input
	return VideoRecording{ID: "video-1", GroupID: input.GroupID}, nil
}

func TestVideoIngestDefaultsToStableAccountGraphGroup(t *testing.T) {
	repository := &videoIngestRepository{}
	store := &realtimeTestStore{}
	video := newVideoService(
		repository, stubVoiceRepository{}, stubTranscriber{}, stubSpeakerAttributor{},
		stubAudioStore{}, store, stubMediaExtractor{}, stubVisualAnalyzer{},
		stubMemographClient{}, config.VideoConfig{}, config.WorkerConfig{MaxAttempts: 5},
	)
	_, err := video.IngestVideo(context.Background(), VideoIngestInput{
		OwnerUserID: "owner-1", SessionID: "session-1", MemoryID: "memory-1",
		FileName: "capture.mp4", MediaType: "video/mp4",
		Content: strings.NewReader("video"),
	})
	if err != nil {
		t.Fatalf("IngestVideo() error = %v", err)
	}
	if repository.input.GroupID != "account-owner:owner-1" {
		t.Fatalf("default group = %q", repository.input.GroupID)
	}
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
func (noAudioExtractor) ExtractFrames(context.Context, string, time.Duration, int) (FrameExtraction, error) {
	return FrameExtraction{}, errors.New("not used")
}
func (noAudioExtractor) ExtractFramesAt(context.Context, string, []VideoFrame) ([]VideoFrame, error) {
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
	job      VideoJob
	response json.RawMessage
	calls    int
}

func (r *memographCompletionRepository) CompleteVideoMemographBranch(
	_ context.Context,
	job VideoJob,
	response json.RawMessage,
) error {
	r.job = job
	r.response = response
	r.calls++
	return nil
}

type captureMemographClient struct {
	stubMemographClient
	requests []EpisodeInsertRequest
	err      error
}

func (c *captureMemographClient) InsertEpisode(
	_ context.Context,
	_ string,
	request EpisodeInsertRequest,
) (json.RawMessage, error) {
	c.requests = append(c.requests, request)
	if c.err != nil {
		return nil, c.err
	}
	source, _ := request.Meta["source"].(string)
	return json.RawMessage(`{"source":"` + source + `"}`), nil
}

func TestVideoMemographWritesOneDurableBranchWithCanonicalOwner(t *testing.T) {
	repository := &memographCompletionRepository{}
	client := &captureMemographClient{}
	video := newVideoService(
		repository, stubVoiceRepository{}, stubTranscriber{}, stubSpeakerAttributor{},
		stubAudioStore{}, stubAudioStore{}, stubMediaExtractor{},
		stubVisualAnalyzer{}, client,
		config.VideoConfig{
			EpisodeDuration: 30 * time.Second, FrameInterval: 5 * time.Second, MaxFrames: 6,
		},
		config.WorkerConfig{MaxAttempts: 5},
	)

	job := VideoJob{
		ID: 12, EpisodeID: "episode-1", MemographSource: "speech",
		OwnerUserID: "user-1", MemoryID: "memory-1",
		SessionID: "session-1", GroupID: "group-1",
		VisualDescription: "View over a lake with a dock.",
		SpeechDescription: "[30.00s-31.00s] Owner: I prefer tea.",
		EpisodeStart:      30, EpisodeEnd: 60,
		Transcript: Transcript{Segments: []TranscriptSegment{{
			StartTime: 30, EndTime: 31, Speaker: "owner_ref",
			SpeakerRole: "owner", Text: "I prefer tea.",
		}}},
	}
	if err := video.processVideoMemograph(context.Background(), job); err != nil {
		t.Fatalf("processVideoMemograph() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("Memograph requests = %d, want one branch", len(client.requests))
	}
	request := client.requests[0]
	if request.Meta["source"] != "speech" ||
		request.Meta["owner_entity_id"] != "account-owner:user-1" {
		t.Fatalf("Memograph metadata = %+v", request.Meta)
	}
	if request.IdempotencyKey == "" || request.Meta["idempotency_key"] != request.IdempotencyKey {
		t.Fatalf("idempotency key/meta = %q/%+v", request.IdempotencyKey, request.Meta)
	}
	if request.StructuredGraph == nil {
		t.Fatal("structured graph is nil")
	}
	owner := findStructuredEntity(request.StructuredGraph.Entities, "account-owner:user-1")
	utterance := findStructuredEntityByText(request.StructuredGraph.Entities, "I prefer tea.")
	if request.StructuredGraph.EpisodeID != "second-brain:video-speech:episode-1" ||
		len(request.StructuredGraph.Entities) != 2 ||
		owner == nil || owner.Type != "Person" ||
		utterance == nil || utterance.Type != "ConversationUtterance" ||
		len(request.StructuredGraph.Utterances) != 1 ||
		request.StructuredGraph.Utterances[0].Text != "I prefer tea." ||
		len(request.StructuredGraph.Relations) != 1 ||
		!hasStructuredRelation(
			request.StructuredGraph.Relations,
			owner.CanonicalID, "SAID", utterance.CanonicalID,
		) {
		t.Fatalf("structured graph = %+v", request.StructuredGraph)
	}
	if repository.calls != 1 || repository.job.MemographSource != "speech" {
		t.Fatalf("branch completion = %d/%+v", repository.calls, repository.job)
	}
}

func TestVideoMemographFailureDoesNotCompleteBranch(t *testing.T) {
	repository := &memographCompletionRepository{}
	client := &captureMemographClient{err: errors.New("database connections exhausted")}
	video := newVideoService(
		repository, stubVoiceRepository{}, stubTranscriber{}, stubSpeakerAttributor{},
		stubAudioStore{}, stubAudioStore{}, stubMediaExtractor{},
		stubVisualAnalyzer{}, client,
		config.VideoConfig{}, config.WorkerConfig{MaxAttempts: 5},
	)
	err := video.processVideoMemograph(context.Background(), VideoJob{
		ID: 13, EpisodeID: "episode-1", MemographSource: "visual",
		MemoryID: "memory-1", VisualDescription: "A lake is visible.",
	})
	if err == nil || !strings.Contains(err.Error(), "database connections exhausted") {
		t.Fatalf("processVideoMemograph() error = %v", err)
	}
	if len(client.requests) != 1 || repository.calls != 0 {
		t.Fatalf("requests/completions = %d/%d", len(client.requests), repository.calls)
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

func TestVideoSpeechDescriptionUsesConfirmedSpeakerName(t *testing.T) {
	got := videoSpeechDescription(Transcript{Segments: []TranscriptSegment{{
		StartTime: 1, EndTime: 2, Speaker: "A", SpeakerRole: "other",
		SpeakerName: "Raj", SpeakerRelationship: "friend", Text: "Hello.",
	}}}, 30, 30, 60)
	if got != "[31.00s-32.00s] Raj (friend): Hello." {
		t.Fatalf("videoSpeechDescription() = %q", got)
	}
}

func TestBuildEvidenceEpisodesUsesFiveSecondWindowsAndExactSpeech(t *testing.T) {
	confidence := 0.9
	episodes := BuildEvidenceEpisodes(
		Transcript{Language: "English", Segments: []TranscriptSegment{{
			ID: "segment-1", StartTime: 2, EndTime: 4, SpeakerRole: "owner",
			Text: "Keep this wording exactly.", Confidence: &confidence,
		}}},
		VisualAnalysis{Observations: []VideoObservation{
			{ObservationID: "obs-1", FrameID: "frame-1", StartTime: 4.9, EndTime: 5.4, Summary: "A document is visible."},
			{ObservationID: "obs-2", FrameID: "frame-2", StartTime: 6, EndTime: 7, Summary: "A screen is visible."},
		}},
		30*time.Second, 0, "session-1", "office", "recording-1", "asset-1", 2,
	)
	visual, speech, summaries := 0, 0, 0
	for _, episode := range episodes {
		switch episode.EvidenceKind {
		case "visual_evidence":
			visual++
			if episode.Visual[0].ObservationID == "obs-1" && episode.StartTime != 0 {
				t.Fatalf("cross-boundary observation assigned to %.2f, want start window 0", episode.StartTime)
			}
		case "speech_evidence":
			speech++
			if !strings.Contains(episode.Description, "Keep this wording exactly.") {
				t.Fatalf("speech was not preserved: %q", episode.Description)
			}
		case "context_summary":
			summaries++
			if len(episode.SupportingEpisodeIDs) == 0 {
				t.Fatal("context summary has no supporting evidence IDs")
			}
		}
	}
	if visual != 2 || speech != 1 || summaries != 1 {
		t.Fatalf("episode counts visual/speech/summary = %d/%d/%d", visual, speech, summaries)
	}
}
