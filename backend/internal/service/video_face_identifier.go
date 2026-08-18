package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
)

type VideoFaceIdentity struct {
	PersonProfileID string
	IdentityStatus  string
	DisplayName     string
	Similarity      *float64
}

type VideoFaceIdentifier interface {
	Identify(ctx context.Context, ownerUserID string, frames []VideoFrame) (map[string]VideoFaceIdentity, error)
}

type videoFaceIdentifier struct {
	repository PersonRepository
	recognizer FaceRecognizer
	store      AudioStore
	config     config.FaceRecognitionConfig
}

func NewVideoFaceIdentifier(
	repository PersonRepository,
	recognizer FaceRecognizer,
	store AudioStore,
	cfg config.FaceRecognitionConfig,
) VideoFaceIdentifier {
	if repository == nil || recognizer == nil || store == nil {
		return nil
	}
	return &videoFaceIdentifier{repository: repository, recognizer: recognizer, store: store, config: cfg}
}

func (s *videoFaceIdentifier) Identify(
	ctx context.Context,
	ownerUserID string,
	frames []VideoFrame,
) (map[string]VideoFaceIdentity, error) {
	identities := make(map[string]VideoFaceIdentity)
	var failures []error
	for _, frame := range frames {
		result, err := s.recognizer.Recognize(ctx, FaceRecognitionInput{
			FileName: frame.FrameID + ".jpg", MediaType: frame.MediaType,
			Image: frame.Image, SingleFace: false,
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("recognize face in %s: %w", frame.FrameID, err))
			continue
		}
		if len(result.Faces) != 1 {
			continue
		}
		face := result.Faces[0]
		if !face.Quality.Usable || len(face.Embedding) == 0 || len(face.Embedding) != result.Dimensions {
			continue
		}
		match, err := s.repository.MatchFace(ctx, MatchFaceProfileInput{
			OwnerUserID: ownerUserID, Provider: result.Provider,
			EmbeddingModel: result.Model, Embedding: face.Embedding,
			MatchThreshold: s.config.MatchThreshold, AmbiguousMargin: s.config.AmbiguousMargin,
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("match face in %s: %w", frame.FrameID, err))
			continue
		}
		if match.Matched {
			identities[frame.FrameID] = VideoFaceIdentity{
				PersonProfileID: match.PersonProfileID, IdentityStatus: match.IdentityStatus,
				DisplayName: match.DisplayName, Similarity: match.Similarity,
			}
			continue
		}
		if match.Ambiguous {
			continue
		}
		crop, err := cropFaceJPEG(frame.Image, face.Box)
		if err != nil {
			failures = append(failures, fmt.Errorf("crop face in %s: %w", frame.FrameID, err))
			continue
		}
		fileName := "face-" + strings.TrimSpace(frame.FrameID) + ".jpg"
		stored, err := s.store.Save(ctx, fileName, bytes.NewReader(crop))
		if err != nil {
			failures = append(failures, fmt.Errorf("store face in %s: %w", frame.FrameID, err))
			continue
		}
		profile, err := s.repository.EnrollFace(ctx, EnrollFaceProfileInput{
			OwnerUserID: ownerUserID, ConsentState: "pending",
			Provider: result.Provider, DetectorModel: result.Detector, EmbeddingModel: result.Model,
			Embedding: face.Embedding, FileName: fileName, FilePath: stored.Path,
			MediaType: "image/jpeg", SizeBytes: stored.SizeBytes,
			DetectionScore: face.DetectionScore, Quality: face.Quality,
			ProvisionalTTL: s.config.ProvisionalTTL,
		})
		if err != nil {
			_ = s.store.Delete(context.Background(), stored.Path)
			failures = append(failures, fmt.Errorf("enroll face in %s: %w", frame.FrameID, err))
			continue
		}
		identities[frame.FrameID] = VideoFaceIdentity{
			PersonProfileID: profile.ID, IdentityStatus: profile.Status, DisplayName: profile.DisplayName,
		}
	}
	return identities, errors.Join(failures...)
}

func cropFaceJPEG(payload []byte, box FaceBox) ([]byte, error) {
	imageValue, _, err := image.Decode(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	bounds := imageValue.Bounds()
	rect := image.Rect(box.X, box.Y, box.X+box.Width, box.Y+box.Height).Intersect(bounds)
	if rect.Empty() {
		return nil, fmt.Errorf("face box is outside image bounds")
	}
	cropped, ok := imageValue.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("image does not support cropping")
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, cropped.SubImage(rect), &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

var _ VideoFaceIdentifier = (*videoFaceIdentifier)(nil)
