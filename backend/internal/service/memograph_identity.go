package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const ownerAwareMemorySchema = "owner-aware-conversation-v1"

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

func addOwnerIdentityMeta(meta map[string]any, ownerUserID string) {
	if ownerID := canonicalOwnerID(ownerUserID); ownerID != "" {
		meta["owner_user_id"] = strings.TrimSpace(ownerUserID)
		meta["owner_entity_id"] = ownerID
		meta["owner_entity_name"] = "Owner"
	}
}

func ownerAwareMemographData(data, ownerUserID string) string {
	ownerID := canonicalOwnerID(ownerUserID)
	if ownerID == "" || strings.TrimSpace(data) == "" {
		return data
	}
	payload := struct {
		Schema         string `json:"schema"`
		CanonicalOwner struct {
			EntityID    string `json:"entity_id"`
			DisplayName string `json:"display_name"`
		} `json:"canonical_owner"`
		IdentityRule string `json:"identity_rule"`
		Episode      string `json:"episode"`
	}{
		Schema: ownerAwareMemorySchema,
		IdentityRule: "Every Owner-labeled utterance refers to canonical_owner. " +
			"Do not create another person entity for Owner, device owner, account holder, or first-person pronouns.",
		Episode: data,
	}
	payload.CanonicalOwner.EntityID = ownerID
	payload.CanonicalOwner.DisplayName = "Owner"
	encoded, err := json.Marshal(payload)
	if err != nil {
		return data
	}
	return string(encoded)
}
