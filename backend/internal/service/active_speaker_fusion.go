package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/rs/zerolog"
)

type ActiveSpeakerFusionVoiceProfile struct {
	ID              string
	PersonProfileID string
	DisplayName     string
	EmbeddingModel  string
}

type ActiveSpeakerFusionJob struct {
	ID                int64
	AnalysisRunID     string
	RecordingID       string
	OwnerUserID       string
	ProcessingVersion int
	Attempts          int
	MaxAttempts       int
	FilePath          string
	FileName          string
	MediaType         string
	Transcript        Transcript
	Tracks            []DenseIdentityTrack
	VoiceProfiles     map[string]ActiveSpeakerFusionVoiceProfile
}

type ActiveSpeakerFusionEvidence struct {
	DeterministicKey       string         `json:"deterministic_key"`
	SegmentID              string         `json:"segment_id"`
	SegmentStartTime       float64        `json:"segment_start_time"`
	SegmentEndTime         float64        `json:"segment_end_time"`
	VoiceSpeakerProfileID  string         `json:"voice_speaker_profile_id"`
	CanonicalPersonID      string         `json:"canonical_person_profile_id"`
	KnownVoiceName         string         `json:"known_voice_name"`
	PersonTrackID          string         `json:"person_track_id,omitempty"`
	VoiceConfidence        float64        `json:"voice_confidence"`
	ActiveSpeakerScore     float64        `json:"active_speaker_score"`
	RunnerUpScore          float64        `json:"runner_up_score"`
	DecisionMargin         float64        `json:"decision_margin"`
	TemporalCoverage       float64        `json:"temporal_coverage"`
	MouthVisibleCoverage   float64        `json:"mouth_visible_coverage"`
	MouthActivity          float64        `json:"mouth_activity"`
	PhysicalPresence       float64        `json:"physical_presence_confidence"`
	SeparationStatus       string         `json:"separation_status"`
	SeparationScore        *float64       `json:"separation_score,omitempty"`
	OverlapGroupID         string         `json:"overlap_group_id,omitempty"`
	OverlappingConflict    bool           `json:"overlapping_conflict"`
	CombinedScore          float64        `json:"combined_score"`
	Decision               string         `json:"decision"`
	Reasons                []string       `json:"reasons"`
	EvidenceObservationIDs []string       `json:"evidence_observation_ids"`
	Raw                    map[string]any `json:"raw"`
}

type ActiveSpeakerFusionResult struct {
	Provider string                        `json:"provider"`
	Model    string                        `json:"model"`
	Version  string                        `json:"version,omitempty"`
	Evidence []ActiveSpeakerFusionEvidence `json:"evidence"`
	Warning  string                        `json:"warning,omitempty"`
}

type ActiveSpeakerFusionPersistenceOptions struct {
	SaveEvidence               bool
	AutoResolveTracks          bool
	AutoBootstrapFaces         bool
	AutoModifyGraph            bool
	MinimumEvidence            int
	MinimumEvidenceSpanSeconds float64
}

type fusionCandidate struct {
	track          DenseIdentityTrack
	temporal       float64
	mouthVisible   float64
	mouthActivity  float64
	observationIDs []string
}

type activeSpeakerFusionService struct {
	repository ActiveSpeakerFusionRepository
	detector   ActiveSpeakerDetector
	store      VideoStore
	config     config.ActiveSpeakerConfig
	profile    string
	staleAfter time.Duration
	logger     *zerolog.Logger
}

func newActiveSpeakerFusionService(
	repository ActiveSpeakerFusionRepository,
	detector ActiveSpeakerDetector,
	store VideoStore,
	cfg config.ActiveSpeakerConfig,
	logger *zerolog.Logger,
) (*activeSpeakerFusionService, error) {
	if repository == nil || store == nil || (cfg.FusionEnabled && detector == nil) {
		return nil, fmt.Errorf("active-speaker fusion dependencies are required")
	}
	model := "disabled"
	if detector != nil {
		model = detector.Model()
	}
	encoded, err := json.Marshal(map[string]any{
		"stage": "active_speaker_fusion", "model": model, "enabled": cfg.FusionEnabled,
		"voice_threshold": cfg.VoiceThreshold, "active_speaker_threshold": cfg.ScoreThreshold,
		"minimum_margin": cfg.MinimumMargin, "minimum_segment_seconds": cfg.MinimumSegmentDuration,
		"minimum_mouth_coverage":       cfg.MinimumMouthCoverage,
		"minimum_temporal_coverage":    cfg.MinimumTemporalCoverage,
		"minimum_independent_segments": cfg.MinimumSeparatedUtterances,
	})
	if err != nil {
		return nil, fmt.Errorf("encode active-speaker fusion configuration: %w", err)
	}
	return &activeSpeakerFusionService{
		repository: repository, detector: detector, store: store, config: cfg,
		profile: string(encoded), staleAfter: 2 * cfg.Timeout, logger: logger,
	}, nil
}

func (s *activeSpeakerFusionService) ProcessNextActiveSpeakerFusion(ctx context.Context) (bool, error) {
	job, found, err := s.repository.ClaimActiveSpeakerFusion(ctx, s.profile, s.staleAfter)
	if err != nil || !found {
		return found, err
	}
	result, processErr := s.process(ctx, job)
	if processErr == nil {
		processErr = s.repository.CompleteActiveSpeakerFusion(ctx, job, result, ActiveSpeakerFusionPersistenceOptions{
			SaveEvidence: s.config.SaveEvidence, AutoResolveTracks: s.config.AutoResolveTracks,
			AutoBootstrapFaces:         s.config.AutoBootstrapFaces,
			AutoModifyGraph:            s.config.AutoModifyGraph,
			MinimumEvidence:            s.config.MinimumSeparatedUtterances,
			MinimumEvidenceSpanSeconds: s.config.MinimumSegmentDuration * float64(s.config.MinimumSeparatedUtterances-1),
		})
	}
	if processErr == nil {
		if s.logger != nil {
			s.logger.Info().Str("recording_id", job.RecordingID).Int("evidence", len(result.Evidence)).Msg("active-speaker fusion completed")
		}
		return true, nil
	}
	dead := job.Attempts >= job.MaxAttempts
	retryCtx := ctx
	var cancel context.CancelFunc
	if ctx.Err() != nil {
		retryCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	if retryErr := s.repository.RetryActiveSpeakerFusion(
		retryCtx, job, processErr.Error(), time.Now().UTC().Add(retryDelayForError(processErr, job.Attempts)), dead,
	); retryErr != nil {
		return true, fmt.Errorf("%v; persist active-speaker retry: %w", processErr, retryErr)
	}
	return true, processErr
}

func (s *activeSpeakerFusionService) process(ctx context.Context, job ActiveSpeakerFusionJob) (ActiveSpeakerFusionResult, error) {
	if !s.config.FusionEnabled || s.detector == nil {
		return ActiveSpeakerFusionResult{Provider: "disabled", Model: "disabled", Warning: "active-speaker fusion disabled by rollout flag"}, nil
	}
	metadata, err := s.detector.Validate(ctx)
	if err != nil {
		return ActiveSpeakerFusionResult{}, fmt.Errorf("validate active-speaker model: %w", err)
	}
	if metadata.Model != s.detector.Model() {
		return ActiveSpeakerFusionResult{}, fmt.Errorf("active-speaker model checksum mismatch: got %q want %q", metadata.Model, s.detector.Model())
	}
	input, candidates, immediate := prepareActiveSpeakerFusion(job, s.config)
	result := ActiveSpeakerFusionResult{
		Provider: metadata.Provider, Model: metadata.Model, Version: metadata.Version, Evidence: immediate,
	}
	if len(input.Segments) == 0 || len(input.PersonTracks) == 0 {
		return result, nil
	}
	path, cleanup, err := materializeObject(ctx, s.store, job.FilePath, job.FileName)
	if err != nil {
		return ActiveSpeakerFusionResult{}, err
	}
	defer cleanup()
	input.VideoPath, input.FileName, input.MediaType = path, job.FileName, job.MediaType
	analysis, err := s.detector.DetectActiveSpeakers(ctx, input)
	if err != nil {
		return ActiveSpeakerFusionResult{}, fmt.Errorf("detect active speakers: %w", err)
	}
	if analysis.Model != metadata.Model || analysis.Provider == "" {
		return ActiveSpeakerFusionResult{}, fmt.Errorf("active-speaker result provenance mismatch")
	}
	result.Provider, result.Model, result.Warning = analysis.Provider, analysis.Model, analysis.Warning
	result.Evidence = append(result.Evidence, evaluateActiveSpeakerFusion(job, candidates, analysis, s.config)...)
	applyFusionBatchConflicts(result.Evidence)
	return result, nil
}

func prepareActiveSpeakerFusion(
	job ActiveSpeakerFusionJob,
	cfg config.ActiveSpeakerConfig,
) (ActiveSpeakerInput, map[string][]fusionCandidate, []ActiveSpeakerFusionEvidence) {
	input := ActiveSpeakerInput{RecordingID: job.RecordingID, FaceObservations: map[string][]DenseFaceObservation{}}
	candidates := make(map[string][]fusionCandidate)
	var immediate []ActiveSpeakerFusionEvidence
	for _, segment := range job.Transcript.Segments {
		if strings.TrimSpace(segment.ID) == "" || segment.EndTime <= segment.StartTime {
			continue
		}
		profile, known := job.VoiceProfiles[segment.SpeakerProfileID]
		voiceScore := pointerValue(segment.IdentityConfidence)
		var reasons []string
		if !known {
			continue
		}
		if profile.PersonProfileID == "" {
			reasons = append(reasons, "voice_identity_unknown")
		}
		if segment.IdentityConfidence == nil || voiceScore < cfg.VoiceThreshold {
			reasons = append(reasons, "voice_match_too_low")
		}
		if segment.EndTime-segment.StartTime < cfg.MinimumSegmentDuration {
			reasons = append(reasons, "segment_too_short")
		}
		if segment.Overlap && (segment.SeparationStatus != "accepted" || segment.SeparationConfidence == nil || *segment.SeparationConfidence < cfg.VoiceThreshold) {
			reasons = append(reasons, "overlapping_speech_not_separated")
		}
		if len(reasons) > 0 {
			immediate = append(immediate, fusionEvidence(job, segment, profile, "", voiceScore, "rejected", reasons, cfg))
			continue
		}
		for _, track := range job.Tracks {
			candidate, overlaps := measureFusionCandidate(track, segment)
			if overlaps {
				candidates[segment.ID] = append(candidates[segment.ID], candidate)
			}
		}
		if len(candidates[segment.ID]) == 0 {
			immediate = append(immediate, fusionEvidence(job, segment, profile, "", voiceScore, "ambiguous", []string{"off_screen"}, cfg))
			continue
		}
		input.Segments = append(input.Segments, segment)
		for _, candidate := range candidates[segment.ID] {
			if _, exists := input.FaceObservations[candidate.track.ID]; !exists {
				input.PersonTracks = append(input.PersonTracks, TemporalPersonTrack{
					ID: candidate.track.ID, StartTime: candidate.track.StartTime, EndTime: candidate.track.EndTime,
					TrackingConfidence: candidate.track.TrackingConfidence,
					EvidenceFrameIDs:   candidate.observationIDs, PhysicalPresence: true,
				})
				input.FaceObservations[candidate.track.ID] = candidate.track.Observations
			}
		}
	}
	return input, candidates, immediate
}

func evaluateActiveSpeakerFusion(
	job ActiveSpeakerFusionJob,
	candidates map[string][]fusionCandidate,
	analysis ActiveSpeakerAnalysis,
	cfg config.ActiveSpeakerConfig,
) []ActiveSpeakerFusionEvidence {
	segments := make(map[string]TranscriptSegment, len(job.Transcript.Segments))
	for _, segment := range job.Transcript.Segments {
		segments[segment.ID] = segment
	}
	scores := make(map[string]map[string]ActiveSpeakerEvidence)
	for _, item := range analysis.Evidence {
		for _, segmentID := range item.SegmentIDs {
			if scores[segmentID] == nil {
				scores[segmentID] = make(map[string]ActiveSpeakerEvidence)
			}
			scores[segmentID][item.PersonTrackID] = item
		}
	}
	var result []ActiveSpeakerFusionEvidence
	for segmentID, options := range candidates {
		segment := segments[segmentID]
		profile := job.VoiceProfiles[segment.SpeakerProfileID]
		voiceScore := pointerValue(segment.IdentityConfidence)
		sort.Slice(options, func(i, j int) bool {
			return scores[segmentID][options[i].track.ID].Score > scores[segmentID][options[j].track.ID].Score
		})
		best, runnerUp := 0.0, 0.0
		if len(options) > 0 {
			best = scores[segmentID][options[0].track.ID].Score
		}
		if len(options) > 1 {
			runnerUp = scores[segmentID][options[1].track.ID].Score
		}
		for index, candidate := range options {
			detected, returned := scores[segmentID][candidate.track.ID]
			reasons := []string{}
			if !returned {
				reasons = append(reasons, "active_speaker_result_missing")
			}
			if detected.Score < cfg.ScoreThreshold {
				reasons = append(reasons, "active_speaker_score_too_low")
			}
			if candidate.temporal < cfg.MinimumTemporalCoverage {
				reasons = append(reasons, "temporal_coverage_too_low")
			}
			if candidate.mouthVisible < cfg.MinimumMouthCoverage {
				reasons = append(reasons, "mouth_visible_coverage_too_low")
			}
			if index == 0 && best-runnerUp < cfg.MinimumMargin {
				reasons = append(reasons, "runner_up_margin_too_low")
			}
			if index > 0 {
				reasons = append(reasons, "not_best_candidate")
			}
			decision := "accepted"
			if len(reasons) > 0 {
				decision = "ambiguous"
				if index > 0 {
					decision = "rejected"
				}
			}
			evidence := fusionEvidence(job, segment, profile, candidate.track.ID, voiceScore, decision, reasons, cfg)
			evidence.ActiveSpeakerScore, evidence.RunnerUpScore = detected.Score, runnerUp
			evidence.DecisionMargin = detected.Score - runnerUp
			evidence.TemporalCoverage, evidence.MouthVisibleCoverage = candidate.temporal, candidate.mouthVisible
			evidence.MouthActivity, evidence.PhysicalPresence = candidate.mouthActivity, candidate.track.TrackingConfidence
			evidence.EvidenceObservationIDs = candidate.observationIDs
			evidence.OverlappingConflict = segment.Overlap && segment.SeparationStatus != "accepted"
			evidence.CombinedScore = fusionCombinedScore(evidence)
			evidence.Raw["talknet"] = detected
			evidence.Raw["candidate_rank"] = index + 1
			result = append(result, evidence)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SegmentStartTime == result[j].SegmentStartTime {
			return result[i].PersonTrackID < result[j].PersonTrackID
		}
		return result[i].SegmentStartTime < result[j].SegmentStartTime
	})
	return result
}

func measureFusionCandidate(track DenseIdentityTrack, segment TranscriptSegment) (fusionCandidate, bool) {
	duration := segment.EndTime - segment.StartTime
	overlap := math.Max(0, math.Min(track.EndTime, segment.EndTime)-math.Max(track.StartTime, segment.StartTime))
	if overlap <= 0 || duration <= 0 {
		return fusionCandidate{}, false
	}
	candidate := fusionCandidate{track: track, temporal: math.Min(1, overlap/duration)}
	visible, activity := 0, 0.0
	for _, observation := range track.Observations {
		if observation.Timestamp < segment.StartTime || observation.Timestamp > segment.EndTime {
			continue
		}
		candidate.observationIDs = append(candidate.observationIDs, observation.ObservationID)
		if observation.MouthVisible {
			visible++
			activity += observation.MouthActivity
		}
	}
	if len(candidate.observationIDs) > 0 {
		candidate.mouthVisible = float64(visible) / float64(len(candidate.observationIDs))
	}
	if visible > 0 {
		candidate.mouthActivity = activity / float64(visible)
	}
	return candidate, true
}

func fusionEvidence(
	job ActiveSpeakerFusionJob,
	segment TranscriptSegment,
	profile ActiveSpeakerFusionVoiceProfile,
	trackID string,
	voiceScore float64,
	decision string,
	reasons []string,
	cfg config.ActiveSpeakerConfig,
) ActiveSpeakerFusionEvidence {
	trackKey := trackID
	if trackKey == "" {
		trackKey = "none"
	}
	hash := sha256.Sum256([]byte(strings.Join([]string{
		job.OwnerUserID, job.RecordingID, fmt.Sprint(job.ProcessingVersion), segment.ID, trackKey,
	}, "\x00")))
	return ActiveSpeakerFusionEvidence{
		DeterministicKey: hex.EncodeToString(hash[:]), SegmentID: segment.ID,
		SegmentStartTime: segment.StartTime, SegmentEndTime: segment.EndTime,
		VoiceSpeakerProfileID: segment.SpeakerProfileID, CanonicalPersonID: profile.PersonProfileID,
		KnownVoiceName: profile.DisplayName, PersonTrackID: trackID, VoiceConfidence: voiceScore,
		SeparationStatus: segment.SeparationStatus, SeparationScore: segment.SeparationConfidence,
		OverlapGroupID: segment.OverlapGroupID, OverlappingConflict: segment.Overlap,
		Decision: decision, Reasons: append([]string(nil), reasons...), Raw: map[string]any{
			"thresholds": map[string]any{
				"voice": cfg.VoiceThreshold, "active_speaker": cfg.ScoreThreshold,
				"runner_up_margin": cfg.MinimumMargin, "temporal_coverage": cfg.MinimumTemporalCoverage,
				"mouth_visible_coverage": cfg.MinimumMouthCoverage,
			},
		},
	}
}

func fusionCombinedScore(e ActiveSpeakerFusionEvidence) float64 {
	score := .25*e.VoiceConfidence + .35*e.ActiveSpeakerScore + .15*e.TemporalCoverage +
		.10*e.MouthVisibleCoverage + .10*e.MouthActivity + .05*e.PhysicalPresence
	return math.Max(0, math.Min(1, score))
}

func applyFusionBatchConflicts(evidence []ActiveSpeakerFusionEvidence) {
	identities := make(map[string]map[string]bool)
	for _, item := range evidence {
		if item.Decision != "accepted" || item.PersonTrackID == "" || item.CanonicalPersonID == "" {
			continue
		}
		if identities[item.PersonTrackID] == nil {
			identities[item.PersonTrackID] = make(map[string]bool)
		}
		identities[item.PersonTrackID][item.CanonicalPersonID] = true
	}
	for index := range evidence {
		item := &evidence[index]
		if item.Decision == "accepted" && len(identities[item.PersonTrackID]) > 1 {
			item.Decision = "ambiguous"
			item.Reasons = append(item.Reasons, "different_known_voices_same_track")
		}
	}
}

func pointerValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

var _ ActiveSpeakerFusionService = (*activeSpeakerFusionService)(nil)
