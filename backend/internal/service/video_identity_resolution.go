package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

func (s *videoService) autoResolveVideoIdentities(ctx context.Context, job *VideoJob) error {
	s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Msg("capture function called")
	if s.activeSpeaker == nil || s.personRepository == nil {
		s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Msg("identity resolution skipped: dependencies unavailable")
		return nil
	}
	tracks := identityPersonTracks(job.VisualAnalysis)
	segments := identifiableSegments(job.Transcript.Segments)
	if len(tracks) == 0 || len(segments) == 0 {
		s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Int("person_tracks", len(tracks)).Int("speaker_segments", len(segments)).Msg("identity resolution skipped: insufficient evidence")
		return nil
	}
	s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Int("person_tracks", len(tracks)).Int("speaker_segments", len(segments)).Msg("active-speaker analysis started")
	path, cleanup, err := materializeObject(ctx, s.store, job.FilePath, job.FileName)
	if err != nil {
		s.warn().Err(err).Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Msg("identity media materialization failed")
		return err
	}
	defer cleanup()
	analysis, err := s.activeSpeaker.DetectActiveSpeakers(ctx, ActiveSpeakerInput{
		RecordingID: job.RecordingID, VideoPath: path, FileName: job.FileName,
		MediaType: job.MediaType, PersonTracks: tracks, Segments: segments,
	})
	if err != nil {
		s.warn().Err(err).Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Msg("active-speaker analysis failed")
		return fmt.Errorf("detect active speakers: %w", err)
	}
	s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Int("evidence_items", len(analysis.Evidence)).Msg("active-speaker analysis completed")
	trackByID := make(map[string]TemporalPersonTrack, len(tracks))
	for _, track := range tracks {
		trackByID[track.ID] = track
	}
	segmentByID := make(map[string]TranscriptSegment, len(segments))
	for _, segment := range segments {
		segmentByID[segment.ID] = segment
	}
	policy := IdentityLinkPolicy{
		ActiveSpeakerThreshold:     s.activeSpeakerConfig.ScoreThreshold,
		MinimumMouthCoverage:       s.activeSpeakerConfig.MinimumMouthCoverage,
		MinimumTemporalCoverage:    s.activeSpeakerConfig.MinimumTemporalCoverage,
		MinimumSeparatedUtterances: s.activeSpeakerConfig.MinimumSeparatedUtterances,
		AutoAccept:                 s.activeSpeakerConfig.AutoLink,
	}
	for _, evidence := range analysis.Evidence {
		track, ok := trackByID[evidence.PersonTrackID]
		if !ok {
			s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Str("person_track_id", evidence.PersonTrackID).Msg("identity evidence skipped: track not found")
			continue
		}
		speakerID, linkedSegments, ok := oneSpeakerProfile(evidence.SegmentIDs, segmentByID)
		if !ok {
			s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Str("person_track_id", evidence.PersonTrackID).Msg("identity evidence skipped: segments do not resolve to one speaker")
			continue
		}
		coverage := trackSegmentCoverage(track, linkedSegments)
		decision := EvaluateIdentityLink(IdentityLinkCandidate{
			TemporalOverlap: coverage > 0, PhysicalPresence: track.PhysicalPresence,
			OverlappingConflict: evidence.OverlappingConflict,
			FaceScore:           1, VoiceScore: 1, ActiveSpeakerScore: evidence.Score,
			VisibleMouthCoverage: evidence.VisibleMouthCoverage, TemporalCoverage: coverage,
			SeparatedUtterances: separatedUtteranceCount(linkedSegments),
		}, policy)
		s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Str("person_track_id", evidence.PersonTrackID).Str("voice_profile_id", speakerID).Str("decision", decision.Decision).Strs("decision_reasons", decision.Reasons).Float64("active_speaker_score", evidence.Score).Float64("temporal_coverage", coverage).Msg("identity link evaluated")
		mergeEvidenceCount := s.activeSpeakerConfig.MergeEvidenceCount
		if mergeEvidenceCount < 2 {
			mergeEvidenceCount = 3
		}
		resolution, err := s.personRepository.ResolveAutomaticIdentity(ctx, AutomaticIdentityEvidenceInput{
			OwnerUserID: job.OwnerUserID, RecordingID: job.RecordingID,
			PersonTrackID: evidence.PersonTrackID, VoiceSpeakerProfileID: speakerID,
			SegmentIDs: evidence.SegmentIDs, ActiveSpeakerScore: evidence.Score,
			VisibleMouthCoverage: evidence.VisibleMouthCoverage, TemporalCoverage: coverage,
			OverlappingConflict: evidence.OverlappingConflict, Decision: decision.Decision,
			FaceProvider: "face-gallery", FaceModel: s.faceConfig.Model,
			ActiveSpeakerProvider: analysis.Provider, ActiveSpeakerModel: analysis.Model,
			ProcessingVersion: job.ProcessingVersion, AutoMerge: s.activeSpeakerConfig.AutoMerge,
			MergeEvidenceRequirement: mergeEvidenceCount,
		})
		if err != nil {
			s.warn().Err(err).Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Str("person_track_id", evidence.PersonTrackID).Str("voice_profile_id", speakerID).Msg("identity resolution persistence failed")
			return err
		}
		s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Str("person_track_id", evidence.PersonTrackID).Str("person_profile_id", resolution.PersonProfileID).Str("resolution", resolution.Decision).Str("merged_from_profile_id", resolution.MergedFromProfileID).Msg("identity evidence resolved")
		if resolution.Decision == "accepted" {
			applyAutomaticIdentityResolution(job, resolution)
		}
	}
	s.debug().Str("capture_function", "autoResolveVideoIdentities").Str("recording_id", job.RecordingID).Msg("capture function completed")
	return nil
}

func identityPersonTracks(analysis VisualAnalysis) []TemporalPersonTrack {
	tracks := make(map[string]TemporalPersonTrack)
	for _, observation := range analysis.Observations {
		for _, person := range observation.People {
			if person.PersonTrackID == "" || person.PersonProfileID == "" || !person.PhysicalPresence {
				continue
			}
			track, exists := tracks[person.PersonTrackID]
			confidence := 1.0
			if person.Confidence != nil {
				confidence = *person.Confidence
			}
			if !exists {
				track = TemporalPersonTrack{
					ID: person.PersonTrackID, StartTime: observation.StartTime, EndTime: observation.EndTime,
					TrackingConfidence: confidence, PhysicalPresence: true,
				}
			} else {
				track.StartTime = math.Min(track.StartTime, observation.StartTime)
				track.EndTime = math.Max(track.EndTime, observation.EndTime)
				track.TrackingConfidence = math.Min(track.TrackingConfidence, confidence)
			}
			if observation.FrameID != "" {
				appendUnique(&track.EvidenceFrameIDs, observation.FrameID)
			}
			tracks[person.PersonTrackID] = track
		}
	}
	result := make([]TemporalPersonTrack, 0, len(tracks))
	for _, track := range tracks {
		result = append(result, track)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartTime < result[j].StartTime })
	return result
}

func identifiableSegments(segments []TranscriptSegment) []TranscriptSegment {
	result := make([]TranscriptSegment, 0, len(segments))
	for _, segment := range segments {
		if strings.TrimSpace(segment.ID) != "" && strings.TrimSpace(segment.SpeakerProfileID) != "" {
			result = append(result, segment)
		}
	}
	return result
}

func oneSpeakerProfile(
	segmentIDs []string, segments map[string]TranscriptSegment,
) (string, []TranscriptSegment, bool) {
	var speakerID string
	result := make([]TranscriptSegment, 0, len(segmentIDs))
	seen := make(map[string]bool, len(segmentIDs))
	for _, segmentID := range segmentIDs {
		if seen[segmentID] {
			continue
		}
		segment, ok := segments[segmentID]
		if !ok || segment.SpeakerProfileID == "" ||
			(speakerID != "" && speakerID != segment.SpeakerProfileID) {
			return "", nil, false
		}
		seen[segmentID] = true
		speakerID = segment.SpeakerProfileID
		result = append(result, segment)
	}
	return speakerID, result, speakerID != ""
}

func trackSegmentCoverage(track TemporalPersonTrack, segments []TranscriptSegment) float64 {
	var overlap, total float64
	for _, segment := range segments {
		duration := math.Max(0, segment.EndTime-segment.StartTime)
		total += duration
		overlap += math.Max(0, math.Min(track.EndTime, segment.EndTime)-math.Max(track.StartTime, segment.StartTime))
	}
	if total == 0 {
		return 0
	}
	return math.Min(1, overlap/total)
}

func separatedUtteranceCount(segments []TranscriptSegment) int {
	if len(segments) == 0 {
		return 0
	}
	ordered := append([]TranscriptSegment(nil), segments...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].StartTime < ordered[j].StartTime })
	count, lastEnd := 1, ordered[0].EndTime
	for _, segment := range ordered[1:] {
		if segment.StartTime-lastEnd >= .25 {
			count++
		}
		lastEnd = math.Max(lastEnd, segment.EndTime)
	}
	return count
}

func applyAutomaticIdentityResolution(job *VideoJob, resolution AutomaticIdentityResolution) {
	for observationIndex := range job.VisualAnalysis.Observations {
		observation := &job.VisualAnalysis.Observations[observationIndex]
		for personIndex := range observation.People {
			person := &observation.People[personIndex]
			if person.PersonTrackID == resolution.PersonTrackID ||
				(resolution.MergedFromProfileID != "" && person.PersonProfileID == resolution.MergedFromProfileID) {
				person.PersonProfileID = resolution.PersonProfileID
				person.PersonIdentityStatus = "confirmed"
				if resolution.DisplayName != "" {
					person.PersonName = resolution.DisplayName
				}
			}
		}
	}
	for segmentIndex := range job.Transcript.Segments {
		segment := &job.Transcript.Segments[segmentIndex]
		if segment.SpeakerProfileID == resolution.VoiceSpeakerProfileID ||
			(resolution.MergedFromProfileID != "" && segment.PersonProfileID == resolution.MergedFromProfileID) {
			segment.PersonProfileID = resolution.PersonProfileID
			segment.SpeakerIdentityStatus = "confirmed"
			if resolution.DisplayName != "" {
				segment.SpeakerName = resolution.DisplayName
			}
		}
	}
}
