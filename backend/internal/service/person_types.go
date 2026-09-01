package service

import (
	"io"
	"time"
)

type FaceEnrollmentInput struct {
	OwnerUserID          string
	PersonProfileID      string
	DisplayName          string
	RelationshipCategory string
	RelationshipLabel    string
	ConsentConfirmed     bool
	FileName             string
	MediaType            string
	Content              io.Reader
}

type FaceRecognitionRequest struct {
	OwnerUserID string
	FileName    string
	MediaType   string
	Content     io.Reader
}

type EnrollFaceProfileInput struct {
	OwnerUserID          string
	PersonProfileID      string
	DisplayName          string
	RelationshipCategory string
	RelationshipLabel    string
	ConsentState         string
	Provider             string
	DetectorModel        string
	EmbeddingModel       string
	Embedding            []float64
	FileName             string
	FilePath             string
	MediaType            string
	SizeBytes            int64
	DetectionScore       float64
	Quality              FaceQuality
	Pose                 FacePose
	SourceRecordingID    string
	SourceTrackID        string
	ObservedAt           time.Time
	ProvisionalTTL       time.Duration
}

type MatchFaceProfileInput struct {
	OwnerUserID     string
	Provider        string
	EmbeddingModel  string
	Embedding       []float64
	PoseBucket      string
	MatchThreshold  float64
	AmbiguousMargin float64
}

type SavePersonTrackInput struct {
	ID                      string
	OwnerUserID             string
	RecordingID             string
	StartTime               float64
	EndTime                 float64
	TemporaryVisualLabel    string
	ResolvedPersonProfileID string
	TrackingConfidence      float64
	EvidenceFrameIDs        []string
	ProcessingVersion       int
}

type UpdatePersonInput struct {
	ID                   string `json:"-"`
	OwnerUserID          string `json:"-"`
	DisplayName          string `json:"display_name"`
	RelationshipCategory string `json:"relationship_category"`
	RelationshipLabel    string `json:"relationship_label"`
}

type ConfirmPersonIdentityInput struct {
	OwnerUserID           string   `json:"-"`
	RecordingIDs          []string `json:"recording_ids"`
	VisualLabel           string   `json:"visual_label"`
	VoiceSpeakerProfileID string   `json:"voice_speaker_profile_id"`
	Confirmed             bool     `json:"confirmed"`
}

type AutomaticIdentityEvidenceInput struct {
	OwnerUserID              string
	RecordingID              string
	PersonTrackID            string
	VoiceSpeakerProfileID    string
	SegmentIDs               []string
	ActiveSpeakerScore       float64
	VisibleMouthCoverage     float64
	TemporalCoverage         float64
	OverlappingConflict      bool
	Decision                 string
	FaceProvider             string
	FaceModel                string
	ActiveSpeakerProvider    string
	ActiveSpeakerModel       string
	ProcessingVersion        int
	AutoMerge                bool
	MergeEvidenceRequirement int
}

type AutomaticIdentityResolution struct {
	PersonTrackID         string
	VoiceSpeakerProfileID string
	PersonProfileID       string
	DisplayName           string
	MergedFromProfileID   string
	Decision              string
}

type PersonProfile struct {
	ID                   string     `json:"id"`
	Status               string     `json:"status"`
	DisplayName          string     `json:"display_name"`
	RelationshipCategory string     `json:"relationship_category"`
	RelationshipLabel    string     `json:"relationship_label"`
	ConsentState         string     `json:"consent_state"`
	FaceProfileCount     int        `json:"face_profile_count"`
	VoiceProfileIDs      []string   `json:"voice_profile_ids"`
	FirstSeenAt          time.Time  `json:"first_seen_at"`
	LastSeenAt           time.Time  `json:"last_seen_at"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type FaceMatch struct {
	Matched            bool        `json:"matched"`
	Ambiguous          bool        `json:"ambiguous"`
	PersonProfileID    string      `json:"person_profile_id,omitempty"`
	DisplayName        string      `json:"display_name,omitempty"`
	IdentityStatus     string      `json:"identity_status,omitempty"`
	Similarity         *float64    `json:"similarity,omitempty"`
	RunnerUpSimilarity *float64    `json:"runner_up_similarity,omitempty"`
	Quality            FaceQuality `json:"quality"`
	Reasons            []string    `json:"reasons"`
}
