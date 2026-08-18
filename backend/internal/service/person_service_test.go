package service

import (
	"context"
	"strings"
	"testing"

	"github.com/arham/ai-second-brain/internal/config"
)

type faceRecognizerStub struct{ result FaceRecognition }

func (s faceRecognizerStub) Recognize(context.Context, FaceRecognitionInput) (FaceRecognition, error) {
	return s.result, nil
}
func (faceRecognizerStub) Validate(context.Context) (FaceProviderMetadata, error) {
	return FaceProviderMetadata{}, nil
}
func (faceRecognizerStub) Provider() string { return "opencv" }
func (faceRecognizerStub) Model() string    { return "sface" }

type personRepositoryCapture struct {
	enrollment EnrollFaceProfileInput
	match      MatchFaceProfileInput
	confirmed  ConfirmPersonIdentityInput
}

func (r *personRepositoryCapture) EnrollFace(_ context.Context, input EnrollFaceProfileInput) (PersonProfile, error) {
	r.enrollment = input
	return PersonProfile{ID: "person-1"}, nil
}
func (r *personRepositoryCapture) MatchFace(_ context.Context, input MatchFaceProfileInput) (FaceMatch, error) {
	r.match = input
	return FaceMatch{Matched: true, PersonProfileID: "person-1", Reasons: []string{}}, nil
}
func (*personRepositoryCapture) SavePersonTrack(context.Context, SavePersonTrackInput) error {
	return nil
}
func (*personRepositoryCapture) ListPeople(context.Context, string) ([]PersonProfile, error) {
	return nil, nil
}
func (*personRepositoryCapture) UpdatePerson(context.Context, UpdatePersonInput) (PersonProfile, error) {
	return PersonProfile{}, nil
}
func (r *personRepositoryCapture) ConfirmIdentity(_ context.Context, input ConfirmPersonIdentityInput) (PersonProfile, error) {
	r.confirmed = input
	return PersonProfile{ID: "person-1"}, nil
}
func (*personRepositoryCapture) DeletePerson(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func usableFaceRecognition() FaceRecognition {
	return FaceRecognition{
		Provider: "opencv", Detector: "yunet", Model: "sface", Dimensions: 2,
		Faces: []DetectedFace{{
			DetectionScore: .99, Quality: FaceQuality{Usable: true, Reasons: []string{}, Score: .9},
			Pose:      FacePose{Bucket: "frontal"},
			Embedding: []float64{.6, .8},
		}},
	}
}

func TestPersonServiceRequiresBiometricConsent(t *testing.T) {
	repository := &personRepositoryCapture{}
	people := newPersonService(repository, faceRecognizerStub{result: usableFaceRecognition()}, stubAudioStore{}, config.FaceRecognitionConfig{MaxUploadBytes: 1024})
	_, err := people.EnrollFace(context.Background(), FaceEnrollmentInput{
		OwnerUserID: "owner", FileName: "face.jpg", Content: strings.NewReader("image"),
	})
	if err == nil || repository.enrollment.OwnerUserID != "" {
		t.Fatalf("error = %v, enrollment = %#v", err, repository.enrollment)
	}
}

func TestPersonServiceKeepsEmbeddingsServerSide(t *testing.T) {
	repository := &personRepositoryCapture{}
	people := newPersonService(repository, faceRecognizerStub{result: usableFaceRecognition()}, stubAudioStore{}, config.FaceRecognitionConfig{
		MaxUploadBytes: 1024, MatchThreshold: .7, AmbiguousMargin: .1,
	})
	match, err := people.RecognizeFace(context.Background(), FaceRecognitionRequest{
		OwnerUserID: "owner", FileName: "face.jpg", Content: strings.NewReader("image"),
	})
	if err != nil || !match.Matched || len(repository.match.Embedding) != 2 {
		t.Fatalf("match = %#v, repository input = %#v, error = %v", match, repository.match, err)
	}
}

func TestPersonServiceRequiresExplicitIdentityConfirmation(t *testing.T) {
	repository := &personRepositoryCapture{}
	people := newPersonService(repository, nil, stubAudioStore{}, config.FaceRecognitionConfig{})
	_, err := people.ConfirmIdentity(context.Background(), ConfirmPersonIdentityInput{
		OwnerUserID: "owner", RecordingIDs: []string{"recording-1"},
		VisualLabel: "person-1", VoiceSpeakerProfileID: "speaker-1",
	})
	if err == nil || repository.confirmed.OwnerUserID != "" {
		t.Fatalf("error = %v, confirmation = %+v", err, repository.confirmed)
	}
	result, err := people.ConfirmIdentity(context.Background(), ConfirmPersonIdentityInput{
		OwnerUserID: " owner ", RecordingIDs: []string{"recording-1", " recording-1 "},
		VisualLabel: " person-1 ", VoiceSpeakerProfileID: " speaker-1 ", Confirmed: true,
	})
	if err != nil || result.ID != "person-1" || len(repository.confirmed.RecordingIDs) != 1 {
		t.Fatalf("result = %+v, confirmation = %+v, error = %v", result, repository.confirmed, err)
	}
}
