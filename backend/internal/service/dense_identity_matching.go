package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	denseSceneTimestampTolerance = 0.30
	denseSceneMinimumIOU         = 0.25
	denseSceneAmbiguousMargin    = 0.10
)

func (s *videoService) matchDenseTrackIdentities(ctx context.Context, job *VideoJob) error {
	if s.personRepository == nil {
		return fmt.Errorf("dense identity matching requires a person repository")
	}
	identities := make(map[string]VideoFaceIdentity, len(job.DenseIdentityTracks))
	for _, track := range job.DenseIdentityTracks {
		identity, err := s.matchDenseTrack(ctx, *job, track)
		if err != nil {
			return err
		}
		identities[track.ID] = identity
		if err := s.personRepository.SavePersonTrack(ctx, SavePersonTrackInput{
			ID: track.ID, OwnerUserID: job.OwnerUserID, RecordingID: job.RecordingID,
			StartTime: track.StartTime, EndTime: track.EndTime,
			TemporaryVisualLabel:    temporaryFaceLabel(track.ID),
			ResolvedPersonProfileID: identity.PersonProfileID,
			TrackingConfidence:      track.TrackingConfidence,
			EvidenceFrameIDs:        track.GalleryObservationIDs,
			ProcessingVersion:       job.ProcessingVersion,
		}); err != nil {
			return fmt.Errorf("save dense identity for track %q: %w", track.ID, err)
		}
	}
	mapScenePeopleToDenseTracks(&job.VisualAnalysis, job.DenseIdentityTracks, identities)
	return nil
}

func (s *videoService) matchDenseTrack(
	ctx context.Context,
	job VideoJob,
	track DenseIdentityTrack,
) (VideoFaceIdentity, error) {
	identity := VideoFaceIdentity{TrackID: track.ID, Outcome: "ambiguous"}
	if track.EmbeddingModel == "" || track.EmbeddingModel != s.faceConfig.Model {
		return identity, fmt.Errorf(
			"dense track %q uses embedding model %q, face gallery requires %q",
			track.ID, track.EmbeddingModel, s.faceConfig.Model,
		)
	}
	gallery := denseGalleryObservations(track)
	if len(gallery) == 0 {
		return identity, nil
	}

	var matched *FaceMatch
	var lowestSimilarity *float64
	for _, observation := range gallery {
		match, err := s.personRepository.MatchFace(ctx, MatchFaceProfileInput{
			OwnerUserID: job.OwnerUserID, Provider: track.Provider,
			EmbeddingModel: track.EmbeddingModel, Embedding: observation.Embedding,
			PoseBucket:      observation.Pose.Bucket,
			MatchThreshold:  s.faceConfig.MatchThreshold,
			AmbiguousMargin: s.faceConfig.AmbiguousMargin,
		})
		if err != nil {
			return identity, fmt.Errorf("match dense face in %q: %w", observation.ObservationID, err)
		}
		if match.Ambiguous {
			return identity, nil
		}
		if !match.Matched {
			continue
		}
		if matched != nil && matched.PersonProfileID != match.PersonProfileID {
			return identity, nil
		}
		if matched == nil {
			copy := match
			matched = &copy
		}
		if match.Similarity != nil {
			if lowestSimilarity == nil || *match.Similarity < *lowestSimilarity {
				score := *match.Similarity
				lowestSimilarity = &score
			}
		}
	}

	personProfileID := ""
	if matched != nil {
		personProfileID = matched.PersonProfileID
		identity.PersonProfileID = matched.PersonProfileID
		identity.IdentityStatus = matched.IdentityStatus
		identity.DisplayName = matched.DisplayName
		identity.Outcome = "attached"
		identity.Similarity = lowestSimilarity
	} else if len(gallery) < 2 {
		return identity, nil
	}

	for _, observation := range gallery {
		profile, err := s.personRepository.EnrollFace(ctx, EnrollFaceProfileInput{
			OwnerUserID: job.OwnerUserID, PersonProfileID: personProfileID,
			ConsentState: "pending", Provider: track.Provider,
			DetectorModel: track.DetectorModel, EmbeddingModel: track.EmbeddingModel,
			Embedding: observation.Embedding, DetectionScore: observation.DetectionScore,
			Quality: observation.Quality, Pose: observation.Pose,
			SourceRecordingID: job.RecordingID, SourceTrackID: track.ID,
			ObservedAt: time.Now().UTC(), ProvisionalTTL: s.faceConfig.ProvisionalTTL,
		})
		if err != nil {
			return identity, fmt.Errorf("enroll dense face in %q: %w", observation.ObservationID, err)
		}
		if personProfileID == "" {
			personProfileID = profile.ID
			identity.PersonProfileID = profile.ID
			identity.IdentityStatus = profile.Status
			identity.DisplayName = profile.DisplayName
			identity.Outcome = "provisional"
		}
	}
	return identity, nil
}

func denseGalleryObservations(track DenseIdentityTrack) []DenseFaceObservation {
	selected := make(map[string]bool, len(track.GalleryObservationIDs))
	for _, id := range track.GalleryObservationIDs {
		selected[id] = true
	}
	gallery := make([]DenseFaceObservation, 0, len(selected))
	for _, observation := range track.Observations {
		if selected[observation.ObservationID] && len(observation.Embedding) > 0 {
			gallery = append(gallery, observation)
		}
	}
	return gallery
}

type denseSceneCandidate struct {
	personIndex int
	trackIndex  int
	score       float64
}

func mapScenePeopleToDenseTracks(
	analysis *VisualAnalysis,
	tracks []DenseIdentityTrack,
	identities map[string]VideoFaceIdentity,
) {
	if analysis == nil {
		return
	}
	for observationIndex := range analysis.Observations {
		observation := &analysis.Observations[observationIndex]
		candidates := make([]denseSceneCandidate, 0)
		// ponytail: scenes contain few faces; a direct person-by-track scan is simpler
		// than maintaining a second spatial index beside the dense tracker.
		for personIndex, person := range observation.People {
			if !person.PhysicalPresence || !person.FaceVisible || person.FaceBox == nil {
				continue
			}
			for trackIndex, track := range tracks {
				dense, ok := closestDenseObservation(track.Observations, observation.StartTime)
				if !ok {
					continue
				}
				score := boxIOU(*person.FaceBox, dense.Box)
				if score >= denseSceneMinimumIOU {
					candidates = append(candidates, denseSceneCandidate{
						personIndex: personIndex, trackIndex: trackIndex, score: score,
					})
				}
			}
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
		for _, candidate := range candidates {
			if !uniqueDenseSceneCandidate(candidate, candidates) {
				continue
			}
			track := tracks[candidate.trackIndex]
			identity := identities[track.ID]
			person := &observation.People[candidate.personIndex]
			person.PersonTrackID = track.ID
			person.FaceIdentityOutcome = identity.Outcome
			person.PersonProfileID = identity.PersonProfileID
			person.PersonIdentityStatus = identity.IdentityStatus
			person.PersonName = identity.DisplayName
			person.FaceMatchConfidence = identity.Similarity
		}
	}
}

func closestDenseObservation(observations []DenseFaceObservation, timestamp float64) (DenseFaceObservation, bool) {
	var closest DenseFaceObservation
	distance := math.Inf(1)
	for _, observation := range observations {
		candidateDistance := math.Abs(observation.Timestamp - timestamp)
		if candidateDistance < distance {
			closest, distance = observation, candidateDistance
		}
	}
	return closest, distance <= denseSceneTimestampTolerance
}

func uniqueDenseSceneCandidate(candidate denseSceneCandidate, candidates []denseSceneCandidate) bool {
	for _, other := range candidates {
		if other.personIndex == candidate.personIndex && other.trackIndex != candidate.trackIndex &&
			candidate.score-other.score < denseSceneAmbiguousMargin {
			return false
		}
		if other.trackIndex == candidate.trackIndex && other.personIndex != candidate.personIndex &&
			candidate.score-other.score < denseSceneAmbiguousMargin {
			return false
		}
		if other.personIndex == candidate.personIndex && other.score > candidate.score {
			return false
		}
		if other.trackIndex == candidate.trackIndex && other.score > candidate.score {
			return false
		}
	}
	return true
}
