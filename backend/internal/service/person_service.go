package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type personService struct {
	repository PersonRepository
	recognizer FaceRecognizer
	store      AudioStore
	config     config.FaceRecognitionConfig
}

func newPersonService(repository PersonRepository, recognizer FaceRecognizer, store AudioStore, cfg config.FaceRecognitionConfig) *personService {
	return &personService{repository: repository, recognizer: recognizer, store: store, config: cfg}
}

func (s *personService) EnrollFace(ctx context.Context, input FaceEnrollmentInput) (PersonProfile, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.PersonProfileID = strings.TrimSpace(input.PersonProfileID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.RelationshipCategory = strings.ToLower(strings.TrimSpace(input.RelationshipCategory))
	input.RelationshipLabel = strings.TrimSpace(input.RelationshipLabel)
	input.FileName = filepath.Base(strings.TrimSpace(input.FileName))
	if input.OwnerUserID == "" || input.Content == nil || input.FileName == "" || input.FileName == "." {
		return PersonProfile{}, validation("file", "an owned, named face image is required")
	}
	if !input.ConsentConfirmed {
		return PersonProfile{}, validation("consent_confirmed", "must be true before biometric enrollment")
	}
	if s.recognizer == nil {
		return PersonProfile{}, fmt.Errorf("face recognition is not configured")
	}
	if err := validatePersonLabel(input.DisplayName, input.RelationshipCategory, input.RelationshipLabel, false); err != nil {
		return PersonProfile{}, err
	}
	payload, err := readFaceImage(input.Content, s.config.MaxUploadBytes)
	if err != nil {
		return PersonProfile{}, err
	}
	result, err := s.recognizer.Recognize(ctx, FaceRecognitionInput{
		FileName: input.FileName, MediaType: input.MediaType, Image: payload, SingleFace: true,
	})
	if err != nil {
		return PersonProfile{}, fmt.Errorf("analyze enrollment face: %w", err)
	}
	if len(result.Faces) != 1 || !result.Faces[0].Quality.Usable || len(result.Faces[0].Embedding) != result.Dimensions {
		return PersonProfile{}, validation("file", "image does not contain exactly one quality-approved face")
	}
	stored, err := s.store.Save(ctx, input.FileName, bytes.NewReader(payload))
	if err != nil {
		return PersonProfile{}, validation("file", err.Error())
	}
	face := result.Faces[0]
	profile, err := s.repository.EnrollFace(ctx, EnrollFaceProfileInput{
		OwnerUserID: input.OwnerUserID, PersonProfileID: input.PersonProfileID,
		DisplayName: input.DisplayName, RelationshipCategory: input.RelationshipCategory,
		RelationshipLabel: input.RelationshipLabel, ConsentState: "granted", Provider: result.Provider,
		DetectorModel: result.Detector, EmbeddingModel: result.Model,
		Embedding: face.Embedding, FileName: input.FileName, FilePath: stored.Path,
		MediaType: input.MediaType, SizeBytes: stored.SizeBytes,
		DetectionScore: face.DetectionScore, Quality: face.Quality,
		Pose: face.Pose, ObservedAt: time.Now().UTC(),
		ProvisionalTTL: s.config.ProvisionalTTL,
	})
	if err != nil {
		_ = s.store.Delete(context.Background(), stored.Path)
		return PersonProfile{}, err
	}
	return profile, nil
}

func (s *personService) RecognizeFace(ctx context.Context, input FaceRecognitionRequest) (FaceMatch, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.FileName = filepath.Base(strings.TrimSpace(input.FileName))
	if input.OwnerUserID == "" || input.Content == nil || input.FileName == "" || input.FileName == "." {
		return FaceMatch{}, validation("file", "an owned, named face image is required")
	}
	if s.recognizer == nil {
		return FaceMatch{}, fmt.Errorf("face recognition is not configured")
	}
	payload, err := readFaceImage(input.Content, s.config.MaxUploadBytes)
	if err != nil {
		return FaceMatch{}, err
	}
	result, err := s.recognizer.Recognize(ctx, FaceRecognitionInput{
		FileName: input.FileName, MediaType: input.MediaType, Image: payload, SingleFace: true,
	})
	if err != nil {
		return FaceMatch{}, fmt.Errorf("analyze recognition face: %w", err)
	}
	if len(result.Faces) != 1 {
		return FaceMatch{Reasons: []string{"exactly_one_face_required"}}, nil
	}
	face := result.Faces[0]
	if !face.Quality.Usable {
		return FaceMatch{Quality: face.Quality, Reasons: append([]string(nil), face.Quality.Reasons...)}, nil
	}
	match, err := s.repository.MatchFace(ctx, MatchFaceProfileInput{
		OwnerUserID: input.OwnerUserID, Provider: result.Provider,
		EmbeddingModel: result.Model, Embedding: face.Embedding,
		PoseBucket:     face.Pose.Bucket,
		MatchThreshold: s.config.MatchThreshold, AmbiguousMargin: s.config.AmbiguousMargin,
	})
	match.Quality = face.Quality
	return match, err
}

func (s *personService) ListPeople(ctx context.Context, ownerUserID string) ([]PersonProfile, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, validation("owner_user_id", "is required")
	}
	return s.repository.ListPeople(ctx, ownerUserID)
}

func (s *personService) UpdatePerson(ctx context.Context, input UpdatePersonInput) (PersonProfile, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.RelationshipCategory = strings.ToLower(strings.TrimSpace(input.RelationshipCategory))
	input.RelationshipLabel = strings.TrimSpace(input.RelationshipLabel)
	if input.ID == "" || input.OwnerUserID == "" {
		return PersonProfile{}, validation("person_profile_id", "is required")
	}
	if err := validatePersonLabel(input.DisplayName, input.RelationshipCategory, input.RelationshipLabel, true); err != nil {
		return PersonProfile{}, err
	}
	return s.repository.UpdatePerson(ctx, input)
}

func (s *personService) ConfirmIdentity(
	ctx context.Context,
	input ConfirmPersonIdentityInput,
) (PersonProfile, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.VisualLabel = strings.TrimSpace(input.VisualLabel)
	input.VoiceSpeakerProfileID = strings.TrimSpace(input.VoiceSpeakerProfileID)
	uniqueRecordings := make([]string, 0, len(input.RecordingIDs))
	seen := make(map[string]bool, len(input.RecordingIDs))
	for _, recordingID := range input.RecordingIDs {
		recordingID = strings.TrimSpace(recordingID)
		if recordingID != "" && !seen[recordingID] {
			seen[recordingID] = true
			uniqueRecordings = append(uniqueRecordings, recordingID)
		}
	}
	input.RecordingIDs = uniqueRecordings
	if input.OwnerUserID == "" || input.VoiceSpeakerProfileID == "" ||
		input.VisualLabel == "" || len(input.RecordingIDs) == 0 {
		return PersonProfile{}, validation("identity_link", "voice profile, visual label, and recording IDs are required")
	}
	if !input.Confirmed {
		return PersonProfile{}, validation("confirmed", "must be true for an explicit identity link")
	}
	return s.repository.ConfirmIdentity(ctx, input)
}

func (s *personService) DeletePerson(ctx context.Context, id, ownerUserID string) error {
	id, ownerUserID = strings.TrimSpace(id), strings.TrimSpace(ownerUserID)
	if id == "" || ownerUserID == "" {
		return validation("person_profile_id", "is required")
	}
	paths, err := s.repository.DeletePerson(ctx, id, ownerUserID)
	if err != nil {
		return err
	}
	var deleteErrors []error
	for _, path := range paths {
		if err := s.store.Delete(ctx, path); err != nil {
			deleteErrors = append(deleteErrors, err)
		}
	}
	if err := errors.Join(deleteErrors...); err != nil {
		return fmt.Errorf("delete retained face samples: %w", err)
	}
	return nil
}

func readFaceImage(reader io.Reader, limit int64) ([]byte, error) {
	if limit < 1 {
		limit = 10 << 20
	}
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, validation("file", "face image could not be read")
	}
	if len(payload) == 0 {
		return nil, validation("file", "face image is empty")
	}
	if int64(len(payload)) > limit {
		return nil, validation("file", "face image exceeds the configured limit")
	}
	return payload, nil
}

func validatePersonLabel(name, category, label string, requireName bool) error {
	if (requireName && name == "") || len([]rune(name)) > 100 {
		return validation("display_name", "must be between 1 and 100 characters")
	}
	valid := map[string]bool{
		"": true, "family": true, "friend": true, "colleague": true,
		"professional": true, "acquaintance": true, "other": true,
	}
	if !valid[category] {
		return validation("relationship_category", "is invalid")
	}
	if len([]rune(label)) > 100 {
		return validation("relationship_label", "must not exceed 100 characters")
	}
	return nil
}

var _ PersonService = (*personService)(nil)
