package service

import "testing"

func TestEnforceCompatibleSpeakerFieldsKeepsAmbiguousOverlapOutOfLegacyFields(t *testing.T) {
	transcript := Transcript{Segments: []TranscriptSegment{{
		SpeakerRole: "other", SpeakerProfileID: "profile-a", PersonProfileID: "person-a",
		SpeakerProfileIDs: []string{"profile-a", "profile-b"}, Overlap: true,
		SeparationStatus: "ambiguous", AmbiguityReasons: []string{"low_match_margin"},
	}}}
	enforceCompatibleSpeakerFields(&transcript)
	segment := transcript.Segments[0]
	if segment.SpeakerRole != "unknown" || segment.SpeakerProfileID != "" || segment.PersonProfileID != "" {
		t.Fatalf("ambiguous overlap leaked into legacy identity fields: %#v", segment)
	}
	if len(segment.SpeakerProfileIDs) != 2 {
		t.Fatalf("candidate identities were lost: %#v", segment.SpeakerProfileIDs)
	}
}

func TestEnforceCompatibleSpeakerFieldsPopulatesAdditiveCandidates(t *testing.T) {
	transcript := Transcript{Segments: []TranscriptSegment{{SpeakerProfileID: "profile-a"}}}
	enforceCompatibleSpeakerFields(&transcript)
	if len(transcript.Segments[0].SpeakerProfileIDs) != 1 || transcript.Segments[0].SpeakerProfileIDs[0] != "profile-a" {
		t.Fatalf("legacy identity was not copied to additive candidates: %#v", transcript.Segments[0])
	}
}
