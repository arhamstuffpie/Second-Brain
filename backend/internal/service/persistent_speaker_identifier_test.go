package service

import "testing"

func TestSpeakerGroupsAggregateOnlyNonOwnerSpeech(t *testing.T) {
	groups := speakerGroups([]TranscriptSegment{
		{ID: "a", StartTime: 0, EndTime: 1.25, Speaker: "A", SpeakerRole: "other"},
		{ID: "owner", StartTime: 1.25, EndTime: 3, Speaker: "owner", SpeakerRole: "owner"},
		{ID: "b", StartTime: 4, EndTime: 5.5, Speaker: "A", SpeakerRole: "unknown"},
		{ID: "invalid", StartTime: 8, EndTime: 7, Speaker: "B", SpeakerRole: "other"},
	})
	if len(groups) != 1 {
		t.Fatalf("speakerGroups() length = %d, want 1", len(groups))
	}
	if groups[0].providerSpeaker != "A" || groups[0].duration != 2.75 || len(groups[0].ranges) != 2 {
		t.Fatalf("speakerGroups() = %#v", groups[0])
	}
}

func TestApplySpeakerProfilePreservesOwnerAndLabelsOtherSpeaker(t *testing.T) {
	transcript := Transcript{Segments: []TranscriptSegment{
		{Speaker: "owner", SpeakerRole: "owner"},
		{Speaker: "A", SpeakerRole: "unknown"},
	}}
	applySpeakerProfile(&transcript, "A", SpeakerProfile{
		ID: "profile-1", Status: "confirmed", DisplayName: "Sarah",
		RelationshipCategory: "family", RelationshipLabel: "sister",
	})
	if transcript.Segments[0].SpeakerProfileID != "" || transcript.Segments[0].SpeakerRole != "owner" {
		t.Fatalf("owner segment changed: %#v", transcript.Segments[0])
	}
	other := transcript.Segments[1]
	if other.SpeakerRole != "other" || other.SpeakerProfileID != "profile-1" ||
		other.SpeakerName != "Sarah" || other.SpeakerRelationship != "sister" {
		t.Fatalf("other segment = %#v", other)
	}
}
