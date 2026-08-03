package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestOwnerAwareVoiceRepositoryPipelineIntegration(t *testing.T) {
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
	const ownerID = "voice-integration-owner"
	_, _ = database.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
	if _, err := database.ExecContext(ctx, `
INSERT INTO users (id, email, password_hash)
VALUES ($1, 'voice-integration@example.com', 'not-used')
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
	repository := newVoiceRepository(baseRepository)
	sample, err := repository.CreateEnrollmentSample(ctx, service.CreateEnrollmentSampleInput{
		OwnerUserID: ownerID, ProviderLabel: "owner_ref", FileName: "owner.wav",
		FilePath: "/tmp/owner.wav", MediaType: "audio/wav", SizeBytes: 100,
		DurationSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if samples, err := repository.ListEnrollmentSamples(ctx, ownerID); err != nil || len(samples) != 1 {
		t.Fatalf("enrollment samples = %+v, %v", samples, err)
	}

	session, err := repository.CreateRealtimeSession(ctx, service.StartRealtimeSessionInput{
		OwnerUserID: ownerID, MemoryID: "memory-1", GroupID: "group-1",
		ChunkDurationSeconds: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	chunkIndex := 0
	recording, err := repository.CreateRecording(ctx, service.CreateRecordingInput{
		OwnerUserID: ownerID, SessionID: session.ID, GroupID: session.GroupID,
		MemoryID: session.MemoryID, FileName: "chunk.wav", FilePath: "/tmp/chunk.wav",
		MediaType: "audio/wav", SizeBytes: 100, ChunkIndex: &chunkIndex,
		IsFinal: true, BatchID: session.BatchID,
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.StopRealtimeSession(ctx, session.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	sttJob, found, err := repository.ClaimJob(ctx)
	if err != nil || !found || sttJob.Kind != "stt" {
		t.Fatalf("STT job = %+v, found=%t, err=%v", sttJob, found, err)
	}
	transcript := service.Transcript{
		Text: "I prefer tea. Coffee is better.", Duration: 10,
		Segments: []service.TranscriptSegment{
			{ID: "seg-owner", StartTime: 1, EndTime: 3, Speaker: "owner_ref", SpeakerRole: "owner", Text: "I prefer tea."},
			{ID: "seg-other", StartTime: 4, EndTime: 6, Speaker: "A", SpeakerRole: "other", Text: "Coffee is better."},
		},
	}
	if err := repository.SaveTranscriptAndQueueAssembly(
		ctx, sttJob, transcript, []string{sample.ID}, "test", "diarize-test", 3,
	); err != nil {
		t.Fatal(err)
	}
	assemblyJob, found, err := repository.ClaimJob(ctx)
	if err != nil || !found || assemblyJob.Kind != "assemble" {
		t.Fatalf("assembly job = %+v, found=%t, err=%v", assemblyJob, found, err)
	}
	snapshot, err := repository.LoadAssembly(ctx, assemblyJob)
	if err != nil {
		t.Fatal(err)
	}
	episodes := service.BuildConversationEpisodes(snapshot, 8*time.Second, 2*time.Minute)
	if len(episodes) != 1 || episodes[0].OwnerUtteranceCount != 1 ||
		episodes[0].OtherUtteranceCount != 1 {
		t.Fatalf("episodes = %+v", episodes)
	}
	if err := repository.SaveAssembledEpisodes(ctx, assemblyJob, snapshot, episodes, 3); err != nil {
		t.Fatal(err)
	}

	memographJob, found, err := repository.ClaimJob(ctx)
	if err != nil || !found || memographJob.Kind != "memograph" {
		t.Fatalf("Memograph job = %+v, found=%t, err=%v", memographJob, found, err)
	}
	if memographJob.OwnerUtteranceCount != 1 || memographJob.OtherUtteranceCount != 1 ||
		len(memographJob.EpisodeSegments) != 2 {
		t.Fatalf("Memograph provenance = %+v", memographJob)
	}
	if err := repository.RetryJob(
		ctx, memographJob, "temporary graph failure", time.Now().Add(-time.Second), false,
	); err != nil {
		t.Fatal(err)
	}
	retriedJob, found, err := repository.ClaimJob(ctx)
	if err != nil || !found || retriedJob.Kind != "memograph" || retriedJob.ID != memographJob.ID {
		t.Fatalf("retried job = %+v, found=%t, err=%v", retriedJob, found, err)
	}
	if err := repository.CompleteMemographEpisode(
		ctx, retriedJob, json.RawMessage(`{"ok":true}`),
	); err != nil {
		t.Fatal(err)
	}
	detail, err := repository.GetRecording(ctx, recording.ID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != "completed" || len(detail.Episodes) != 1 ||
		detail.Episodes[0].OwnerUtteranceCount != 1 ||
		detail.Episodes[0].OtherUtteranceCount != 1 {
		t.Fatalf("recording detail = %+v", detail)
	}
}
