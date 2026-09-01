package service

import (
	"fmt"
	"sort"
)

type ObjectState string

const (
	ObjectRestingOnSurface  ObjectState = "resting_on_surface"
	ObjectApproached        ObjectState = "approached"
	ObjectContactOnly       ObjectState = "contact_only"
	ObjectControlled        ObjectState = "controlled_by_actor"
	ObjectMovingWithActor   ObjectState = "moving_with_actor"
	ObjectReleasedOnSurface ObjectState = "released_on_surface"
	ObjectFalling           ObjectState = "falling"
	ObjectOccluded          ObjectState = "occluded"
	ObjectUnknown           ObjectState = "unknown"
)

type ActionType string

const (
	ActionReachFor           ActionType = "REACH_FOR"
	ActionTouch              ActionType = "TOUCH"
	ActionGrasp              ActionType = "GRASP"
	ActionPickUp             ActionType = "PICK_UP"
	ActionHold               ActionType = "HOLD"
	ActionCarry              ActionType = "CARRY"
	ActionPutDown            ActionType = "PUT_DOWN"
	ActionDrop               ActionType = "DROP"
	ActionHandOver           ActionType = "HAND_OVER"
	ActionReceive            ActionType = "RECEIVE"
	ActionUnknownInteraction ActionType = "UNKNOWN_INTERACTION"
)

// TemporalObservation is the deterministic boundary between an inference
// provider and the state-transition validator. Appearance is deliberately not
// part of the contract: actor and object continuity must come from local tracks.
type TemporalObservation struct {
	Timestamp            float64     `json:"timestamp"`
	ActorTrackID         string      `json:"actor_track_id"`
	ObjectTrackID        string      `json:"object_track_id"`
	State                ObjectState `json:"state"`
	Supported            bool        `json:"supported"`
	ActorMoving          bool        `json:"actor_moving"`
	MovingTowardSupport  bool        `json:"moving_toward_support"`
	Released             bool        `json:"released"`
	TrackContinuity      bool        `json:"track_continuity"`
	InteractionVisible   bool        `json:"interaction_visible"`
	ActorConfidence      float64     `json:"actor_confidence"`
	ObjectConfidence     float64     `json:"object_confidence"`
	TransitionConfidence float64     `json:"transition_confidence"`
	EvidenceFrameIDs     []string    `json:"evidence_frame_ids"`
	ObservationID        string      `json:"observation_id"`
}

type ActionConfidence struct {
	ActorTracking   float64 `json:"actor_tracking"`
	ObjectTracking  float64 `json:"object_tracking"`
	StateTransition float64 `json:"state_transition"`
}

type ActionEvent struct {
	ActorTrackID          string           `json:"actor_track_id"`
	SecondaryActorTrackID string           `json:"secondary_actor_track_id,omitempty"`
	ObjectTrackID         string           `json:"object_track_id"`
	Type                  ActionType       `json:"action_type"`
	StartTime             float64          `json:"start_time"`
	EndTime               float64          `json:"end_time"`
	BeforeState           ObjectState      `json:"before_state"`
	AfterState            ObjectState      `json:"after_state"`
	Confidence            ActionConfidence `json:"component_confidence"`
	OverallConfidence     float64          `json:"overall_confidence"`
	AmbiguityReasons      []string         `json:"ambiguity_reasons"`
	EvidenceFrameIDs      []string         `json:"evidence_frame_ids"`
	ObservationIDs        []string         `json:"observation_ids"`
}

// ValidateTemporalActions turns ordered provider observations into conservative
// events. It never fills a missing before/after phase with an inference.
func ValidateTemporalActions(input []TemporalObservation) ([]ActionEvent, error) {
	byObject := make(map[string][]TemporalObservation)
	for _, observation := range input {
		if observation.Timestamp < 0 || observation.ActorTrackID == "" || observation.ObjectTrackID == "" {
			return nil, fmt.Errorf("temporal observation requires non-negative time and stable actor/object tracks")
		}
		if !validObjectState(observation.State) {
			return nil, fmt.Errorf("unsupported object state %q", observation.State)
		}
		byObject[observation.ObjectTrackID] = append(byObject[observation.ObjectTrackID], observation)
	}
	var events []ActionEvent
	for _, observations := range byObject {
		sort.SliceStable(observations, func(i, j int) bool { return observations[i].Timestamp < observations[j].Timestamp })
		events = append(events, validateObjectActions(observations)...)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].StartTime == events[j].StartTime {
			return events[i].Type < events[j].Type
		}
		return events[i].StartTime < events[j].StartTime
	})
	if events == nil {
		events = []ActionEvent{}
	}
	return events, nil
}

func validateObjectActions(observations []TemporalObservation) []ActionEvent {
	if len(observations) == 0 {
		return nil
	}
	for index := 1; index < len(observations); index++ {
		previous, current := observations[index-1], observations[index]
		if isControlled(previous.State) && isControlled(current.State) &&
			previous.ActorTrackID != current.ActorTrackID {
			if !previous.TrackContinuity || !current.TrackContinuity || previous.State == ObjectOccluded || current.State == ObjectOccluded {
				return []ActionEvent{newActionEvent(ActionUnknownInteraction, observations, previous.ActorTrackID, "object-track continuity is uncertain during transfer")}
			}
			handOver := newActionEvent(ActionHandOver, observations[index-1:index+1], previous.ActorTrackID, "")
			handOver.SecondaryActorTrackID = current.ActorTrackID
			receive := newActionEvent(ActionReceive, observations[index-1:index+1], current.ActorTrackID, "")
			receive.SecondaryActorTrackID = previous.ActorTrackID
			return []ActionEvent{handOver, receive}
		}
	}

	actor := observations[0].ActorTrackID
	for _, observation := range observations {
		if observation.ActorTrackID != actor {
			return []ActionEvent{newActionEvent(ActionUnknownInteraction, observations, actor, "multiple actors overlap without verified control transfer")}
		}
	}
	if interactionUncertain(observations) {
		return []ActionEvent{newActionEvent(ActionUnknownInteraction, observations, actor, "transition is occluded or object-track continuity is uncertain")}
	}

	first := observations[0]
	startsHeld := isControlled(first.State)
	resting := stateIndex(observations, ObjectRestingOnSurface)
	contact := stateIndex(observations, ObjectContactOnly)
	controlled := stateIndex(observations, ObjectControlled)
	moving := stateIndex(observations, ObjectMovingWithActor)
	released := stateIndex(observations, ObjectReleasedOnSurface)
	falling := stateIndex(observations, ObjectFalling)

	if controlled >= 0 && falling > controlled {
		return []ActionEvent{newActionEvent(ActionDrop, observations[controlled:falling+1], actor, "")}
	}
	if startsHeld && released > 0 {
		if observations[released].Released && hasMoveTowardSupport(observations[:released+1]) {
			return []ActionEvent{newActionEvent(ActionPutDown, observations[:released+1], actor, "")}
		}
		return []ActionEvent{newActionEvent(ActionUnknownInteraction, observations, actor, "release or visible support transition is missing")}
	}
	if resting >= 0 && controlled > resting && moving > controlled &&
		observations[controlled].Supported && !observations[moving].Supported {
		pickup := newActionEvent(ActionPickUp, observations[resting:moving+1], actor, "")
		hold := newActionEvent(ActionHold, observations[moving:], actor, "")
		return []ActionEvent{pickup, hold}
	}
	if startsHeld {
		events := []ActionEvent{newActionEvent(ActionHold, observations, actor, "")}
		if moving >= 0 && hasActorMovement(observations[moving:]) {
			events = append(events, newActionEvent(ActionCarry, observations[moving:], actor, ""))
		}
		return events
	}
	if controlled >= 0 {
		if observations[controlled].Supported {
			return []ActionEvent{newActionEvent(ActionGrasp, observations[max(0, contact):controlled+1], actor, "")}
		}
		return []ActionEvent{newActionEvent(ActionUnknownInteraction, observations, actor, "independent support before control is not visible")}
	}
	if contact >= 0 {
		return []ActionEvent{newActionEvent(ActionTouch, observations[contact:contact+1], actor, "")}
	}
	if stateIndex(observations, ObjectApproached) >= 0 {
		return []ActionEvent{newActionEvent(ActionReachFor, observations, actor, "")}
	}
	for _, observation := range observations {
		if observation.InteractionVisible {
			return []ActionEvent{newActionEvent(ActionUnknownInteraction, observations, actor, "visible interaction has no verified state transition")}
		}
	}
	return nil
}

func newActionEvent(action ActionType, observations []TemporalObservation, actor, ambiguity string) ActionEvent {
	first, last := observations[0], observations[len(observations)-1]
	event := ActionEvent{
		ActorTrackID: actor, ObjectTrackID: first.ObjectTrackID, Type: action,
		StartTime: first.Timestamp, EndTime: last.Timestamp,
		BeforeState: first.State, AfterState: last.State,
		Confidence:       ActionConfidence{ActorTracking: 1, ObjectTracking: 1, StateTransition: 1},
		EvidenceFrameIDs: []string{}, ObservationIDs: []string{}, AmbiguityReasons: []string{},
	}
	seenFrames := map[string]bool{}
	seenObservations := map[string]bool{}
	for _, observation := range observations {
		event.Confidence.ActorTracking = confidenceMin(event.Confidence.ActorTracking, observation.ActorConfidence)
		event.Confidence.ObjectTracking = confidenceMin(event.Confidence.ObjectTracking, observation.ObjectConfidence)
		event.Confidence.StateTransition = confidenceMin(event.Confidence.StateTransition, observation.TransitionConfidence)
		for _, id := range observation.EvidenceFrameIDs {
			if id != "" && !seenFrames[id] {
				seenFrames[id] = true
				event.EvidenceFrameIDs = append(event.EvidenceFrameIDs, id)
			}
		}
		if observation.ObservationID != "" && !seenObservations[observation.ObservationID] {
			seenObservations[observation.ObservationID] = true
			event.ObservationIDs = append(event.ObservationIDs, observation.ObservationID)
		}
	}
	event.OverallConfidence = min(event.Confidence.ActorTracking, event.Confidence.ObjectTracking, event.Confidence.StateTransition)
	if ambiguity != "" {
		event.AmbiguityReasons = append(event.AmbiguityReasons, ambiguity)
	}
	return event
}

func confidenceMin(current, value float64) float64 {
	if value < 0 || value > 1 {
		return 0
	}
	return min(current, value)
}

func validObjectState(state ObjectState) bool {
	switch state {
	case ObjectRestingOnSurface, ObjectApproached, ObjectContactOnly, ObjectControlled,
		ObjectMovingWithActor, ObjectReleasedOnSurface, ObjectFalling, ObjectOccluded, ObjectUnknown:
		return true
	default:
		return false
	}
}

func stateIndex(observations []TemporalObservation, state ObjectState) int {
	for index, observation := range observations {
		if observation.State == state {
			return index
		}
	}
	return -1
}

func isControlled(state ObjectState) bool {
	return state == ObjectControlled || state == ObjectMovingWithActor
}

func interactionUncertain(observations []TemporalObservation) bool {
	for _, observation := range observations {
		if observation.State == ObjectOccluded || !observation.TrackContinuity {
			return true
		}
	}
	return false
}

func hasMoveTowardSupport(observations []TemporalObservation) bool {
	for _, observation := range observations {
		if observation.MovingTowardSupport {
			return true
		}
	}
	return false
}

func hasActorMovement(observations []TemporalObservation) bool {
	for _, observation := range observations {
		if observation.ActorMoving {
			return true
		}
	}
	return false
}
