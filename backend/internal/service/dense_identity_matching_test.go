package service

import (
	"context"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

const denseTestModel = "opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79"

func denseIdentityTrack(id string, box FaceBox, embeddings ...[]float64) DenseIdentityTrack {
	track := DenseIdentityTrack{
		DensePersonTrack: DensePersonTrack{
			ID: id, LifecycleStatus: "ended", StartTime: 0, EndTime: 1,
			TrackingConfidence: .95,
		},
		Provider: "opencv", DetectorModel: "opencv/yunet-2023mar",
		EmbeddingModel: denseTestModel,
	}
	for index, embedding := range embeddings {
		observationID := id + "-observation-" + string(rune('1'+index))
		track.GalleryObservationIDs = append(track.GalleryObservationIDs, observationID)
		track.Observations = append(track.Observations, DenseFaceObservation{
			ObservationID: observationID, FrameIndex: index, Timestamp: float64(index) / 8,
			Box: box, DetectionScore: .98,
			Quality: FaceQuality{Usable: true, Score: .9},
			Pose:    FacePose{Bucket: "frontal"}, Embedding: embedding,
		})
	}
	track.ObservationCount = len(track.Observations)
	return track
}

func TestDenseIdentityMatchingUsesTrackGalleryAndMapsScenePerson(t *testing.T) {
	similarity := .91
	repository := &videoFaceRepository{matchResult: FaceMatch{
		Matched: true, PersonProfileID: "person-1", IdentityStatus: "confirmed",
		DisplayName: "Mark", Similarity: &similarity,
	}}
	video := &videoService{
		personRepository: repository,
		faceConfig: config.FaceRecognitionConfig{
			Model: denseTestModel, MatchThreshold: .5, AmbiguousMargin: .1,
			ProvisionalTTL: 30 * 24 * time.Hour,
		},
	}
	faceBox := &FaceBox{X: 10, Y: 20, Width: 50, Height: 50}
	job := VideoJob{
		OwnerUserID: "owner-1", RecordingID: "recording-1", ProcessingVersion: 3,
		DenseIdentityTracks: []DenseIdentityTrack{
			denseIdentityTrack("dense-track-1", *faceBox, []float64{1, 0}, []float64{.99, .01}),
		},
		VisualAnalysis: VisualAnalysis{Observations: []VideoObservation{{
			StartTime: 0, People: []VisualPerson{{
				PhysicalPresence: true, FaceVisible: true, FaceBox: faceBox,
			}},
		}}},
	}
	if err := video.matchDenseTrackIdentities(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	person := job.VisualAnalysis.Observations[0].People[0]
	if repository.matches != 2 || repository.enrollments != 2 || len(repository.tracks) != 1 {
		t.Fatalf("matches/enrollments/tracks = %d/%d/%d", repository.matches, repository.enrollments, len(repository.tracks))
	}
	if person.PersonTrackID != "dense-track-1" || person.PersonProfileID != "person-1" ||
		person.PersonName != "Mark" || person.FaceIdentityOutcome != "attached" {
		t.Fatalf("mapped person = %+v", person)
	}
	if repository.tracks[0].ID != "dense-track-1" ||
		repository.tracks[0].ResolvedPersonProfileID != "person-1" {
		t.Fatalf("saved track = %+v", repository.tracks[0])
	}
}

func TestDenseIdentityMatchingKeepsAmbiguousGalleryUnknown(t *testing.T) {
	repository := &videoFaceRepository{matchResults: []FaceMatch{
		{Reasons: []string{"no_compatible_face_profiles"}},
		{Ambiguous: true, Reasons: []string{"match_threshold_or_runner_up_margin_not_met"}},
	}}
	video := &videoService{
		personRepository: repository,
		faceConfig: config.FaceRecognitionConfig{
			Model: denseTestModel, MatchThreshold: .5, AmbiguousMargin: .1,
			ProvisionalTTL: 30 * 24 * time.Hour,
		},
	}
	job := VideoJob{
		OwnerUserID: "owner-1", RecordingID: "recording-1", ProcessingVersion: 3,
		DenseIdentityTracks: []DenseIdentityTrack{
			denseIdentityTrack("dense-track-1", FaceBox{Width: 50, Height: 50}, []float64{1, 0}, []float64{0, 1}),
		},
	}
	if err := video.matchDenseTrackIdentities(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	if repository.enrollments != 0 || len(repository.tracks) != 1 ||
		repository.tracks[0].ResolvedPersonProfileID != "" {
		t.Fatalf("enrollments/tracks = %d/%+v", repository.enrollments, repository.tracks)
	}
}

func TestDenseSceneMappingKeepsCrossingTracksSeparate(t *testing.T) {
	left := denseIdentityTrack("track-left", FaceBox{X: 10, Y: 10, Width: 40, Height: 40}, []float64{1, 0})
	right := denseIdentityTrack("track-right", FaceBox{X: 110, Y: 10, Width: 40, Height: 40}, []float64{0, 1})
	analysis := VisualAnalysis{Observations: []VideoObservation{{
		StartTime: 0,
		People: []VisualPerson{
			{PhysicalPresence: true, FaceVisible: true, FaceBox: &FaceBox{X: 112, Y: 10, Width: 40, Height: 40}},
			{PhysicalPresence: true, FaceVisible: true, FaceBox: &FaceBox{X: 12, Y: 10, Width: 40, Height: 40}},
		},
	}}}
	mapScenePeopleToDenseTracks(&analysis, []DenseIdentityTrack{left, right}, map[string]VideoFaceIdentity{
		"track-left":  {TrackID: "track-left", PersonProfileID: "person-left", Outcome: "attached"},
		"track-right": {TrackID: "track-right", PersonProfileID: "person-right", Outcome: "attached"},
	})
	people := analysis.Observations[0].People
	if people[0].PersonTrackID != "track-right" || people[0].PersonProfileID != "person-right" ||
		people[1].PersonTrackID != "track-left" || people[1].PersonProfileID != "person-left" {
		t.Fatalf("crossing scene mapping = %+v", people)
	}
}
