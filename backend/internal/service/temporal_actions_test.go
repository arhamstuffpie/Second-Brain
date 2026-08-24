package service

import "testing"

func temporalObservation(at float64, state ObjectState) TemporalObservation {
	return TemporalObservation{
		Timestamp: at, ActorTrackID: "person-1", ObjectTrackID: "cup-1", State: state,
		TrackContinuity: true, InteractionVisible: true, ActorConfidence: .9,
		ObjectConfidence: .8, TransitionConfidence: .85,
		EvidenceFrameIDs: []string{"frame"}, ObservationID: "observation",
	}
}

func requireActions(t *testing.T, observations []TemporalObservation, want ...ActionType) {
	t.Helper()
	events, err := ValidateTemporalActions(observations)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != len(want) {
		t.Fatalf("actions = %#v, want %v", events, want)
	}
	for index := range want {
		if events[index].Type != want[index] {
			t.Fatalf("action %d = %s, want %s", index, events[index].Type, want[index])
		}
	}
}

func TestValidateTemporalActions(t *testing.T) {
	t.Run("reach without contact", func(t *testing.T) {
		requireActions(t, []TemporalObservation{
			temporalObservation(0, ObjectRestingOnSurface), temporalObservation(1, ObjectApproached),
		}, ActionReachFor)
	})
	t.Run("touch without control", func(t *testing.T) {
		contact := temporalObservation(1, ObjectContactOnly)
		contact.Supported = true
		requireActions(t, []TemporalObservation{temporalObservation(0, ObjectApproached), contact}, ActionTouch)
	})
	t.Run("grasp remains supported", func(t *testing.T) {
		control := temporalObservation(2, ObjectControlled)
		control.Supported = true
		requireActions(t, []TemporalObservation{
			temporalObservation(0, ObjectRestingOnSurface), temporalObservation(1, ObjectContactOnly), control,
		}, ActionGrasp)
	})
	t.Run("pick up requires before control and unsupported motion", func(t *testing.T) {
		control := temporalObservation(1, ObjectControlled)
		control.Supported = true
		moving := temporalObservation(2, ObjectMovingWithActor)
		requireActions(t, []TemporalObservation{temporalObservation(0, ObjectRestingOnSurface), control, moving}, ActionPickUp, ActionHold)
	})
	t.Run("clip starts held", func(t *testing.T) {
		requireActions(t, []TemporalObservation{
			temporalObservation(0, ObjectControlled), temporalObservation(1, ObjectMovingWithActor),
		}, ActionHold)
	})
	t.Run("put down requires visible support and release", func(t *testing.T) {
		lowered := temporalObservation(1, ObjectControlled)
		lowered.MovingTowardSupport = true
		released := temporalObservation(2, ObjectReleasedOnSurface)
		released.Supported, released.Released = true, true
		requireActions(t, []TemporalObservation{temporalObservation(0, ObjectControlled), lowered, released}, ActionPutDown)
	})
	t.Run("clip ends before release", func(t *testing.T) {
		lowered := temporalObservation(1, ObjectControlled)
		lowered.MovingTowardSupport = true
		requireActions(t, []TemporalObservation{temporalObservation(0, ObjectControlled), lowered}, ActionHold)
	})
	t.Run("drop follows loss of control", func(t *testing.T) {
		requireActions(t, []TemporalObservation{
			temporalObservation(0, ObjectControlled), temporalObservation(1, ObjectFalling),
		}, ActionDrop)
	})
	t.Run("overlapping actors are ambiguous", func(t *testing.T) {
		second := temporalObservation(1, ObjectApproached)
		second.ActorTrackID = "person-2"
		requireActions(t, []TemporalObservation{temporalObservation(0, ObjectApproached), second}, ActionUnknownInteraction)
	})
	t.Run("occluded track switch is ambiguous", func(t *testing.T) {
		occluded := temporalObservation(1, ObjectOccluded)
		occluded.TrackContinuity = false
		requireActions(t, []TemporalObservation{temporalObservation(0, ObjectControlled), occluded}, ActionUnknownInteraction)
	})
	t.Run("verified handover creates receive pair", func(t *testing.T) {
		second := temporalObservation(1, ObjectControlled)
		second.ActorTrackID = "person-2"
		requireActions(t, []TemporalObservation{temporalObservation(0, ObjectControlled), second}, ActionHandOver, ActionReceive)
	})
}
