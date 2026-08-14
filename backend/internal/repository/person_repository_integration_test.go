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
