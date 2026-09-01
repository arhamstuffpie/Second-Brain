package service

import (
	"context"
	"testing"

	"github.com/arham/ai-second-brain/internal/config"
)

type activeSpeakerDetectorStub struct{ result ActiveSpeakerAnalysis }

func (s activeSpeakerDetectorStub) DetectActiveSpeakers(context.Context, ActiveSpeakerInput) (ActiveSpeakerAnalysis, error) {
	return s.result, nil
}
func (activeSpeakerDetectorStub) Validate(context.Context) (ProviderMetadata, error) {
	return ProviderMetadata{}, nil
}
func (activeSpeakerDetectorStub) Provider() string { return "test-active-speaker" }
func (activeSpeakerDetectorStub) Model() string    { return "test-model" }

type automaticIdentityRepositoryCapture struct {
	personRepositoryCapture
	input AutomaticIdentityEvidenceInput
}

func (r *automaticIdentityRepositoryCapture) ResolveAutomaticIdentity(
	_ context.Context, input AutomaticIdentityEvidenceInput,
) (AutomaticIdentityResolution, error) {
	r.input = input
	return AutomaticIdentityResolution{
		PersonTrackID: input.PersonTrackID, VoiceSpeakerProfileID: input.VoiceSpeakerProfileID,
		PersonProfileID: "person-1", DisplayName: "Mark", Decision: input.Decision,
	}, nil
}

func TestVideoMergeAutomaticallyLinksStrongActiveSpeakerEvidence(t *testing.T) {
	repository := &automaticIdentityRepositoryCapture{}
	video := &videoService{
		store:            &realtimeTestStore{},
		personRepository: repository,
		activeSpeaker: activeSpeakerDetectorStub{result: ActiveSpeakerAnalysis{
			Provider: "test-active-speaker", Model: "test-model",
			Evidence: []ActiveSpeakerEvidence{{
				PersonTrackID: "track-1", SegmentIDs: []string{"segment-1", "segment-2"},
				Score: .95, VisibleMouthCoverage: .9,
			}},
		}},
		activeSpeakerConfig: config.ActiveSpeakerConfig{
			AutoLink: true, ScoreThreshold: .85, MinimumMouthCoverage: .75,
			MinimumTemporalCoverage: .75, MinimumSeparatedUtterances: 2,
			MergeEvidenceCount: 3,
		},
		faceConfig: config.FaceRecognitionConfig{Model: "face-model"},
	}
	job := VideoJob{
		OwnerUserID: "owner-1", RecordingID: "recording-1", FilePath: "video.mp4",
		FileName: "video.mp4", MediaType: "video/mp4", ProcessingVersion: 2,
		Transcript: Transcript{Segments: []TranscriptSegment{
			{ID: "segment-1", StartTime: 0, EndTime: 1, SpeakerProfileID: "voice-1"},
			{ID: "segment-2", StartTime: 2, EndTime: 3, SpeakerProfileID: "voice-1"},
		}},
		VisualAnalysis: VisualAnalysis{Observations: []VideoObservation{
			{FrameID: "frame-1", StartTime: 0, EndTime: 1.5, People: []VisualPerson{{
				PersonTrackID: "track-1", PersonProfileID: "person-1", PhysicalPresence: true,
			}}},
			{FrameID: "frame-2", StartTime: 1.5, EndTime: 3.5, People: []VisualPerson{{
				PersonTrackID: "track-1", PersonProfileID: "person-1", PhysicalPresence: true,
			}}},
		}},
	}
	if err := video.autoResolveVideoIdentities(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	if repository.input.Decision != "accepted" || repository.input.VoiceSpeakerProfileID != "voice-1" ||
		repository.input.PersonTrackID != "track-1" {
		t.Fatalf("automatic identity input = %+v", repository.input)
	}
	if job.Transcript.Segments[0].PersonProfileID != "person-1" ||
		job.Transcript.Segments[0].SpeakerName != "Mark" {
		t.Fatalf("resolved transcript = %+v", job.Transcript.Segments)
	}
}
