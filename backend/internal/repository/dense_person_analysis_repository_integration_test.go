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
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	const ownerID = "dense-person-integration-owner"
	if _, err := database.ExecContext(ctx, `
INSERT INTO users (id,email,password_hash) VALUES ($1,'dense-person@example.com','not-used')
ON CONFLICT (id) DO NOTHING`, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM video_recordings WHERE owner_user_id=$1`, ownerID)
		_, _ = database.ExecContext(context.Background(), `DELETE FROM media_assets WHERE owner_user_id=$1`, ownerID)
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
	var stageCount int
	var dependenciesCorrect bool
	if err := database.QueryRowContext(ctx, `
SELECT COUNT(*),BOOL_AND(
    CASE stage
      WHEN 'audio_analysis' THEN cardinality(depends_on)=0
      WHEN 'dense_person_tracking' THEN cardinality(depends_on)=0
      WHEN 'transcription' THEN depends_on=ARRAY['audio_analysis']::TEXT[]
      WHEN 'identity_matching' THEN depends_on=ARRAY['dense_person_tracking','transcription']::TEXT[]
      WHEN 'active_speaker_fusion' THEN depends_on=ARRAY['identity_matching']::TEXT[]
      WHEN 'episode_generation' THEN depends_on=ARRAY['active_speaker_fusion']::TEXT[]
      WHEN 'graph_persistence' THEN depends_on=ARRAY['episode_generation']::TEXT[]
      ELSE FALSE
    END
)
FROM analysis_stage_jobs job
JOIN analysis_runs run ON run.id=job.analysis_run_id
WHERE run.recording_id=$1`, recording.ID).Scan(&stageCount, &dependenciesCorrect); err != nil {
		t.Fatal(err)
	}
	if stageCount != 7 || !dependenciesCorrect {
		t.Fatalf("analysis stages=%d dependencies correct=%t, want 7/true", stageCount, dependenciesCorrect)
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
	owners, err := videoRepository.PipelineDebugOwners(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var foundDebugOwner bool
	for _, owner := range owners {
		if owner.ID == ownerID && owner.RecordingCount == 1 && owner.RunCount == 1 {
			foundDebugOwner = true
		}
	}
	if !foundDebugOwner {
		t.Fatalf("pipeline debug owners = %+v", owners)
	}
	debugOverview, err := videoRepository.PipelineDebugAnalysisOverview(ctx, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(debugOverview.Runs) != 1 || len(debugOverview.Runs[0].Stages) != 7 {
		t.Fatalf("pipeline debug overview = %+v", debugOverview)
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
