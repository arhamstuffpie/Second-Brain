package service

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type videoFaceRepository struct {
	personRepositoryCapture
	matchResult FaceMatch
	enrollments int
}

func (r *videoFaceRepository) MatchFace(_ context.Context, input MatchFaceProfileInput) (FaceMatch, error) {
	r.match = input
	return r.matchResult, nil
}

func (r *videoFaceRepository) EnrollFace(_ context.Context, input EnrollFaceProfileInput) (PersonProfile, error) {
	r.enrollment = input
	r.enrollments++
	return PersonProfile{ID: "new-person", Status: "provisional"}, nil
}

type faceSampleStore struct {
	payload []byte
}

func (s *faceSampleStore) Save(_ context.Context, _ string, input io.Reader) (StoredAudio, error) {
	s.payload, _ = io.ReadAll(input)
	return StoredAudio{Path: "face-sample.jpg", SizeBytes: int64(len(s.payload))}, nil
}
func (*faceSampleStore) Open(context.Context, string) (io.ReadCloser, error) { return nil, nil }
func (*faceSampleStore) Delete(context.Context, string) error                { return nil }

func faceTestJPEG(t *testing.T) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			value.Set(x, y, color.RGBA{R: 180, G: 120, B: 90, A: 255})
		}
	}
	var payload bytes.Buffer
	if err := jpeg.Encode(&payload, value, nil); err != nil {
		t.Fatal(err)
	}
	return payload.Bytes()
}

func TestVideoFaceIdentifierReusesCanonicalPersonAcrossSessions(t *testing.T) {
	repository := &videoFaceRepository{matchResult: FaceMatch{
		Matched: true, PersonProfileID: "person-1", IdentityStatus: "confirmed", DisplayName: "Mark",
	}}
	recognition := usableFaceRecognition()
	recognition.Faces[0].Box = FaceBox{X: 2, Y: 2, Width: 10, Height: 10}
	identifier := NewVideoFaceIdentifier(
		repository, faceRecognizerStub{result: recognition}, &faceSampleStore{},
		config.FaceRecognitionConfig{MatchThreshold: .5, AmbiguousMargin: .1},
	)
	for _, frameID := range []string{"session-1-frame", "session-2-frame"} {
		identities, err := identifier.Identify(context.Background(), "owner-1", []VideoFrame{{
			FrameID: frameID, MediaType: "image/jpeg", Image: faceTestJPEG(t),
		}})
		if err != nil || identities[frameID].PersonProfileID != "person-1" || identities[frameID].DisplayName != "Mark" {
			t.Fatalf("frame %s identities = %+v, error = %v", frameID, identities, err)
		}
	}
	if repository.enrollments != 0 {
		t.Fatalf("canonical match created %d new profiles", repository.enrollments)
	}
}

func TestVideoFaceIdentifierCreatesConsentPendingCroppedProfile(t *testing.T) {
	repository := &videoFaceRepository{matchResult: FaceMatch{Reasons: []string{"no_compatible_face_profiles"}}}
	store := &faceSampleStore{}
	recognition := usableFaceRecognition()
	recognition.Faces[0].Box = FaceBox{X: 2, Y: 2, Width: 10, Height: 10}
	identifier := NewVideoFaceIdentifier(
		repository, faceRecognizerStub{result: recognition}, store,
		config.FaceRecognitionConfig{MatchThreshold: .5, AmbiguousMargin: .1, ProvisionalTTL: 30 * 24 * time.Hour},
	)
	identities, err := identifier.Identify(context.Background(), "owner-1", []VideoFrame{{
		FrameID: "frame-1", MediaType: "image/jpeg", Image: faceTestJPEG(t),
	}})
	if err != nil || identities["frame-1"].PersonProfileID != "new-person" {
		t.Fatalf("identities = %+v, error = %v", identities, err)
	}
	if repository.enrollments != 1 || repository.enrollment.ConsentState != "pending" ||
		repository.enrollment.DisplayName != "" || len(store.payload) == 0 {
		t.Fatalf("enrollment = %+v, stored bytes = %d", repository.enrollment, len(store.payload))
	}
}
