package service

import "testing"

func strictIdentityPolicy() IdentityLinkPolicy {
	return IdentityLinkPolicy{
		FaceThreshold: .7, VoiceThreshold: .7, MinimumMargin: .1,
		ActiveSpeakerThreshold: .8, MinimumMouthCoverage: .75,
		MinimumTemporalCoverage: .75, MinimumSeparatedUtterances: 2,
	}
}

func strongIdentityCandidate() IdentityLinkCandidate {
	return IdentityLinkCandidate{
		TemporalOverlap: true, PhysicalPresence: true, FaceScore: .9, FaceRunnerUp: .5,
		VoiceScore: .9, VoiceRunnerUp: .5, ActiveSpeakerScore: .95,
		VisibleMouthCoverage: .9, TemporalCoverage: .9, SeparatedUtterances: 2,
	}
}

func TestEvaluateIdentityLinkRejectsMereCooccurrence(t *testing.T) {
	candidate := strongIdentityCandidate()
	candidate.ActiveSpeakerScore = .1
	result := EvaluateIdentityLink(candidate, strictIdentityPolicy())
	if result.Decision != "ambiguous" {
		t.Fatalf("decision = %q, want ambiguous", result.Decision)
	}
}

func TestEvaluateIdentityLinkRejectsOverlapAndScreenFaces(t *testing.T) {
	candidate := strongIdentityCandidate()
	candidate.OverlappingConflict = true
	if result := EvaluateIdentityLink(candidate, strictIdentityPolicy()); result.Decision != "ambiguous" {
		t.Fatalf("overlap decision = %q", result.Decision)
	}
	candidate = strongIdentityCandidate()
	candidate.PhysicalPresence = false
	if result := EvaluateIdentityLink(candidate, strictIdentityPolicy()); result.Decision != "ambiguous" {
		t.Fatalf("screen face decision = %q", result.Decision)
	}
}

func TestEvaluateIdentityLinkDefaultsToReviewSuggestion(t *testing.T) {
	result := EvaluateIdentityLink(strongIdentityCandidate(), strictIdentityPolicy())
	if result.Decision != "suggested" {
		t.Fatalf("decision = %q, want suggested", result.Decision)
	}
}
