package service

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMemographIdempotencyKeyIsStableAndBranchSpecific(t *testing.T) {
	first := memographIdempotencyKey("memory-1", "episode-1", "speech")
	second := memographIdempotencyKey("memory-1", "episode-1", "speech")
	visual := memographIdempotencyKey("memory-1", "episode-1", "visual")
	if first == "" || first != second {
		t.Fatalf("stable keys = %q and %q", first, second)
	}
	if first == visual {
		t.Fatalf("speech and visual keys are equal: %q", first)
	}
	refreshed := memographIdempotencyKey("memory-1", "episode-1", "speech", 1)
	if refreshed == first || refreshed != memographIdempotencyKey("memory-1", "episode-1", "speech", 1) {
		t.Fatalf("graph revision key = %q, original = %q", refreshed, first)
	}
}

type retryHintError struct{ delay time.Duration }

func (e retryHintError) Error() string             { return "retry later" }
func (e retryHintError) RetryDelay() time.Duration { return e.delay }

func TestRetryDelayForErrorUsesDependencyHint(t *testing.T) {
	err := errors.New("outer: " + retryHintError{delay: 30 * time.Second}.Error())
	if got := retryDelayForError(err, 1); got != time.Second {
		t.Fatalf("unwrapped retry delay = %s, want 1s", got)
	}
	wrapped := errors.Join(errors.New("outer"), retryHintError{delay: 30 * time.Second})
	if got := retryDelayForError(wrapped, 1); got != 30*time.Second {
		t.Fatalf("hinted retry delay = %s, want 30s", got)
	}
}

func TestStructuredConversationKeepsOneCanonicalOwnerAcrossSessions(t *testing.T) {
	first := structuredVoiceConversation(VoiceJob{
		EpisodeID: "episode-1", SessionID: "session-1", OwnerUserID: "user-1",
		Description: "First conversation", EpisodeStart: 0, EpisodeEnd: 2,
		EpisodeSegments: []EpisodeSegment{{
			StartTime: 0, EndTime: 1, Speaker: "owner_ref",
			SpeakerRole: "owner", Text: "What time is the meeting?",
		}, {
			StartTime: 1, EndTime: 2, Speaker: "A",
			SpeakerRole: "other", Text: "It starts at three.",
		}},
	})
	second := structuredVoiceConversation(VoiceJob{
		EpisodeID: "episode-2", SessionID: "session-2", OwnerUserID: "user-1",
		Description: "Second conversation", EpisodeStart: 5, EpisodeEnd: 7,
		EpisodeSegments: []EpisodeSegment{{
			StartTime: 5, EndTime: 6, Speaker: "owner_ref_2",
			SpeakerRole: "owner", Text: "Please send the notes.",
		}, {
			StartTime: 6, EndTime: 7, Speaker: "A",
			SpeakerRole: "other", Text: "I will send them.",
		}},
	})
	if first == nil || second == nil {
		t.Fatalf("structured conversations = %#v / %#v", first, second)
	}
	firstOwner := findStructuredEntity(first.Entities, "account-owner:user-1")
	secondOwner := findStructuredEntity(second.Entities, "account-owner:user-1")
	if firstOwner == nil || secondOwner == nil || firstOwner.Type != "Person" || secondOwner.Type != "Person" {
		t.Fatalf("canonical owners = %#v / %#v", firstOwner, secondOwner)
	}
	if len(first.Utterances) != 2 || first.Utterances[0].SpeakerID != firstOwner.CanonicalID ||
		len(first.Relations) != 4 || first.Relations[0].Predicate != "ASKED" {
		t.Fatalf("first structured conversation = %+v", first)
	}
	for _, entity := range first.Entities {
		if entity.Type == "ConversationEpisode" {
			t.Fatalf("synthetic conversation episode entity remains: %+v", entity)
		}
	}
	ownerQuestion := findStructuredEntityByText(first.Entities, "What time is the meeting?")
	otherStatement := findStructuredEntityByText(first.Entities, "It starts at three.")
	if ownerQuestion == nil || otherStatement == nil ||
		ownerQuestion.Type != "ConversationUtterance" ||
		otherStatement.Type != "ConversationUtterance" {
		t.Fatalf("conversation utterance entities = %+v", first.Entities)
	}
	if !hasStructuredRelation(first.Relations, firstOwner.CanonicalID, "ASKED", ownerQuestion.CanonicalID) ||
		!hasStructuredRelation(first.Relations, firstOwner.CanonicalID, "HAS_CONTEXT", otherStatement.CanonicalID) ||
		!hasStructuredRelation(first.Relations, ownerQuestion.CanonicalID, "FOLLOWED_BY", otherStatement.CanonicalID) {
		t.Fatalf("direct owner/sequence relations = %+v", first.Relations)
	}
	if hasStructuredRelation(first.Relations, firstOwner.CanonicalID, "SAID", otherStatement.CanonicalID) {
		t.Fatalf("non-owner statement became an owner fact: %+v", first.Relations)
	}
	if first.Entities[2].CanonicalID == second.Entities[2].CanonicalID {
		t.Fatalf("session-local speakers were conflated: %+v / %+v", first.Entities, second.Entities)
	}
}

func TestDefaultMemoryGroupIDIsStablePerAccount(t *testing.T) {
	if got := defaultMemoryGroupID(" user-1 "); got != "account-owner:user-1" {
		t.Fatalf("defaultMemoryGroupID() = %q", got)
	}
	if defaultMemoryGroupID("user-1") == defaultMemoryGroupID("user-2") {
		t.Fatal("different accounts share a default graph group")
	}
}

func TestStructuredConversationKeepsRepeatedUtterancesDistinct(t *testing.T) {
	graph := buildStructuredConversation(
		"episode-1", "audio", "session-1", "user-1", "", "Repeated reply",
		0, 2,
		[]groundedConversationSegment{
			{StartTime: 0, EndTime: 1, Role: "owner", Text: "Okay."},
			{StartTime: 1, EndTime: 2, Role: "owner", Text: "Okay."},
		},
	)
	if graph == nil {
		t.Fatal("structured conversation is nil")
	}
	utterances := make([]StructuredEntity, 0, 2)
	for _, entity := range graph.Entities {
		if entity.Type == "ConversationUtterance" {
			utterances = append(utterances, entity)
		}
	}
	if len(utterances) != 2 ||
		utterances[0].CanonicalID == utterances[1].CanonicalID ||
		utterances[0].Name != "Okay." || utterances[1].Name != "Okay." {
		t.Fatalf("repeated utterance entities = %+v", utterances)
	}
}

func TestStructuredConversationUsesCleanTitlesAndConfirmedSpeakerName(t *testing.T) {
	graph := buildStructuredConversation(
		"episode-1", "audio", "session-1", "user-1", "", "Conversation",
		0, 2,
		[]groundedConversationSegment{{
			StartTime: 0, EndTime: 2, Role: "other", Speaker: "A",
			SpeakerProfileID: "profile-1", SpeakerName: "Raj",
			Text: "Game management decides close matches.",
		}},
	)
	if graph == nil {
		t.Fatal("structured conversation is nil")
	}
	speaker := findStructuredEntity(graph.Entities, "speaker-profile:profile-1")
	utterance := findStructuredEntityByText(graph.Entities, "Game management decides close matches.")
	if speaker == nil || speaker.Name != "Raj" || utterance == nil ||
		utterance.Name != "Game management decides close matches." ||
		strings.Contains(utterance.Name, " @ ") {
		t.Fatalf("speaker/utterance entities = %+v", graph.Entities)
	}
}

func findStructuredEntity(entities []StructuredEntity, canonicalID string) *StructuredEntity {
	for index := range entities {
		if entities[index].CanonicalID == canonicalID {
			return &entities[index]
		}
	}
	return nil
}

func findStructuredEntityByText(entities []StructuredEntity, text string) *StructuredEntity {
	for index := range entities {
		if entities[index].Name == text || strings.HasPrefix(entities[index].Name, text+" [") {
			return &entities[index]
		}
	}
	return nil
}

func hasStructuredRelation(
	relations []StructuredRelation, source, predicate, target string,
) bool {
	for _, relation := range relations {
		if relation.Source == source && relation.Predicate == predicate && relation.Target == target {
			return true
		}
	}
	return false
}

func TestStructuredVisualEvidenceUsesObservationLocalIDs(t *testing.T) {
	confidence := 0.9
	graph := structuredVisualEvidence(VideoJob{
		SourceIdentity: "window-1", SessionID: "session-1", MediaAssetID: "asset-1", OwnerUserID: "user-1",
		EpisodeStart: 0, EpisodeEnd: 5, VisualDescription: "A person holds a document.",
		EpisodeVisual: []VideoObservation{{
			ObservationID: "obs-1", LocationGuess: "office",
			People:    []VisualPerson{{VisualLabel: "person-1", Action: "holding an agreement", Confidence: &confidence}},
			Objects:   []DetectedObject{{ObjectID: "document-1", Name: "Agreement", Confidence: &confidence}},
			Relations: []VisualRelation{{Source: "person-1", Predicate: "holds", Target: "document-1", Confidence: &confidence}},
		}},
	})
	if graph == nil {
		t.Fatalf("visual graph = %+v", graph)
	}
	evidence := findStructuredEntity(graph.Entities, "visual-evidence:window-1")
	owner := findStructuredEntity(graph.Entities, "account-owner:user-1")
	person := findStructuredEntity(graph.Entities, "asset-1:obs-1:person-1")
	document := findStructuredEntity(graph.Entities, "asset-1:obs-1:document-1")
	if evidence == nil || evidence.Name != "Visual evidence" || owner == nil || person == nil || document == nil {
		t.Fatalf("visual entities = %+v", graph.Entities)
	}
	if strings.Contains(evidence.Name, "0") || strings.Contains(evidence.CanonicalID, "0-5") ||
		!hasStructuredRelation(graph.Relations, owner.CanonicalID, "HAS_VISUAL_CONTEXT", evidence.CanonicalID) ||
		!hasStructuredRelation(graph.Relations, evidence.CanonicalID, "OBSERVED_PERSON", person.CanonicalID) ||
		!hasStructuredRelation(graph.Relations, evidence.CanonicalID, "OBSERVED_OBJECT", document.CanonicalID) ||
		!hasStructuredRelation(graph.Relations, person.CanonicalID, "HOLDS", document.CanonicalID) {
		t.Fatalf("visual graph = %+v", graph)
	}
	for _, entity := range graph.Entities {
		if entity.CanonicalID == evidence.CanonicalID || entity.CanonicalID == owner.CanonicalID {
			continue
		}
		connected := false
		for _, relation := range graph.Relations {
			if relation.Source == entity.CanonicalID || relation.Target == entity.CanonicalID {
				connected = true
				break
			}
		}
		if !connected {
			t.Fatalf("orphan visual entity %q in %+v", entity.CanonicalID, graph.Relations)
		}
	}
}

func TestStructuredVisualEvidenceReusesResolvedPersonAcrossSessions(t *testing.T) {
	job := func(source, session, asset, observation string) VideoJob {
		return VideoJob{
			SourceIdentity: source, SessionID: session, MediaAssetID: asset, OwnerUserID: "user-1",
			EpisodeStart: 0, EpisodeEnd: 5,
			EpisodeVisual: []VideoObservation{{
				ObservationID: observation,
				People: []VisualPerson{{
					VisualLabel: "person-1", PersonProfileID: "person-42",
					PersonIdentityStatus: "confirmed", PersonName: "Mark",
				}},
			}},
		}
	}
	first := structuredVisualEvidence(job("window-1", "session-1", "asset-1", "obs-1"))
	second := structuredVisualEvidence(job("window-2", "session-2", "asset-2", "obs-2"))
	firstPerson := findStructuredEntity(first.Entities, "person-profile:person-42")
	secondPerson := findStructuredEntity(second.Entities, "person-profile:person-42")
	if firstPerson == nil || secondPerson == nil || firstPerson.Name != "Mark" || secondPerson.Name != "Mark" {
		t.Fatalf("resolved people = %+v / %+v", first.Entities, second.Entities)
	}
}

func TestConfirmedVoiceAndVisualIdentityUseSameCanonicalPerson(t *testing.T) {
	voice := structuredVideoConversation(VideoJob{
		EpisodeID: "speech-1", SessionID: "session-1", OwnerUserID: "user-1",
		EpisodeStart: 0, EpisodeEnd: 2,
		Transcript: Transcript{Segments: []TranscriptSegment{{
			StartTime: 0, EndTime: 2, Speaker: "A", SpeakerRole: "other",
			SpeakerProfileID: "voice-1", PersonProfileID: "person-42", SpeakerName: "Mark",
			Text: "Hello.",
		}}},
	})
	visual := structuredVisualEvidence(VideoJob{
		SourceIdentity: "visual-1", SessionID: "session-1", MediaAssetID: "asset-1",
		OwnerUserID: "user-1", EpisodeStart: 0, EpisodeEnd: 2,
		EpisodeVisual: []VideoObservation{{ObservationID: "observation-1", People: []VisualPerson{{
			VisualLabel: "person-1", PersonProfileID: "person-42", PersonName: "Mark",
		}}}},
	})
	if voice == nil || visual == nil ||
		findStructuredEntity(voice.Entities, "person-profile:person-42") == nil ||
		findStructuredEntity(visual.Entities, "person-profile:person-42") == nil {
		t.Fatalf("voice/visual entities = %+v / %+v", voice, visual)
	}
	if findStructuredEntity(voice.Entities, "speaker-profile:voice-1") != nil {
		t.Fatalf("linked voice retained a separate speaker node: %+v", voice.Entities)
	}
}

func TestVisualRelationPredicateKeepsVocabularyBounded(t *testing.T) {
	if visualRelationPredicate("plays with") != "PLAYS_WITH" ||
		visualRelationPredicate("invented predicate") != "INTERACTS_WITH" {
		t.Fatal("visual relation predicate was not normalized")
	}
}
