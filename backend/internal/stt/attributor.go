package stt

import (
	"context"
	"strings"

	"github.com/arham/ai-second-brain/internal/service"
)

// ReferenceAttributor converts provider speaker labels into stable semantic
// roles. Unknown diarization labels are never promoted to owner.
type ReferenceAttributor struct{}

func NewReferenceAttributor() *ReferenceAttributor { return &ReferenceAttributor{} }

func (*ReferenceAttributor) Attribute(
	ctx context.Context,
	input service.SpeakerAttributionInput,
) (service.Transcript, error) {
	if err := ctx.Err(); err != nil {
		return service.Transcript{}, err
	}
	ownerLabels := make(map[string]struct{}, len(input.References))
	for _, reference := range input.References {
		if label := strings.TrimSpace(reference.ProviderLabel); label != "" {
			ownerLabels[label] = struct{}{}
		}
	}
	transcript := input.Transcript
	for index := range transcript.Segments {
		segment := &transcript.Segments[index]
		segment.SpeakerRole = "unknown"
		if len(ownerLabels) == 0 {
			continue
		}
		if _, owner := ownerLabels[strings.TrimSpace(segment.Speaker)]; owner {
			segment.SpeakerRole = "owner"
		} else if strings.TrimSpace(segment.Speaker) != "" && segment.Speaker != "speaker" {
			segment.SpeakerRole = "other"
		}
	}
	return transcript, nil
}

var _ service.SpeakerAttributor = (*ReferenceAttributor)(nil)
