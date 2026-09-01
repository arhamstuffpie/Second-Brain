package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestDensePersonAnalysisRepositoryPersistsTwoFaceTracks(t *testing.T) {
	databaseURL := os.Getenv("APP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("APP_TEST_DATABASE_URL is not set")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	const ownerID = "dense-person-integration-owner"
	if _, err := database.ExecContext(ctx, `
INSERT INTO users (id,email,password_hash) VALUES ($1,'dense-person@example.com','not-used')
ON CONFLICT (id) DO NOTHING`, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, ownerID)
	})
	baseRepository, err := newBase(database)
	if err != nil {
		t.Fatal(err)
	}
	videoRepository := newVideoRepository(baseRepository)
	recording, err := videoRepository.CreateVideoRecording(ctx, service.CreateVideoRecordingInput{
		OwnerUserID: ownerID, SessionID: "session-1", GroupID: "group-1", MemoryID: "memory-1",
		FileName: "two-faces.mp4", FilePath: "/tmp/two-faces.mp4", MediaType: "video/mp4",
		SizeBytes: 100, StorageProvider: "local", SHA256: "dense-person-sha", FrameInterval: 5,
	}, 5)
	if err != nil {
		t.Fatal(err)
	}
	repository := newDensePersonAnalysisRepository(baseRepository)
	job, found, err := repository.ClaimDensePersonAnalysis(ctx, `{}`, time.Hour)
	if err != nil || !found {
		t.Fatalf("claim dense person job: found=%t err=%v", found, err)
	}
	if job.RecordingID != recording.ID {
		t.Fatalf("claimed recording %q, want %q", job.RecordingID, recording.ID)
	}
	observation := func(id string, frame int, embedding []float64) service.DenseFaceObservation {
		return service.DenseFaceObservation{
			ObservationID: id, FrameIndex: frame, Timestamp: float64(frame) / 8,
			Box:            service.FaceBox{X: frame, Y: frame, Width: 100, Height: 100},
			DetectionScore: 0.95, Quality: service.FaceQuality{Usable: true, Score: 0.9},
			Pose: service.FacePose{Bucket: "frontal"}, Embedding: embedding,
			MouthVisible: true, MouthActivity: 0.5,
		}
	}
	analysis := service.DensePersonAnalysis{
		RecordingID: recording.ID, ProcessingVersion: job.ProcessingVersion,
		DurationSeconds: 2, AnalyzedFPS: 8,
		Provenance: service.ModelProvenance{
			DetectorModel: "opencv/yunet-2023mar", EmbeddingModel: "opencv/sface-2021dec",
			RuntimeVersion: "test", Device: "cpu",
		},
		Tracks: []service.DensePersonTrack{
			{
				ID: "dense-track-1", ProviderTrackReference: "provider-1", LifecycleStatus: "ended",
				FirstFrame: 0, LastFrame: 0, StartTime: 0, EndTime: 0,
				ObservationCount: 1, TrackingConfidence: 0.95,
				Quality:               service.PersonTrackQuality{Mean: 0.9, Maximum: 0.9, UsableObservations: 1},
				GalleryObservationIDs: []string{"dense-observation-1"},
				Observations:          []service.DenseFaceObservation{observation("dense-observation-1", 0, []float64{0.1, 0.2})},
			},
			{
				ID: "dense-track-2", ProviderTrackReference: "provider-2", LifecycleStatus: "ended",
				FirstFrame: 8, LastFrame: 8, StartTime: 1, EndTime: 1,
				ObservationCount: 1, TrackingConfidence: 0.94,
				Quality:               service.PersonTrackQuality{Mean: 0.88, Maximum: 0.88, UsableObservations: 1},
				GalleryObservationIDs: []string{"dense-observation-2"},
				Observations:          []service.DenseFaceObservation{observation("dense-observation-2", 8, []float64{0.3, 0.4})},
			},
		},
	}
	if err := repository.CompleteDensePersonAnalysis(ctx, job, analysis); err != nil {
		t.Fatal(err)
	}
	var tracks, observations, embeddings int
	if err := database.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM person_tracks WHERE owner_user_id=$1 AND recording_id=$2),
  (SELECT COUNT(*) FROM face_track_observations WHERE owner_user_id=$1 AND recording_id=$2),
  (SELECT COUNT(*) FROM face_track_observation_embeddings WHERE owner_user_id=$1 AND recording_id=$2)`,
		ownerID, recording.ID,
	).Scan(&tracks, &observations, &embeddings); err != nil {
		t.Fatal(err)
	}
	if tracks != 2 || observations != 2 || embeddings != 2 {
		t.Fatalf("tracks=%d observations=%d embeddings=%d, want 2/2/2", tracks, observations, embeddings)
	}
}
