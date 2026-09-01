package service

type IdentityLinkPolicy struct {
	FaceThreshold              float64
	VoiceThreshold             float64
	MinimumMargin              float64
	ActiveSpeakerThreshold     float64
	MinimumMouthCoverage       float64
	MinimumTemporalCoverage    float64
	MinimumSeparatedUtterances int
	AutoAccept                 bool
}

type IdentityLinkCandidate struct {
	TemporalOverlap      bool
	PhysicalPresence     bool
	OverlappingConflict  bool
	FaceScore            float64
	FaceRunnerUp         float64
	VoiceScore           float64
	VoiceRunnerUp        float64
	ActiveSpeakerScore   float64
	VisibleMouthCoverage float64
	TemporalCoverage     float64
	SeparatedUtterances  int
}

type IdentityLinkResult struct {
	Decision string   `json:"decision"`
	Reasons  []string `json:"reasons"`
}

// EvaluateIdentityLink keeps recognition, active speaking, and co-occurrence
// separate. Automatic acceptance requires every independent evidence gate.
func EvaluateIdentityLink(candidate IdentityLinkCandidate, policy IdentityLinkPolicy) IdentityLinkResult {
	var reasons []string
	if !candidate.TemporalOverlap {
		reasons = append(reasons, "no_temporal_overlap")
	}
	if !candidate.PhysicalPresence {
		reasons = append(reasons, "face_not_verified_as_physically_present")
	}
	if candidate.OverlappingConflict {
		reasons = append(reasons, "overlapping_speaker_conflict")
	}
	if candidate.FaceScore < policy.FaceThreshold || candidate.FaceScore-candidate.FaceRunnerUp < policy.MinimumMargin {
		reasons = append(reasons, "face_match_or_margin_too_low")
	}
	if candidate.VoiceScore < policy.VoiceThreshold || candidate.VoiceScore-candidate.VoiceRunnerUp < policy.MinimumMargin {
		reasons = append(reasons, "voice_match_or_margin_too_low")
	}
	if candidate.ActiveSpeakerScore < policy.ActiveSpeakerThreshold {
		reasons = append(reasons, "active_speaker_evidence_too_low")
	}
	if candidate.VisibleMouthCoverage < policy.MinimumMouthCoverage {
		reasons = append(reasons, "visible_mouth_coverage_too_low")
	}
	if candidate.TemporalCoverage < policy.MinimumTemporalCoverage {
		reasons = append(reasons, "temporal_coverage_too_low")
	}
	if candidate.SeparatedUtterances < policy.MinimumSeparatedUtterances {
		reasons = append(reasons, "insufficient_separated_utterances")
	}
	if len(reasons) > 0 {
		return IdentityLinkResult{Decision: "ambiguous", Reasons: reasons}
	}
	if policy.AutoAccept {
		return IdentityLinkResult{Decision: "accepted", Reasons: []string{}}
	}
	return IdentityLinkResult{Decision: "suggested", Reasons: []string{"human_confirmation_required"}}
}
