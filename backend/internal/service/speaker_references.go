package service

import (
	"context"
	"fmt"
	"io"
)

func loadOwnerSpeakerReferences(
	ctx context.Context,
	repository VoiceRepository,
	store AudioStore,
	ownerUserID string,
) ([]KnownSpeakerReference, error) {
	records, err := repository.ListEnrollmentSamples(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	references := make([]KnownSpeakerReference, 0, len(records))
	for _, record := range records {
		reference, openErr := store.Open(ctx, record.FilePath)
		if openErr != nil {
			return nil, fmt.Errorf("open owner voice sample %s: %w", record.ID, openErr)
		}
		content, readErr := io.ReadAll(io.LimitReader(reference, record.SizeBytes+1))
		closeErr := reference.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read owner voice sample %s: %w", record.ID, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close owner voice sample %s: %w", record.ID, closeErr)
		}
		if int64(len(content)) != record.SizeBytes {
			return nil, fmt.Errorf("owner voice sample %s size changed", record.ID)
		}
		references = append(references, KnownSpeakerReference{
			ID: record.ID, ProviderLabel: record.ProviderLabel,
			MediaType: record.MediaType, Audio: content,
		})
	}
	return references, nil
}

func speakerReferenceIDs(references []KnownSpeakerReference) []string {
	ids := make([]string, 0, len(references))
	for _, reference := range references {
		ids = append(ids, reference.ID)
	}
	return ids
}
