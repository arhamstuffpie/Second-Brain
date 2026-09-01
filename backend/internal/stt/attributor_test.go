package stt

import (
	"context"
	"testing"

	"github.com/arham/ai-second-brain/internal/service"
)

func TestReferenceAttributorNeverPromotesUnknownLabel(t *testing.T) {
	result, err := NewReferenceAttributor().Attribute(context.Background(), service.SpeakerAttributionInput{
		Transcript: service.Transcript{Segments: []service.TranscriptSegment{
			{Speaker: "owner_ref", Text: "I prefer tea."},
			{Speaker: "A", Text: "I prefer coffee."},
			{Speaker: "speaker", Text: "Unlabeled."},
		}},
		References: []service.KnownSpeakerReference{{ProviderLabel: "owner_ref"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"owner", "other", "unknown"}
	for index, role := range want {
		if result.Segments[index].SpeakerRole != role {
			t.Fatalf("segment %d role = %q, want %q", index, result.Segments[index].SpeakerRole, role)
		}
	}
}

func TestReferenceAttributorWithoutEnrollmentMarksEveryoneUnknown(t *testing.T) {
	result, err := NewReferenceAttributor().Attribute(context.Background(), service.SpeakerAttributionInput{
		Transcript: service.Transcript{Segments: []service.TranscriptSegment{{Speaker: "A", Text: "Hello"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Segments[0].SpeakerRole != "unknown" {
		t.Fatalf("role = %q, want unknown", result.Segments[0].SpeakerRole)
	}
}

func TestReferenceAttributorMatchesProviderLabelsCaseInsensitively(t *testing.T) {
	result, err := NewReferenceAttributor().Attribute(context.Background(), service.SpeakerAttributionInput{
		Transcript: service.Transcript{Segments: []service.TranscriptSegment{{
			Speaker: "  OWNER_REF  ", Text: "Is the meeting still at three?",
		}}},
		References: []service.KnownSpeakerReference{{ProviderLabel: "owner_ref"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Segments[0].SpeakerRole != "owner" {
		t.Fatalf("role = %q, want owner", result.Segments[0].SpeakerRole)
	}
}
