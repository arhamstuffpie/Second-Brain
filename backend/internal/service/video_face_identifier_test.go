package service

import (
	"context"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type videoFaceRepository struct {
	personRepositoryCapture
	matchResult  FaceMatch
	matchResults []FaceMatch
	matches      int
	enrollments  int
	newProfiles  int
	tracks       []SavePersonTrackInput
}

func (r *videoFaceRepository) MatchFace(_ context.Context, input MatchFaceProfileInput) (FaceMatch, error) {
	r.match = input
	index := r.matches
	r.matches++
	if index < len(r.matchResults) {
		result := r.matchResults[index]
		return result, nil
	}
	return r.matchResult, nil
}

func (r *videoFaceRepository) EnrollFace(_ context.Context, input EnrollFaceProfileInput) (PersonProfile, error) {
	r.enrollment = input
	r.enrollments++
	if input.PersonProfileID != "" {
		return PersonProfile{ID: input.PersonProfileID, Status: "provisional"}, nil
	}
	r.newProfiles++
	return PersonProfile{ID: "new-person", Status: "provisional"}, nil
}

func (r *videoFaceRepository) SavePersonTrack(_ context.Context, input SavePersonTrackInput) error {
	r.tracks = append(r.tracks, input)
	return nil
}

func TestVideoFaceIdentifierReusesCanonicalPersonAcrossSessions(t *testing.T) {
	repository := &videoFaceRepository{matchResult: FaceMatch{
		Matched: true, PersonProfileID: "person-1", IdentityStatus: "confirmed", DisplayName: "Mark",
	}}
	recognition := usableFaceRecognition()
	recognition.Faces[0].Box = FaceBox{X: 2, Y: 2, Width: 10, Height: 10}
	identifier := NewVideoFaceIdentifier(
		repository, faceRecognizerStub{result: recognition},
		config.FaceRecognitionConfig{MatchThreshold: .5, AmbiguousMargin: .1},
	)
	for _, capture := range []struct{ recordingID, frameID string }{
		{"recording-1", "session-1-frame"},
		{"recording-2", "session-2-frame"},
	} {
		identities, err := identifier.Identify(context.Background(), "owner-1", capture.recordingID, 1, []VideoFrame{{
			FrameID: capture.frameID, MediaType: "image/jpeg", Image: []byte("image"),
		}})
		if err != nil || identities[capture.frameID].PersonProfileID != "person-1" || identities[capture.frameID].DisplayName != "Mark" {
			t.Fatalf("frame %s identities = %+v, error = %v", capture.frameID, identities, err)
		}
	}
	if repository.enrollments != 2 || repository.enrollment.PersonProfileID != "person-1" {
		t.Fatalf("canonical gallery additions = %d, last = %+v", repository.enrollments, repository.enrollment)
	}
}

func TestVideoFaceIdentifierSeparatesUnknownTracksAcrossRecordings(t *testing.T) {
	repository := &videoFaceRepository{matchResult: FaceMatch{Reasons: []string{"no_compatible_face_profiles"}}}
	recognition := usableFaceRecognition()
	recognition.Faces[0].Box = FaceBox{X: 2, Y: 2, Width: 10, Height: 10}
	identifier := NewVideoFaceIdentifier(
		repository, faceRecognizerStub{result: recognition},
		config.FaceRecognitionConfig{MatchThreshold: .5, AmbiguousMargin: .1},
	)

	var trackIDs []string
	for _, recordingID := range []string{"recording-mark", "recording-mrwho"} {
		identities, err := identifier.Identify(context.Background(), "owner-1", recordingID, 1, []VideoFrame{{
			FrameID: "frame-1", MediaType: "image/jpeg", Image: []byte("image"),
		}})
		if err != nil || identities["frame-1"].TrackID == "" {
			t.Fatalf("recording %s identities = %+v, error = %v", recordingID, identities, err)
		}
		trackIDs = append(trackIDs, identities["frame-1"].TrackID)
	}
	if trackIDs[0] == trackIDs[1] || len(repository.tracks) != 2 ||
		repository.tracks[0].RecordingID == repository.tracks[1].RecordingID {
		t.Fatalf("track IDs = %v, saved tracks = %+v", trackIDs, repository.tracks)
	}
}

func TestVideoFaceIdentifierKeepsFirstUnknownFrameAsUnresolvedTrack(t *testing.T) {
	repository := &videoFaceRepository{matchResult: FaceMatch{Reasons: []string{"no_compatible_face_profiles"}}}
	recognition := usableFaceRecognition()
	recognition.Faces[0].Box = FaceBox{X: 2, Y: 2, Width: 10, Height: 10}
	identifier := NewVideoFaceIdentifier(
		repository, faceRecognizerStub{result: recognition},
		config.FaceRecognitionConfig{MatchThreshold: .5, AmbiguousMargin: .1, ProvisionalTTL: 30 * 24 * time.Hour},
	)
	identities, err := identifier.Identify(context.Background(), "owner-1", "recording-1", 1, []VideoFrame{{
		FrameID: "frame-1", MediaType: "image/jpeg", Image: []byte("image"),
	}})
	if err != nil || identities["frame-1"].PersonProfileID != "" || identities["frame-1"].TrackID == "" ||
		identities["frame-1"].Outcome != "ambiguous" {
		t.Fatalf("identities = %+v, error = %v", identities, err)
	}
	if repository.enrollments != 0 || len(repository.tracks) != 1 ||
		repository.tracks[0].ResolvedPersonProfileID != "" {
		t.Fatalf("enrollment = %+v, tracks = %+v", repository.enrollment, repository.tracks)
	}
}

func TestVideoFaceIdentifierDoesNotCreateProfileFromAmbiguousTrack(t *testing.T) {
	repository := &videoFaceRepository{matchResults: []FaceMatch{
		{Reasons: []string{"no_compatible_face_profiles"}},
		{Ambiguous: true, Reasons: []string{"match_threshold_or_runner_up_margin_not_met"}},
	}}
	recognition := usableFaceRecognition()
	recognition.Faces[0].Box = FaceBox{X: 2, Y: 2, Width: 10, Height: 10}
	identifier := NewVideoFaceIdentifier(repository, faceRecognizerStub{result: recognition}, config.FaceRecognitionConfig{
		MatchThreshold: .5, AmbiguousMargin: .1, ProvisionalTTL: 30 * 24 * time.Hour,
	})
	identities, err := identifier.Identify(context.Background(), "owner-1", "recording-1", 1, []VideoFrame{
		{FrameID: "frame-1", Timestamp: 0, Image: []byte("image")},
		{FrameID: "frame-2", Timestamp: 5, Image: []byte("image")},
	})
	if err != nil || identities["frame-1"].PersonProfileID != "" ||
		identities["frame-2"].PersonProfileID != "" || repository.enrollments != 0 ||
		identities["frame-1"].TrackID != identities["frame-2"].TrackID {
		t.Fatalf("identities = %+v, error = %v", identities, err)
	}
}

func TestVideoFaceIdentifierCreatesOneProvisionalProfilePerUnknownTrack(t *testing.T) {
	repository := &videoFaceRepository{matchResult: FaceMatch{Reasons: []string{"no_compatible_face_profiles"}}}
	recognition := usableFaceRecognition()
	recognition.Faces[0].Box = FaceBox{X: 2, Y: 2, Width: 10, Height: 10}
	identifier := NewVideoFaceIdentifier(repository, faceRecognizerStub{result: recognition}, config.FaceRecognitionConfig{
		MatchThreshold: .5, AmbiguousMargin: .1, ProvisionalTTL: 30 * 24 * time.Hour,
	})
	identities, err := identifier.Identify(context.Background(), "owner-1", "recording-1", 1, []VideoFrame{
		{FrameID: "frame-1", Timestamp: 0, Image: []byte("image")},
		{FrameID: "frame-2", Timestamp: 5, Image: []byte("image")},
		{FrameID: "frame-3", Timestamp: 10, Image: []byte("image")},
	})
	if err != nil || repository.enrollments != 3 || repository.newProfiles != 1 {
		t.Fatalf("enrollments/new profiles = %d/%d, identities = %+v, error = %v", repository.enrollments, repository.newProfiles, identities, err)
	}
	for _, frameID := range []string{"frame-1", "frame-2", "frame-3"} {
		if identities[frameID].PersonProfileID != "new-person" ||
			identities[frameID].TrackID != identities["frame-1"].TrackID {
			t.Fatalf("identities = %+v", identities)
		}
	}
	if identities["frame-1"].Outcome != "provisional" || identities["frame-2"].Outcome != "provisional" ||
		identities["frame-3"].Outcome != "attached" {
		t.Fatalf("identity outcomes = %+v", identities)
	}
}
