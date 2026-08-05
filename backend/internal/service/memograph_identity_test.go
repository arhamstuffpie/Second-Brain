package service

import (
	"encoding/json"
	"errors"
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

func TestOwnerAwareMemographDataUsesCanonicalIdentity(t *testing.T) {
	data := ownerAwareMemographData("Owner: I prefer tea.", "user-1")
	var payload struct {
		Schema         string `json:"schema"`
		CanonicalOwner struct {
			EntityID    string `json:"entity_id"`
			DisplayName string `json:"display_name"`
		} `json:"canonical_owner"`
		Episode string `json:"episode"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("decode owner-aware data: %v", err)
	}
	if payload.Schema != ownerAwareMemorySchema ||
		payload.CanonicalOwner.EntityID != "account-owner:user-1" ||
		payload.CanonicalOwner.DisplayName != "Owner" ||
		payload.Episode != "Owner: I prefer tea." {
		t.Fatalf("payload = %+v", payload)
	}
}
