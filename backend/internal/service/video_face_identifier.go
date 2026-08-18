package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type VideoFaceIdentity struct {
	PersonProfileID string
	IdentityStatus  string
	DisplayName     string
	TrackID         string
	Pose            FacePose
	Similarity      *float64
}

type VideoFaceIdentifier interface {
	Identify(ctx context.Context, ownerUserID, recordingID string, processingVersion int, frames []VideoFrame) (map[string]VideoFaceIdentity, error)
}

type videoFaceIdentifier struct {
	repository PersonRepository
	recognizer FaceRecognizer
	config     config.FaceRecognitionConfig
}

type localFaceTrack struct {
	id         string
	start      float64
	end        float64
	lastFace   DetectedFace
	identity   VideoFaceIdentity
	evidence   []string
	confidence float64
}

func NewVideoFaceIdentifier(repository PersonRepository, recognizer FaceRecognizer, cfg config.FaceRecognitionConfig) VideoFaceIdentifier {
	if repository == nil || recognizer == nil {
		return nil
	}
	return &videoFaceIdentifier{repository: repository, recognizer: recognizer, config: cfg}
}

func (s *videoFaceIdentifier) Identify(
	ctx context.Context,
	ownerUserID, recordingID string,
	processingVersion int,
	frames []VideoFrame,
) (map[string]VideoFaceIdentity, error) {
	ordered := append([]VideoFrame(nil), frames...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Timestamp < ordered[j].Timestamp })
	identities := make(map[string]VideoFaceIdentity)
	var track *localFaceTrack
	var failures []error
	for _, frame := range ordered {
		result, err := s.recognizer.Recognize(ctx, FaceRecognitionInput{
			FileName: frame.FrameID + ".jpg", MediaType: frame.MediaType,
			Image: frame.Image, SingleFace: false,
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("recognize face in %s: %w", frame.FrameID, err))
			continue
		}
		if len(result.Faces) != 1 {
			track = nil
			continue
		}
		face := result.Faces[0]
		if !face.Quality.Usable || len(face.Embedding) == 0 || len(face.Embedding) != result.Dimensions {
			continue
		}

		continuing, trackScore := false, 0.0
		if track != nil {
			continuing, trackScore = sameFaceTrack(*track, frame.Timestamp, face, s.config.MatchThreshold)
			if !continuing {
				track = nil
			}
		}
		match, err := s.repository.MatchFace(ctx, MatchFaceProfileInput{
			OwnerUserID: ownerUserID, Provider: result.Provider,
			EmbeddingModel: result.Model, Embedding: face.Embedding,
			PoseBucket: face.Pose.Bucket, MatchThreshold: s.config.MatchThreshold,
			AmbiguousMargin: s.config.AmbiguousMargin,
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("match face in %s: %w", frame.FrameID, err))
			continue
		}

		identity := VideoFaceIdentity{Pose: face.Pose}
		if match.Matched {
			if continuing && track.identity.PersonProfileID != match.PersonProfileID {
				track = nil
				continue
			}
			identity = VideoFaceIdentity{
				PersonProfileID: match.PersonProfileID, IdentityStatus: match.IdentityStatus,
				DisplayName: match.DisplayName, Pose: face.Pose, Similarity: match.Similarity,
			}
		} else if continuing {
			identity = track.identity
			identity.Pose = face.Pose
			identity.Similarity = &trackScore
		} else if match.Ambiguous {
			continue
		}

		if track == nil {
			track = &localFaceTrack{
				id: faceTrackID(recordingID, frame.FrameID, processingVersion), start: frame.Timestamp,
				end: frame.Timestamp, confidence: face.DetectionScore,
			}
		}
		identity.TrackID = track.id
		if identity.PersonProfileID == "" {
			profile, enrollErr := s.remember(ctx, ownerUserID, recordingID, track.id, result, face, "")
			if enrollErr != nil {
				failures = append(failures, fmt.Errorf("create provisional face in %s: %w", frame.FrameID, enrollErr))
				continue
			}
			identity.PersonProfileID, identity.IdentityStatus, identity.DisplayName = profile.ID, profile.Status, profile.DisplayName
		} else {
			if _, rememberErr := s.remember(ctx, ownerUserID, recordingID, track.id, result, face, identity.PersonProfileID); rememberErr != nil {
				failures = append(failures, fmt.Errorf("extend face gallery in %s: %w", frame.FrameID, rememberErr))
			}
		}

		track.end = frame.Timestamp
		track.lastFace = face
		track.identity = identity
		track.evidence = append(track.evidence, frame.FrameID)
		if continuing {
			track.confidence = math.Min(track.confidence, trackScore)
		}
		identities[frame.FrameID] = identity
		if recordingID != "" {
			if saveErr := s.repository.SavePersonTrack(ctx, SavePersonTrackInput{
				ID: track.id, OwnerUserID: ownerUserID, RecordingID: recordingID,
				StartTime: track.start, EndTime: track.end,
				TemporaryVisualLabel:    anonymousFaceLabel(identity.PersonProfileID),
				ResolvedPersonProfileID: identity.PersonProfileID,
				TrackingConfidence:      track.confidence, EvidenceFrameIDs: track.evidence,
				ProcessingVersion: processingVersion,
			}); saveErr != nil {
				failures = append(failures, saveErr)
			}
		}
	}
	return identities, errors.Join(failures...)
}

func (s *videoFaceIdentifier) remember(
	ctx context.Context,
	ownerUserID, recordingID, trackID string,
	result FaceRecognition,
	face DetectedFace,
	personProfileID string,
) (PersonProfile, error) {
	return s.repository.EnrollFace(ctx, EnrollFaceProfileInput{
		OwnerUserID: ownerUserID, PersonProfileID: personProfileID, ConsentState: "pending",
		Provider: result.Provider, DetectorModel: result.Detector, EmbeddingModel: result.Model,
		Embedding: face.Embedding, DetectionScore: face.DetectionScore, Quality: face.Quality,
		Pose: face.Pose, SourceRecordingID: recordingID, SourceTrackID: trackID,
		ObservedAt: time.Now().UTC(), ProvisionalTTL: s.config.ProvisionalTTL,
	})
}

// ponytail: this conservative tracker reuses the sampled single-face pipeline;
// replace it with dense multi-face tracking when visual people expose bounding boxes.
func sameFaceTrack(track localFaceTrack, timestamp float64, face DetectedFace, threshold float64) (bool, float64) {
	if timestamp < track.end || timestamp-track.end > 15 || len(track.lastFace.Embedding) != len(face.Embedding) {
		return false, 0
	}
	score := cosine(track.lastFace.Embedding, face.Embedding)
	strict := score >= threshold+0.08
	spatiallyContinuous := boxIOU(track.lastFace.Box, face.Box) >= 0.35 && score >= threshold-0.08
	return strict || spatiallyContinuous, score
}

func cosine(left, right []float64) float64 {
	var dot, leftNorm, rightNorm float64
	for index := range left {
		dot += left[index] * right[index]
		leftNorm += left[index] * left[index]
		rightNorm += right[index] * right[index]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return -1
	}
	return dot / math.Sqrt(leftNorm*rightNorm)
}

func boxIOU(left, right FaceBox) float64 {
	x1, y1 := max(left.X, right.X), max(left.Y, right.Y)
	x2, y2 := min(left.X+left.Width, right.X+right.Width), min(left.Y+left.Height, right.Y+right.Height)
	intersection := max(0, x2-x1) * max(0, y2-y1)
	union := left.Width*left.Height + right.Width*right.Height - intersection
	if union <= 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func faceTrackID(recordingID, frameID string, processingVersion int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", recordingID, frameID, processingVersion)))
	return fmt.Sprintf("face-track:%x", digest[:16])
}

func anonymousFaceLabel(personProfileID string) string {
	if len(personProfileID) > 8 {
		personProfileID = personProfileID[:8]
	}
	return "Person " + personProfileID
}

var _ VideoFaceIdentifier = (*videoFaceIdentifier)(nil)
