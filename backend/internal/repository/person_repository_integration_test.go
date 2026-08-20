package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPersonRepositoryRejectsCrossAccountIdentityAccess(t *testing.T) {
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
	for _, user := range []struct{ id, email string }{{"face-owner-1", "face-owner-1@example.com"}, {"face-owner-2", "face-owner-2@example.com"}} {
		if _, err := database.ExecContext(ctx, `INSERT INTO users (id,email,password_hash) VALUES ($1,$2,'unused') ON CONFLICT (id) DO NOTHING`, user.id, user.email); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM users WHERE id IN ('face-owner-1','face-owner-2')`)
	})
	base, err := newBase(database)
	if err != nil {
		t.Fatal(err)
	}
	repository := newPersonRepository(base)
	person, err := repository.EnrollFace(ctx, service.EnrollFaceProfileInput{
		OwnerUserID: "face-owner-1", DisplayName: "Known person", Provider: "opencv",
		DetectorModel: "yunet", EmbeddingModel: "sface", Embedding: []float64{0.6, 0.8},
		FileName: "face.jpg", FilePath: "/tmp/face-owner-1.jpg", MediaType: "image/jpeg",
		SizeBytes: 100, DetectionScore: .99, Quality: service.FaceQuality{Usable: true},
		ProvisionalTTL: 30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	match, err := repository.MatchFace(ctx, service.MatchFaceProfileInput{
		OwnerUserID: "face-owner-1", Provider: "opencv", EmbeddingModel: "sface",
		Embedding: []float64{0.6, 0.8}, MatchThreshold: .7, AmbiguousMargin: .1,
	})
	if err != nil || !match.Matched || match.PersonProfileID != person.ID {
		t.Fatalf("owned match = %#v, error = %v", match, err)
	}
	crossAccountMatch, err := repository.MatchFace(ctx, service.MatchFaceProfileInput{
		OwnerUserID: "face-owner-2", Provider: "opencv", EmbeddingModel: "sface",
		Embedding: []float64{0.6, 0.8}, MatchThreshold: .7, AmbiguousMargin: .1,
	})
	if err != nil || crossAccountMatch.Matched {
		t.Fatalf("cross-account match = %#v, error = %v", crossAccountMatch, err)
	}
	_, err = repository.EnrollFace(ctx, service.EnrollFaceProfileInput{
		OwnerUserID: "face-owner-2", PersonProfileID: person.ID, Provider: "opencv",
		DetectorModel: "yunet", EmbeddingModel: "sface", Embedding: []float64{0.6, 0.8},
		FileName: "face.jpg", FilePath: "/tmp/face-owner-2.jpg", MediaType: "image/jpeg",
		SizeBytes: 100, DetectionScore: .99, Quality: service.FaceQuality{Usable: true},
		ProvisionalTTL: 30 * 24 * time.Hour,
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("cross-account enrollment error = %v, want not found", err)
	}
}

func TestPersonRepositoryConfirmsVoiceAndVisualAsOnePerson(t *testing.T) {
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
	const ownerID = "identity-confirm-owner"
	if _, err := database.ExecContext(ctx, `
	INSERT INTO users (id,email,password_hash) VALUES ($1,'identity-confirm@example.com','unused')`, ownerID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = database.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, ownerID) }()
	if _, err := database.ExecContext(ctx, `
INSERT INTO person_profiles (id,owner_user_id,status,expires_at)
VALUES ('face-person-mark',$1,'provisional',NOW()+INTERVAL '30 days');
INSERT INTO face_profiles (
    id,owner_user_id,person_profile_id,status,provider,detector_model,
    embedding_model,embedding_dimensions,centroid,expires_at
) VALUES (
    'face-profile-mark',$1,'face-person-mark','provisional','opencv','yunet',
    'sface',2,ARRAY[0.6,0.8],NOW()+INTERVAL '30 days'
)`, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
INSERT INTO voice_speaker_profiles (
    id,owner_user_id,status,display_name,embedding_model,embedding_dimensions,centroid
) VALUES ('speaker-mark',$1,'confirmed','Mark','test-model',2,ARRAY[0.6,0.8])`, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
INSERT INTO video_recordings (
    id,owner_user_id,session_id,group_id,memory_id,file_name,file_path,media_type,size_bytes,
    status,audio_status,visual_status,merge_status,processing_version,transcript,visual_analysis
) VALUES (
    'recording-mark',$1,'session-1','group-1','memory-1','mark.mp4','mark.mp4','video/mp4',100,
    'completed','completed','completed','completed',2,
    '{"segments":[{"id":"segment-1","speaker":"A","speaker_profile_id":"speaker-mark","speaker_name":"Mark","start_time":0,"end_time":2,"text":"Hello"}]}'::jsonb,
    '{"observations":[{"observation_id":"observation-1","frame_id":"frame-1","start_time":0,"end_time":2,"people":[{"visual_label":"person-1","person_track_id":"face-track-mark","person_profile_id":"face-person-mark","person_identity_status":"provisional"}]}]}'::jsonb
)`, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
INSERT INTO video_episodes (
    id,recording_id,bucket_index,start_time,end_time,description,visual_observations,
    status,evidence_kind,source_identity,processing_version
) VALUES
    ('visual-episode','recording-mark',0,0,2,'Mark is visible',
     '[{"observation_id":"observation-1","people":[{"visual_label":"person-1"}]}]'::jsonb,
     'completed','visual_evidence','visual-1',2),
    ('speech-episode','recording-mark',0,0,2,'Mark speaks','[]'::jsonb,
     'completed','speech_evidence','speech-1',2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
INSERT INTO video_jobs (kind,episode_id,status,max_attempts,source)
VALUES ('memograph','visual-episode','completed',3,'visual'),
       ('memograph','speech-episode','completed',3,'speech')`); err != nil {
		t.Fatal(err)
	}
	base, err := newBase(database)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := newPersonRepository(base).ConfirmIdentity(ctx, service.ConfirmPersonIdentityInput{
		OwnerUserID: ownerID, RecordingIDs: []string{"recording-mark"},
		VisualLabel: "person-1", VoiceSpeakerProfileID: "speaker-mark", Confirmed: true,
	})
	if err != nil || profile.ID != "face-person-mark" || profile.DisplayName != "Mark" {
		t.Fatalf("profile = %+v, error = %v", profile, err)
	}
	var speakerPersonID, transcriptPersonID, visualPersonID string
	if err := database.QueryRowContext(ctx, `
SELECT s.person_profile_id,
       r.transcript->'segments'->0->>'person_profile_id',
       r.visual_analysis->'observations'->0->'people'->0->>'person_profile_id'
FROM voice_speaker_profiles s CROSS JOIN video_recordings r
WHERE s.id='speaker-mark' AND r.id='recording-mark'`).Scan(
		&speakerPersonID, &transcriptPersonID, &visualPersonID,
	); err != nil {
		t.Fatal(err)
	}
	if speakerPersonID != profile.ID || transcriptPersonID != profile.ID || visualPersonID != profile.ID {
		t.Fatalf("canonical IDs = %q / %q / %q, want %q", speakerPersonID, transcriptPersonID, visualPersonID, profile.ID)
	}
	var faceStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM face_profiles WHERE id='face-profile-mark'`).Scan(&faceStatus); err != nil {
		t.Fatal(err)
	}
	if faceStatus != "confirmed" {
		t.Fatalf("face status = %q, want confirmed", faceStatus)
	}
	var tracks, acceptedLinks int
	if err := database.QueryRowContext(ctx, `
SELECT (SELECT COUNT(*) FROM person_tracks WHERE owner_user_id=$1),
       (SELECT COUNT(*) FROM identity_link_evidence WHERE owner_user_id=$1 AND decision='accepted')`, ownerID).Scan(&tracks, &acceptedLinks); err != nil {
		t.Fatal(err)
	}
	if tracks != 1 || acceptedLinks != 1 {
		t.Fatalf("tracks/accepted links = %d/%d", tracks, acceptedLinks)
	}
	var queuedJobs, revisedEpisodes int
	if err := database.QueryRowContext(ctx, `
SELECT (SELECT COUNT(*) FROM video_jobs j JOIN video_episodes e ON e.id=j.episode_id
        WHERE e.recording_id='recording-mark' AND j.status='queued'),
       (SELECT COUNT(*) FROM video_episodes WHERE recording_id='recording-mark' AND graph_revision=1 AND status='queued')`).Scan(
		&queuedJobs, &revisedEpisodes,
	); err != nil {
		t.Fatal(err)
	}
	if queuedJobs != 2 || revisedEpisodes != 2 {
		t.Fatalf("queued jobs/revised episodes = %d/%d", queuedJobs, revisedEpisodes)
	}
	speakers := newSpeakerProfileRepository(base)
	listed, err := speakers.ListSpeakerProfiles(ctx, ownerID)
	if err != nil || len(listed) != 1 || listed[0].PersonProfileID != profile.ID {
		t.Fatalf("linked speaker list = %+v, error = %v", listed, err)
	}
	resolution, err := speakers.ResolveSpeakerProfile(ctx, service.ResolveSpeakerProfileInput{
		OwnerUserID: ownerID, EmbeddingModel: "test-model", Embedding: []float64{0.6, 0.8},
		MatchThreshold: .5, AmbiguousMargin: .1, ProvisionalTTL: 30 * 24 * time.Hour,
	})
	if err != nil || resolution.Profile.PersonProfileID != profile.ID {
		t.Fatalf("future speaker resolution = %+v, error = %v", resolution, err)
	}
}
