package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type persistentSpeakerIdentifier struct {
	repository SpeakerProfileRepository
	embedder   SpeakerEmbedder
	clipper    SpeakerClipper
	store      AudioStore
	minClip    time.Duration
	maxClip    time.Duration
	threshold  float64
	margin     float64
	ttl        time.Duration
}

func NewPersistentSpeakerIdentifier(
	repository SpeakerProfileRepository,
	embedder SpeakerEmbedder,
	clipper SpeakerClipper,
	store AudioStore,
	cfg config.SpeakerEmbeddingConfig,
) (SpeakerIdentifier, error) {
	if repository == nil || embedder == nil || clipper == nil || store == nil {
		return nil, fmt.Errorf("persistent speaker identification dependencies are required")
	}
	if cfg.MinClipDuration < 2*time.Second || cfg.MaxClipDuration < cfg.MinClipDuration ||
		cfg.MaxClipDuration > 10*time.Second || cfg.ProvisionalTTL <= 0 ||
		cfg.MatchThreshold <= 0 || cfg.MatchThreshold > 1 ||
		cfg.AmbiguousMargin < 0 || cfg.AmbiguousMargin >= 1 {
		return nil, fmt.Errorf("persistent speaker identification configuration is invalid")
	}
	return &persistentSpeakerIdentifier{
		repository: repository, embedder: embedder, clipper: clipper, store: store,
		minClip: cfg.MinClipDuration, maxClip: cfg.MaxClipDuration,
		threshold: cfg.MatchThreshold, margin: cfg.AmbiguousMargin,
		ttl: cfg.ProvisionalTTL,
	}, nil
}

func (s *persistentSpeakerIdentifier) Identify(
	ctx context.Context, input SpeakerIdentificationInput,
) (Transcript, error) {
	transcript := input.Transcript
	if strings.TrimSpace(input.OwnerUserID) == "" || strings.TrimSpace(input.AudioPath) == "" ||
		(input.SourceKind != "voice" && input.SourceKind != "video") ||
		strings.TrimSpace(input.SourceRecordingID) == "" {
		return transcript, fmt.Errorf("speaker identification input is invalid")
	}
	groups := speakerGroups(transcript.Segments)
	if len(groups) == 0 {
		return transcript, nil
	}
	if err := purgeExpiredSpeakerFiles(ctx, s.repository, s.store, input.OwnerUserID); err != nil {
		return transcript, fmt.Errorf("purge expired speaker profiles: %w", err)
	}

	profiles, err := s.repository.ListSpeakerProfiles(ctx, input.OwnerUserID)
	if err != nil {
		return transcript, fmt.Errorf("load speaker profiles: %w", err)
	}
	profilesByID := make(map[string]SpeakerProfile, len(profiles))
	for _, profile := range profiles {
		profilesByID[profile.ID] = profile
	}

	var identificationErrors []error
	for _, group := range groups {
		observation, found, err := s.repository.FindSpeakerObservation(
			ctx, input.OwnerUserID, input.SourceKind, input.SourceRecordingID, group.providerSpeaker,
		)
		if err != nil {
			identificationErrors = append(identificationErrors, err)
			continue
		}
		if found {
			if profile, exists := profilesByID[observation.ProfileID]; exists {
				applySpeakerProfile(&transcript, group.providerSpeaker, profile)
			}
			continue
		}
		if group.duration < s.minClip.Seconds() {
			continue
		}
		clip, err := s.clipper.ExtractSpeakerClip(ctx, input.AudioPath, group.ranges, s.maxClip)
		if err != nil {
			identificationErrors = append(identificationErrors, fmt.Errorf("extract %s speaker clip: %w", group.providerSpeaker, err))
			continue
		}
		if clip.DurationSeconds < s.minClip.Seconds() || clip.DurationSeconds > s.maxClip.Seconds()+0.01 {
			identificationErrors = append(identificationErrors, fmt.Errorf("speaker %s clip duration is outside configured bounds", group.providerSpeaker))
			continue
		}
		embedding, err := s.embedder.Embed(ctx, SpeakerEmbeddingInput{
			FileName: clip.FileName, MediaType: clip.MediaType, Audio: clip.Audio,
		})
		if err != nil {
			identificationErrors = append(identificationErrors, fmt.Errorf("embed %s speaker clip: %w", group.providerSpeaker, err))
			continue
		}
		resolution, err := s.repository.ResolveSpeakerProfile(ctx, ResolveSpeakerProfileInput{
			OwnerUserID: input.OwnerUserID, EmbeddingModel: embedding.Model,
			Embedding: embedding.Vector, MatchThreshold: s.threshold,
			AmbiguousMargin: s.margin, ProvisionalTTL: s.ttl,
		})
		if err != nil {
			identificationErrors = append(identificationErrors, fmt.Errorf("resolve %s speaker: %w", group.providerSpeaker, err))
			continue
		}
		if resolution.Created {
			stored, saveErr := s.store.Save(ctx, clip.FileName, bytes.NewReader(clip.Audio))
			if saveErr != nil {
				_, _ = s.repository.DeleteSpeakerProfile(ctx, resolution.Profile.ID, input.OwnerUserID)
				identificationErrors = append(identificationErrors, fmt.Errorf("store %s speaker sample: %w", group.providerSpeaker, saveErr))
				continue
			}
			_, sampleErr := s.repository.CreateSpeakerSample(ctx, CreateSpeakerSampleInput{
				OwnerUserID: input.OwnerUserID, ProfileID: resolution.Profile.ID,
				SourceKind: input.SourceKind, SourceRecordingID: input.SourceRecordingID,
				ProviderSpeaker: group.providerSpeaker, FileName: clip.FileName,
				FilePath: stored.Path, MediaType: clip.MediaType, SizeBytes: stored.SizeBytes,
				DurationSeconds: clip.DurationSeconds,
			})
			if sampleErr != nil {
				_ = s.store.Delete(context.Background(), stored.Path)
				_, _ = s.repository.DeleteSpeakerProfile(ctx, resolution.Profile.ID, input.OwnerUserID)
				identificationErrors = append(identificationErrors, fmt.Errorf("record %s speaker sample: %w", group.providerSpeaker, sampleErr))
				continue
			}
		}
		outcome := "matched"
		if resolution.Created {
			outcome = "created"
		}
		_, err = s.repository.CreateSpeakerObservation(ctx, CreateSpeakerObservationInput{
			OwnerUserID: input.OwnerUserID, ProfileID: resolution.Profile.ID,
			SourceKind: input.SourceKind, SourceRecordingID: input.SourceRecordingID,
			ProviderSpeaker: group.providerSpeaker, SegmentIDs: group.segmentIDs,
			Outcome: outcome, Similarity: resolution.Similarity,
			RunnerUpSimilarity: resolution.RunnerUpSimilarity,
		})
		if err != nil {
			identificationErrors = append(identificationErrors, fmt.Errorf("record %s speaker observation: %w", group.providerSpeaker, err))
			continue
		}
		profilesByID[resolution.Profile.ID] = resolution.Profile
		applySpeakerProfile(&transcript, group.providerSpeaker, resolution.Profile)
	}
	return transcript, errors.Join(identificationErrors...)
}

type groupedSpeaker struct {
	providerSpeaker string
	ranges          []AudioRange
	segmentIDs      []string
	duration        float64
}

func speakerGroups(segments []TranscriptSegment) []groupedSpeaker {
	groups := make(map[string]*groupedSpeaker)
	for _, segment := range segments {
		speaker := strings.TrimSpace(segment.Speaker)
		if speaker == "" || segment.SpeakerRole == "owner" || segment.EndTime <= segment.StartTime {
			continue
		}
		group := groups[speaker]
		if group == nil {
			group = &groupedSpeaker{providerSpeaker: speaker}
			groups[speaker] = group
		}
		group.ranges = append(group.ranges, AudioRange{StartTime: segment.StartTime, EndTime: segment.EndTime})
		if segment.ID != "" {
			group.segmentIDs = append(group.segmentIDs, segment.ID)
		}
		group.duration += segment.EndTime - segment.StartTime
	}
	result := make([]groupedSpeaker, 0, len(groups))
	for _, group := range groups {
		sort.Slice(group.ranges, func(i, j int) bool { return group.ranges[i].StartTime < group.ranges[j].StartTime })
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].providerSpeaker < result[j].providerSpeaker })
	return result
}

func applySpeakerProfile(transcript *Transcript, providerSpeaker string, profile SpeakerProfile) {
	relationship := strings.TrimSpace(profile.RelationshipLabel)
	if relationship == "" {
		relationship = strings.TrimSpace(profile.RelationshipCategory)
	}
	for index := range transcript.Segments {
		segment := &transcript.Segments[index]
		if segment.Speaker != providerSpeaker || segment.SpeakerRole == "owner" {
			continue
		}
		segment.SpeakerRole = "other"
		segment.SpeakerProfileID = profile.ID
		segment.SpeakerIdentityStatus = profile.Status
		if profile.Status == "confirmed" {
			segment.SpeakerName = profile.DisplayName
			segment.SpeakerRelationship = relationship
		}
	}
}

func enrichTranscriptSpeakerProfiles(transcript *Transcript, profiles []SpeakerProfile) {
	if transcript == nil {
		return
	}
	byID := make(map[string]SpeakerProfile, len(profiles))
	for _, profile := range profiles {
		byID[profile.ID] = profile
	}
	for index := range transcript.Segments {
		segment := &transcript.Segments[index]
		if segment.SpeakerProfileID == "" {
			continue
		}
		profile, found := byID[segment.SpeakerProfileID]
		if !found {
			segment.SpeakerProfileID = ""
			segment.SpeakerName = ""
			segment.SpeakerRelationship = ""
			segment.SpeakerIdentityStatus = ""
			continue
		}
		applySpeakerProfile(transcript, segment.Speaker, profile)
	}
}

func enrichEpisodeSpeakerProfile(segment *EpisodeSegment, profiles []SpeakerProfile) {
	if segment == nil || segment.SpeakerProfileID == "" {
		return
	}
	for _, profile := range profiles {
		if profile.ID != segment.SpeakerProfileID {
			continue
		}
		segment.SpeakerIdentityStatus = profile.Status
		segment.SpeakerName = ""
		segment.SpeakerRelationship = ""
		if profile.Status == "confirmed" {
			segment.SpeakerName = profile.DisplayName
			segment.SpeakerRelationship = profile.RelationshipLabel
			if segment.SpeakerRelationship == "" {
				segment.SpeakerRelationship = profile.RelationshipCategory
			}
		}
		return
	}
	segment.SpeakerProfileID = ""
	segment.SpeakerName = ""
	segment.SpeakerRelationship = ""
	segment.SpeakerIdentityStatus = ""
}

func appendTranscriptWarning(current, warning string) string {
	current = strings.TrimSpace(current)
	warning = strings.TrimSpace(warning)
	if current == "" {
		return warning
	}
	if warning == "" || strings.Contains(current, warning) {
		return current
	}
	return current + "; " + warning
}

func purgeExpiredSpeakerFiles(
	ctx context.Context, repository SpeakerProfileRepository, store AudioStore, ownerUserID string,
) error {
	paths, err := repository.PurgeExpiredSpeakerProfiles(ctx, ownerUserID)
	if err != nil {
		return err
	}
	var deleteErrors []error
	for _, path := range paths {
		if err := store.Delete(ctx, path); err != nil {
			deleteErrors = append(deleteErrors, err)
		}
	}
	if err := errors.Join(deleteErrors...); err != nil {
		return fmt.Errorf("delete expired speaker sample files: %w", err)
	}
	return nil
}

var _ SpeakerIdentifier = (*persistentSpeakerIdentifier)(nil)
