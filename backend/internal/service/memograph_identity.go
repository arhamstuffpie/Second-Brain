package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func memographIdempotencyKey(memoryID, episodeID, source string) string {
	digest := sha256.Sum256([]byte(strings.Join(
		[]string{"second-brain-v1", memoryID, episodeID, source}, "\x00",
	)))
	return "second-brain-" + hex.EncodeToString(digest[:])
}

func canonicalOwnerID(ownerUserID string) string {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return ""
	}
	return "account-owner:" + ownerUserID
}

// defaultMemoryGroupID keeps graph identity stable across capture sessions.
// Callers may still provide an explicit group to intentionally isolate graphs.
func defaultMemoryGroupID(ownerUserID string) string {
	return canonicalOwnerID(ownerUserID)
}

func addOwnerIdentityMeta(meta map[string]any, ownerUserID string) {
	if ownerID := canonicalOwnerID(ownerUserID); ownerID != "" {
		meta["owner_user_id"] = strings.TrimSpace(ownerUserID)
		meta["owner_entity_id"] = ownerID
		meta["owner_entity_name"] = "Owner"
	}
}

type groundedConversationSegment struct {
	StartTime  float64
	EndTime    float64
	Speaker    string
	Role       string
	Text       string
	Confidence *float64
}

func structuredVoiceConversation(job VoiceJob) *StructuredGraph {
	segments := make([]groundedConversationSegment, 0, len(job.EpisodeSegments))
	for _, segment := range job.EpisodeSegments {
		segments = append(segments, groundedConversationSegment{
			StartTime: segment.StartTime, EndTime: segment.EndTime,
			Speaker: segment.Speaker, Role: segment.SpeakerRole,
			Text: segment.Text, Confidence: segment.Confidence,
		})
	}
	return buildStructuredConversation(
		job.EpisodeID, "audio", job.SessionID, job.OwnerUserID,
		job.Location, job.Description, job.EpisodeStart, job.EpisodeEnd, segments,
	)
}

func structuredVideoConversation(job VideoJob) *StructuredGraph {
	segments := make([]groundedConversationSegment, 0, len(job.Transcript.Segments))
	for _, segment := range job.Transcript.Segments {
		start := job.StartOffset + segment.StartTime
		end := job.StartOffset + segment.EndTime
		if start < job.EpisodeStart || start >= job.EpisodeEnd {
			continue
		}
		segments = append(segments, groundedConversationSegment{
			StartTime: start, EndTime: end, Speaker: segment.Speaker,
			Role: segment.SpeakerRole, Text: segment.Text,
			Confidence: segment.Confidence,
		})
	}
	return buildStructuredConversation(
		job.EpisodeID, "video-speech", job.SessionID, job.OwnerUserID,
		job.Location, job.SpeechDescription, job.EpisodeStart, job.EpisodeEnd, segments,
	)
}

func buildStructuredConversation(
	episodeID, source, sessionID, ownerUserID, location, summary string,
	startTime, endTime float64,
	segments []groundedConversationSegment,
) *StructuredGraph {
	ownerID := canonicalOwnerID(ownerUserID)
	if ownerID == "" || strings.TrimSpace(episodeID) == "" || len(segments) == 0 {
		return nil
	}
	stableEpisodeID := "second-brain:" + source + ":" + strings.TrimSpace(episodeID)
	graph := &StructuredGraph{
		EpisodeID: stableEpisodeID, SceneID: strings.TrimSpace(sessionID),
		StartTime: startTime, EndTime: endTime, Summary: strings.TrimSpace(summary),
		Location: strings.TrimSpace(location),
		Entities: []StructuredEntity{{
			CanonicalID: ownerID,
			Name:        "Owner",
			Type:        "Person",
		}},
		Relations:  make([]StructuredRelation, 0, len(segments)*3),
		Utterances: make([]StructuredUtterance, 0, len(segments)),
	}
	seenEntities := map[string]struct{}{ownerID: {}}
	previousUtteranceID := ""
	for index, segment := range segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" {
			continue
		}
		speakerID, speakerName := structuredSpeakerIdentity(
			ownerID, sessionID, segment.Role, segment.Speaker,
		)
		if _, exists := seenEntities[speakerID]; !exists {
			seenEntities[speakerID] = struct{}{}
			graph.Entities = append(graph.Entities, StructuredEntity{
				CanonicalID: speakerID, Name: speakerName, Type: "Person",
				Confidence: segment.Confidence,
			})
		}
		utteranceID := structuredUtteranceID(
			stableEpisodeID, index, segment.StartTime, segment.EndTime, text,
		)
		graph.Entities = append(graph.Entities, StructuredEntity{
			CanonicalID: utteranceID,
			Name: conversationUtteranceName(
				text, utteranceID, segment.StartTime,
			),
			Type: "ConversationUtterance", Confidence: segment.Confidence,
		})
		graph.Utterances = append(graph.Utterances, StructuredUtterance{
			SpeakerID: speakerID, Speaker: speakerName, Text: text,
			StartTime: segment.StartTime, EndTime: segment.EndTime,
			Confidence: segment.Confidence,
		})
		predicate, verb := "SAID", "said"
		if isQuestionUtterance(text) {
			predicate, verb = "ASKED", "asked"
		}
		graph.Relations = append(graph.Relations, StructuredRelation{
			Source: speakerID, Predicate: predicate, Target: utteranceID,
			Fact: fmt.Sprintf(
				"[%.2fs-%.2fs] %s %s: %s",
				segment.StartTime, segment.EndTime, speakerName, verb, text,
			),
			Confidence: segment.Confidence,
		})
		if speakerID != ownerID {
			graph.Relations = append(graph.Relations, StructuredRelation{
				Source: ownerID, Predicate: "HAS_CONTEXT", Target: utteranceID,
				Fact: fmt.Sprintf(
					"[%.2fs-%.2fs] Non-owner context for Owner: %s %s: %s",
					segment.StartTime, segment.EndTime, speakerName, verb, text,
				),
				Confidence: segment.Confidence,
			})
		}
		if previousUtteranceID != "" {
			graph.Relations = append(graph.Relations, StructuredRelation{
				Source: previousUtteranceID, Predicate: "FOLLOWED_BY",
				Target: utteranceID,
				Fact:   "The next timestamped utterance in this conversation.",
			})
		}
		previousUtteranceID = utteranceID
	}
	if len(graph.Utterances) == 0 {
		return nil
	}
	return graph
}

func structuredUtteranceID(
	episodeID string, index int, startTime, endTime float64, text string,
) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%d\x00%.6f\x00%.6f\x00%s",
		episodeID, index, startTime, endTime, text,
	)))
	return "conversation-utterance:" + hex.EncodeToString(digest[:12])
}

func conversationUtteranceName(text, utteranceID string, startTime float64) string {
	const maximumRunes = 120
	shortID := strings.TrimPrefix(utteranceID, "conversation-utterance:")
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	suffix := fmt.Sprintf(" [%s @ %.2fs]", shortID, startTime)
	available := maximumRunes - len([]rune(suffix))
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= available {
		return string(runes) + suffix
	}
	return string(runes[:available-3]) + "..." + suffix
}

func isQuestionUtterance(text string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(text), "\"'”’)]}")
	runes := []rune(trimmed)
	if len(runes) == 0 {
		return false
	}
	switch runes[len(runes)-1] {
	case '?', '？', '؟':
		return true
	default:
		return false
	}
}

func structuredSpeakerIdentity(
	ownerID, sessionID, role, providerSpeaker string,
) (string, string) {
	if role == "owner" {
		return ownerID, "Owner"
	}
	providerSpeaker = strings.TrimSpace(providerSpeaker)
	if providerSpeaker == "" || strings.EqualFold(providerSpeaker, "speaker") {
		providerSpeaker = "unidentified"
	}
	normalizedRole := "unknown"
	displayRole := "Unknown speaker"
	if role == "other" {
		normalizedRole = "other"
		displayRole = "Other speaker"
	}
	digest := sha256.Sum256([]byte(strings.Join(
		[]string{strings.TrimSpace(sessionID), normalizedRole, strings.ToLower(providerSpeaker)},
		"\x00",
	)))
	identity := "conversation-speaker:" + hex.EncodeToString(digest[:12])
	name := displayRole + " " + providerSpeaker
	if sessionID != "" {
		name += " in session " + strings.TrimSpace(sessionID)
	}
	return identity, name
}
