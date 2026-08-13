package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func memographIdempotencyKey(memoryID, episodeID, source string, graphRevision ...int64) string {
	identity := []string{"second-brain-v1", memoryID, episodeID, source}
	if len(graphRevision) > 0 && graphRevision[0] > 0 {
		identity = append(identity, fmt.Sprintf("graph-revision:%d", graphRevision[0]))
	}
	digest := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
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
	StartTime        float64
	EndTime          float64
	Speaker          string
	Role             string
	SpeakerProfileID string
	SpeakerName      string
	Text             string
	Confidence       *float64
}

func structuredVoiceConversation(job VoiceJob) *StructuredGraph {
	segments := make([]groundedConversationSegment, 0, len(job.EpisodeSegments))
	for _, segment := range job.EpisodeSegments {
		segments = append(segments, groundedConversationSegment{
			StartTime: segment.StartTime, EndTime: segment.EndTime,
			Speaker: segment.Speaker, Role: segment.SpeakerRole,
			SpeakerProfileID: segment.SpeakerProfileID, SpeakerName: segment.SpeakerName,
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
			SpeakerProfileID: segment.SpeakerProfileID, SpeakerName: segment.SpeakerName,
			Confidence: segment.Confidence,
		})
	}
	return buildStructuredConversation(
		job.EpisodeID, "video-speech", job.SessionID, job.OwnerUserID,
		job.Location, job.SpeechDescription, job.EpisodeStart, job.EpisodeEnd, segments,
	)
}

func structuredVisualEvidence(job VideoJob) *StructuredGraph {
	if len(job.EpisodeVisual) == 0 || strings.TrimSpace(job.SourceIdentity) == "" {
		return nil
	}
	evidenceID := "visual-evidence:" + strings.TrimSpace(job.SourceIdentity)
	graph := &StructuredGraph{
		EpisodeID: "second-brain:visual:" + job.SourceIdentity,
		SceneID:   strings.TrimSpace(job.SessionID),
		StartTime: job.EpisodeStart, EndTime: job.EpisodeEnd,
		Summary: job.VisualDescription, Location: job.Location,
		Entities:   []StructuredEntity{{CanonicalID: evidenceID, Name: "Visual evidence", Type: "VisualEvidence"}},
		Relations:  []StructuredRelation{},
		Utterances: []StructuredUtterance{},
	}
	seen := map[string]bool{evidenceID: true}
	if ownerID := canonicalOwnerID(job.OwnerUserID); ownerID != "" {
		seen[ownerID] = true
		graph.Entities = append(graph.Entities, StructuredEntity{
			CanonicalID: ownerID, Name: "Owner", Type: "Person",
		})
		graph.Relations = append(graph.Relations, StructuredRelation{
			Source: ownerID, Predicate: "HAS_VISUAL_CONTEXT", Target: evidenceID,
			Fact: "Owner has this visual evidence.",
		})
	}
	for _, observation := range job.EpisodeVisual {
		type entityRef struct{ id, name string }
		labels := make(map[string]entityRef)
		for _, person := range observation.People {
			label := strings.TrimSpace(person.VisualLabel)
			if label == "" {
				continue
			}
			id := strings.Join([]string{job.MediaAssetID, observation.ObservationID, label}, ":")
			name := "Unidentified " + strings.ReplaceAll(label, "-", " ")
			labels[label] = entityRef{id: id, name: name}
			if !seen[id] {
				seen[id] = true
				graph.Entities = append(graph.Entities, StructuredEntity{
					CanonicalID: id, Name: name,
					Type: "person", Confidence: person.Confidence,
				})
				fact := "Visual evidence shows " + name + "."
				if action := strings.TrimSpace(person.Action); action != "" {
					fact = "Visual evidence shows " + name + " " + action + "."
				}
				graph.Relations = append(graph.Relations, StructuredRelation{
					Source: evidenceID, Predicate: "OBSERVED_PERSON", Target: id,
					Fact: fact, Confidence: person.Confidence,
				})
			}
		}
		for index, object := range observation.Objects {
			label := strings.TrimSpace(object.ObjectID)
			if label == "" {
				label = fmt.Sprintf("object-%d", index+1)
			}
			id := strings.Join([]string{job.MediaAssetID, observation.ObservationID, label}, ":")
			name := strings.TrimSpace(object.Name)
			labels[label] = entityRef{id: id, name: name}
			if !seen[id] {
				seen[id] = true
				graph.Entities = append(graph.Entities, StructuredEntity{
					CanonicalID: id, Name: name, Type: "object",
					Confidence: object.Confidence,
				})
				graph.Relations = append(graph.Relations, StructuredRelation{
					Source: evidenceID, Predicate: "OBSERVED_OBJECT", Target: id,
					Fact: "Visual evidence shows " + name + ".", Confidence: object.Confidence,
				})
			}
		}
		if location := strings.TrimSpace(observation.LocationGuess); location != "" {
			placeID := evidenceID + ":place:" + stableShortID(strings.ToLower(location))
			if !seen[placeID] {
				seen[placeID] = true
				graph.Entities = append(graph.Entities, StructuredEntity{
					CanonicalID: placeID, Name: location, Type: "Place",
					Confidence: observation.Confidence,
				})
				graph.Relations = append(graph.Relations, StructuredRelation{
					Source: evidenceID, Predicate: "OBSERVED_AT", Target: placeID,
					Fact:       "Visual evidence appears to occur at " + location + ".",
					Confidence: observation.Confidence,
				})
			}
		}
		for _, relation := range observation.Relations {
			source, sourceOK := labels[strings.TrimSpace(relation.Source)]
			target, targetOK := labels[strings.TrimSpace(relation.Target)]
			if !sourceOK || !targetOK || strings.TrimSpace(relation.Predicate) == "" {
				continue
			}
			graph.Relations = append(graph.Relations, StructuredRelation{
				Source: source.id, Predicate: visualRelationPredicate(relation.Predicate), Target: target.id,
				Fact:       fmt.Sprintf("%s %s %s.", source.name, strings.TrimSpace(relation.Predicate), target.name),
				Confidence: relation.Confidence,
			})
		}
	}
	return graph
}

func stableShortID(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:8])
}

func visualRelationPredicate(value string) string {
	predicate := strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(strings.TrimSpace(value)))
	allowed := map[string]bool{
		"HOLDS": true, "LOOKS_AT": true, "INTERACTS_WITH": true, "PURCHASES": true,
		"PLAYS_WITH": true, "WALKS_THROUGH": true, "EXPLORES": true, "TALKS_TO": true,
		"SITS_ON": true, "STANDS_IN": true, "LOCATED_AT": true, "NEAR": true,
		"BESIDE": true, "ON": true, "INSIDE": true, "BEHIND": true,
		"IN_FRONT_OF": true, "ATTACHED_TO": true, "WEARS": true, "DISPLAYED_ON": true,
		"CONTAINS": true, "PICKS_UP": true, "CARRIES": true, "RIDES": true,
		"DRIVES": true, "ENTERS": true, "EXITS": true,
	}
	if allowed[predicate] {
		return predicate
	}
	return "INTERACTS_WITH"
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
			segment.SpeakerProfileID, segment.SpeakerName,
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
			Name:        conversationUtteranceName(text),
			Type:        "ConversationUtterance", Confidence: segment.Confidence,
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

func conversationUtteranceName(text string) string {
	const maximumRunes = 120
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maximumRunes {
		return string(runes)
	}
	return string(runes[:maximumRunes-3]) + "..."
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
	ownerID, sessionID, role, providerSpeaker string, persistentIdentity ...string,
) (string, string) {
	if role == "owner" {
		return ownerID, "Owner"
	}
	speakerProfileID := ""
	speakerName := ""
	if len(persistentIdentity) > 0 {
		speakerProfileID = persistentIdentity[0]
	}
	if len(persistentIdentity) > 1 {
		speakerName = persistentIdentity[1]
	}
	if speakerProfileID = strings.TrimSpace(speakerProfileID); speakerProfileID != "" {
		if speakerName = strings.TrimSpace(speakerName); speakerName == "" {
			speakerName = "Unlabeled speaker"
		}
		return "speaker-profile:" + speakerProfileID, speakerName
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
