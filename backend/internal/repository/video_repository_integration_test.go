package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/arham/ai-second-brain/internal/service"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestVideoRepositoryPipelineIntegration(t *testing.T) {
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
	if err := database.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	const ownerID = "video-integration-owner"
	if _, err := database.ExecContext(ctx, `
INSERT INTO users (id, email, password_hash)
VALUES ($1, 'video-integration@example.com', 'not-used')
ON CONFLICT (id) DO NOTHING`, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	baseRepository, err := newBase(database)
	if err != nil {
		t.Fatal(err)
	}
	repository := newVideoRepository(baseRepository)
	session, err := repository.CreateVideoRealtimeSession(ctx, service.StartVideoRealtimeSessionInput{
		OwnerUserID: ownerID, MemoryID: "memory-1",
		ChunkDurationSeconds: 30, FrameIntervalSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.GroupID != "account-owner:"+ownerID {
		t.Fatalf("default graph group = %q", session.GroupID)
	}
	recording, err := repository.CreateRealtimeVideoChunk(
		ctx,
		service.CreateRealtimeVideoChunkInput{
			OwnerUserID: ownerID, RealtimeSessionID: session.ID,
			ClientChunkID: "550e8400-e29b-41d4-a716-446655440000",
			FileName:      "chunk.webm", FilePath: "/tmp/chunk.webm",
			MediaType: "video/webm", SizeBytes: 100,
		},
		3,
	)
	if err != nil {
		t.Fatal(err)
	}
	if recording.ChunkIndex == nil || *recording.ChunkIndex != 0 {
		t.Fatalf("recording chunk index = %v, want 0", recording.ChunkIndex)
	}

	first, found, err := repository.ClaimVideoJob(ctx)
	if err != nil || !found {
		t.Fatalf("claim first job = %+v, %t, %v", first, found, err)
	}
	second, found, err := repository.ClaimVideoJob(ctx)
	if err != nil || !found {
		t.Fatalf("claim second job = %+v, %t, %v", second, found, err)
	}
	for _, job := range []service.VideoJob{first, second} {
		switch job.Kind {
		case "audio":
			err = repository.SaveVideoTranscript(
				ctx, job,
				service.Transcript{Segments: []service.TranscriptSegment{{
					StartTime: 0, EndTime: 2, Speaker: "A", Text: "Hello",
				}}},
				[]string{"sample-1"}, "test", "test", 3,
			)
		case "visual":
			err = repository.SaveVideoAnalysis(
				ctx, job, 30,
				service.VisualAnalysis{Observations: []service.VideoObservation{{
					StartTime: 0, EndTime: 5, Summary: "A person is visible.",
				}}},
				"test", "test", 3,
			)
		default:
			t.Fatalf("initial job kind = %q, want audio or visual", job.Kind)
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	mergeJob, found, err := repository.ClaimVideoJob(ctx)
	if err != nil || !found || mergeJob.Kind != "merge" {
		t.Fatalf("merge job = %+v, %t, %v", mergeJob, found, err)
	}
	if len(mergeJob.Transcript.Segments) != 1 ||
		len(mergeJob.VisualAnalysis.Observations) != 1 {
		t.Fatalf("merge inputs = %+v / %+v", mergeJob.Transcript, mergeJob.VisualAnalysis)
	}
	if err := repository.SaveVideoEpisodes(ctx, mergeJob, []service.VideoEpisodeDraft{{
		BucketIndex: 0, StartTime: 0, EndTime: 5,
		Description:       "A said hello while a person was visible.",
		VisualDescription: "A person was visible.",
		SpeechDescription: "A Said : Hello",
		Visual:            mergeJob.VisualAnalysis.Observations,
	}}, 3); err != nil {
		t.Fatal(err)
	}

	sources := make(map[string]bool, 2)
	for index := 0; index < 2; index++ {
		memographJob, found, claimErr := repository.ClaimVideoJob(ctx)
		if claimErr != nil || !found || memographJob.Kind != "memograph" {
			t.Fatalf("memograph job %d = %+v, %t, %v", index, memographJob, found, claimErr)
		}
		if memographJob.VisualDescription != "A person was visible." ||
			memographJob.SpeechDescription != "A Said : Hello" {
			t.Fatalf("Memograph branch descriptions = %+v", memographJob)
		}
		sources[memographJob.MemographSource] = true
		if err := repository.CompleteVideoMemographBranch(
			ctx, memographJob, json.RawMessage(`{"ok":true}`),
		); err != nil {
			t.Fatal(err)
		}
	}
	if !sources["visual"] || !sources["speech"] {
		t.Fatalf("Memograph sources = %+v", sources)
	}
	detail, err := repository.GetVideoRecording(ctx, recording.ID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != "completed" || len(detail.Episodes) != 1 ||
		detail.Episodes[0].Status != "completed" {
		t.Fatalf("recording detail = %+v", detail)
	}
	if len(detail.SpeakerReferenceIDs) != 1 || detail.SpeakerReferenceIDs[0] != "sample-1" {
		t.Fatalf("speaker reference IDs = %+v", detail.SpeakerReferenceIDs)
	}
	if detail.STTProvider != "test" || detail.STTModel != "test" ||
		detail.VisualProvider != "test" || detail.VisualModel != "test" {
		t.Fatalf("recording providers = %+v", detail)
	}
}
