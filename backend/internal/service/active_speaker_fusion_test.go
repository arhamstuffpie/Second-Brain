package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

func fusionTestConfig() config.ActiveSpeakerConfig {
	return config.ActiveSpeakerConfig{
		VoiceThreshold: .8, ScoreThreshold: .8, MinimumMargin: .15,
		MinimumSegmentDuration: .5, MinimumMouthCoverage: .6,
		MinimumTemporalCoverage: .7, MinimumSeparatedUtterances: 2,
	}
}

func fusionTestJob() ActiveSpeakerFusionJob {
	confidence := .94
	return ActiveSpeakerFusionJob{
		OwnerUserID: "owner-1", RecordingID: "recording-1", ProcessingVersion: 3,
		Transcript: Transcript{Segments: []TranscriptSegment{{
			ID: "segment-1", StartTime: 10, EndTime: 14, SpeakerProfileID: "voice-casy",
			IdentityConfidence: &confidence,
		}}},
		VoiceProfiles: map[string]ActiveSpeakerFusionVoiceProfile{
			"voice-casy": {ID: "voice-casy", PersonProfileID: "person-casy", DisplayName: "Casy"},
		},
		Tracks: []DenseIdentityTrack{fusionTrack("track-1", 9, 15, .9, .8)},
	}
}

func fusionTrack(id string, start, end, mouthCoverage, activity float64) DenseIdentityTrack {
	observations := make([]DenseFaceObservation, 10)
	for index := range observations {
		observations[index] = DenseFaceObservation{
			ObservationID: id + "-observation-" + string(rune('a'+index)),
			Timestamp:     10 + float64(index)*.4, MouthVisible: float64(index)/10 < mouthCoverage,
			MouthActivity: activity,
		}
	}
	return DenseIdentityTrack{DensePersonTrack: DensePersonTrack{
		ID: id, StartTime: start, EndTime: end, TrackingConfidence: .95, Observations: observations,
	}}
}

func fusionAnalysis(segmentID string, scores map[string]float64) ActiveSpeakerAnalysis {
	analysis := ActiveSpeakerAnalysis{Provider: "talknet", Model: "talknet@sha256:test"}
	for trackID, score := range scores {
		analysis.Evidence = append(analysis.Evidence, ActiveSpeakerEvidence{
			PersonTrackID: trackID, SegmentIDs: []string{segmentID}, Score: score,
		})
	}
	return analysis
}

func TestActiveSpeakerFusionLinksOneLabelledVoiceToSpeakingFace(t *testing.T) {
	job, cfg := fusionTestJob(), fusionTestConfig()
	_, candidates, immediate := prepareActiveSpeakerFusion(job, cfg)
	if len(immediate) != 0 {
		t.Fatalf("immediate evidence = %+v", immediate)
	}
	result := evaluateActiveSpeakerFusion(job, candidates, fusionAnalysis("segment-1", map[string]float64{"track-1": .91}), cfg)
	if len(result) != 1 || result[0].Decision != "accepted" || result[0].CanonicalPersonID != "person-casy" {
		t.Fatalf("fusion result = %+v", result)
	}
}

func TestActiveSpeakerFusionChoosesSpeakingFaceAndRejectsSilentFace(t *testing.T) {
	job, cfg := fusionTestJob(), fusionTestConfig()
	job.Tracks = append(job.Tracks, fusionTrack("track-2", 9, 15, .9, .05))
	_, candidates, _ := prepareActiveSpeakerFusion(job, cfg)
	result := evaluateActiveSpeakerFusion(job, candidates, fusionAnalysis("segment-1", map[string]float64{
		"track-1": .92, "track-2": .12,
	}), cfg)
	if len(result) != 2 || result[0].PersonTrackID != "track-1" || result[0].Decision != "accepted" || result[1].Decision != "rejected" {
		t.Fatalf("fusion result = %+v", result)
	}
}

func TestActiveSpeakerFusionKeepsSimilarFacesAmbiguous(t *testing.T) {
	job, cfg := fusionTestJob(), fusionTestConfig()
	job.Tracks = append(job.Tracks, fusionTrack("track-2", 9, 15, .9, .8))
	_, candidates, _ := prepareActiveSpeakerFusion(job, cfg)
	result := evaluateActiveSpeakerFusion(job, candidates, fusionAnalysis("segment-1", map[string]float64{
		"track-1": .91, "track-2": .87,
	}), cfg)
	if len(result) == 0 || result[0].Decision != "ambiguous" || !containsString(result[0].Reasons, "runner_up_margin_too_low") {
		t.Fatalf("fusion result = %+v", result)
	}
}

func TestActiveSpeakerFusionTreatsOffscreenAndUnsafeOverlapAsUnknown(t *testing.T) {
	cfg := fusionTestConfig()
	offscreen := fusionTestJob()
	offscreen.Tracks = nil
	input, _, evidence := prepareActiveSpeakerFusion(offscreen, cfg)
	if len(input.Segments) != 0 || len(evidence) != 1 || !containsString(evidence[0].Reasons, "off_screen") {
		t.Fatalf("offscreen input=%+v evidence=%+v", input, evidence)
	}
	overlap := fusionTestJob()
	overlap.Transcript.Segments[0].Overlap = true
	overlap.Transcript.Segments[0].SeparationStatus = "ambiguous"
	input, _, evidence = prepareActiveSpeakerFusion(overlap, cfg)
	if len(input.Segments) != 0 || len(evidence) != 1 || !containsString(evidence[0].Reasons, "overlapping_speech_not_separated") {
		t.Fatalf("overlap input=%+v evidence=%+v", input, evidence)
	}
}

func TestActiveSpeakerFusionEvaluatesReliablySeparatedOverlapIndependently(t *testing.T) {
	job, cfg := fusionTestJob(), fusionTestConfig()
	separation := .93
	job.Transcript.Segments[0].Overlap = true
	job.Transcript.Segments[0].SeparationStatus = "accepted"
	job.Transcript.Segments[0].SeparationConfidence = &separation
	input, _, evidence := prepareActiveSpeakerFusion(job, cfg)
	if len(input.Segments) != 1 || len(evidence) != 0 {
		t.Fatalf("input=%+v evidence=%+v", input, evidence)
	}
}

func TestActiveSpeakerFusionUsesDenseContinuityAcrossPoseAndBriefGaps(t *testing.T) {
	job, cfg := fusionTestJob(), fusionTestConfig()
	job.Tracks[0].Observations = job.Tracks[0].Observations[:7]
	for index := range job.Tracks[0].Observations {
		job.Tracks[0].Observations[index].Pose.Bucket = "left_profile"
	}
	_, candidates, _ := prepareActiveSpeakerFusion(job, cfg)
	if got := candidates["segment-1"][0].temporal; got != 1 {
		t.Fatalf("temporal coverage = %v, want dense track continuity 1", got)
	}
}

func TestActiveSpeakerFusionRejectsWeakOrCrossOwnerVoiceAndIsDeterministic(t *testing.T) {
	job, cfg := fusionTestJob(), fusionTestConfig()
	weak := .4
	job.Transcript.Segments[0].IdentityConfidence = &weak
	_, _, first := prepareActiveSpeakerFusion(job, cfg)
	_, _, second := prepareActiveSpeakerFusion(job, cfg)
	if len(first) != 1 || first[0].DeterministicKey != second[0].DeterministicKey || !containsString(first[0].Reasons, "voice_match_too_low") {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	job.VoiceProfiles = map[string]ActiveSpeakerFusionVoiceProfile{}
	input, _, evidence := prepareActiveSpeakerFusion(job, cfg)
	if len(input.Segments) != 0 || len(evidence) != 0 {
		t.Fatalf("cross-owner voice leaked into fusion: input=%+v evidence=%+v", input, evidence)
	}
}

func TestActiveSpeakerFusionConflictsDifferentKnownVoicesOnOneTrack(t *testing.T) {
	evidence := []ActiveSpeakerFusionEvidence{
		{PersonTrackID: "track-1", CanonicalPersonID: "person-casy", Decision: "accepted"},
		{PersonTrackID: "track-1", CanonicalPersonID: "person-john", Decision: "accepted"},
	}
	applyFusionBatchConflicts(evidence)
	for _, item := range evidence {
		if item.Decision != "ambiguous" || !containsString(item.Reasons, "different_known_voices_same_track") {
			t.Fatalf("conflicting evidence = %+v", evidence)
		}
	}
}

type fusionRepositoryStub struct {
	job     ActiveSpeakerFusionJob
	retries int
}

func (r *fusionRepositoryStub) ClaimActiveSpeakerFusion(context.Context, string, time.Duration) (ActiveSpeakerFusionJob, bool, error) {
	return r.job, true, nil
}
func (*fusionRepositoryStub) CompleteActiveSpeakerFusion(context.Context, ActiveSpeakerFusionJob, ActiveSpeakerFusionResult, ActiveSpeakerFusionPersistenceOptions) error {
	return nil
}
func (r *fusionRepositoryStub) RetryActiveSpeakerFusion(context.Context, ActiveSpeakerFusionJob, string, time.Time, bool) error {
	r.retries++
	return nil
}

type mismatchedFusionDetector struct{}

func (mismatchedFusionDetector) DetectActiveSpeakers(context.Context, ActiveSpeakerInput) (ActiveSpeakerAnalysis, error) {
	return ActiveSpeakerAnalysis{}, errors.New("must not run")
}
func (mismatchedFusionDetector) Validate(context.Context) (ProviderMetadata, error) {
	return ProviderMetadata{Provider: "talknet", Model: "wrong-checksum"}, nil
}
func (mismatchedFusionDetector) Provider() string { return "talknet" }
func (mismatchedFusionDetector) Model() string    { return "talknet@sha256:expected" }

func TestActiveSpeakerFusionModelMismatchFailsSafelyAndRetries(t *testing.T) {
	repository := &fusionRepositoryStub{job: fusionTestJob()}
	repository.job.ID, repository.job.AnalysisRunID = 1, "run-1"
	repository.job.Attempts, repository.job.MaxAttempts = 1, 3
	configured, err := newActiveSpeakerFusionService(repository, mismatchedFusionDetector{}, densePersonStoreStub{}, config.ActiveSpeakerConfig{
		Timeout: time.Minute, FusionEnabled: true, VoiceThreshold: .8, ScoreThreshold: .8, MinimumMargin: .1,
		MinimumSegmentDuration: .5, MinimumMouthCoverage: .5, MinimumTemporalCoverage: .5,
		MinimumSeparatedUtterances: 2,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	processed, err := configured.ProcessNextActiveSpeakerFusion(context.Background())
	if !processed || err == nil || repository.retries != 1 {
		t.Fatalf("processed=%t err=%v retries=%d", processed, err, repository.retries)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
