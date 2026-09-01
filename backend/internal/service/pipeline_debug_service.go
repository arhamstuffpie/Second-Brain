package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type PipelineDebugService interface {
	Providers() PipelineDebugStatus
	AnalyzeFace(context.Context, PipelineDebugFile) (PipelineDebugRun, error)
	EmbedSpeaker(context.Context, PipelineDebugFile) (PipelineDebugRun, error)
	DetectActiveSpeaker(context.Context, PipelineDebugActiveSpeakerInput) (PipelineDebugRun, error)
}

type PipelineDebugFile struct {
	FileName  string
	MediaType string
	Content   []byte
}

type PipelineDebugActiveSpeakerInput struct {
	File         PipelineDebugFile
	RecordingID  string
	PersonTracks []TemporalPersonTrack
	Segments     []TranscriptSegment
}

type PipelineDebugProvider struct {
	Stage       string `json:"stage"`
	Enabled     bool   `json:"enabled"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	CostProfile string `json:"cost_profile"`
}

type PipelineDebugStatus struct {
	MemographCalled bool                    `json:"memograph_called"`
	Providers       []PipelineDebugProvider `json:"providers"`
}

type PipelineDebugRun struct {
	RunID           string    `json:"run_id"`
	Stage           string    `json:"stage"`
	StartedAt       time.Time `json:"started_at"`
	DurationMS      int64     `json:"duration_ms"`
	MemographCalled bool      `json:"memograph_called"`
	Request         any       `json:"request"`
	Response        any       `json:"response"`
}

type pipelineDebugService struct {
	face          FaceRecognizer
	speaker       SpeakerEmbedder
	activeSpeaker ActiveSpeakerDetector
}

func newPipelineDebugService(face FaceRecognizer, speaker SpeakerEmbedder, activeSpeaker ActiveSpeakerDetector) *pipelineDebugService {
	return &pipelineDebugService{face: face, speaker: speaker, activeSpeaker: activeSpeaker}
}

func (s *pipelineDebugService) Providers() PipelineDebugStatus {
	providers := []PipelineDebugProvider{
		{Stage: "face", Enabled: s.face != nil, CostProfile: "local"},
		{Stage: "speaker", Enabled: s.speaker != nil, CostProfile: "local"},
		{Stage: "active_speaker", Enabled: s.activeSpeaker != nil, CostProfile: "local"},
		{Stage: "stt", Enabled: false, Provider: "blocked in debug mode", CostProfile: "paid"},
		{Stage: "vision", Enabled: false, Provider: "blocked in debug mode", CostProfile: "paid"},
		{Stage: "memograph", Enabled: false, Provider: "never called by debug routes", CostProfile: "paid"},
	}
	if s.face != nil {
		providers[0].Provider, providers[0].Model = s.face.Provider(), s.face.Model()
	}
	if s.speaker != nil {
		providers[1].Provider = "speaker-embedding-http"
		if metadata, ok := s.speaker.(interface {
			Provider() string
			Model() string
		}); ok {
			providers[1].Provider, providers[1].Model = metadata.Provider(), metadata.Model()
		}
	}
	if s.activeSpeaker != nil {
		providers[2].Provider, providers[2].Model = s.activeSpeaker.Provider(), s.activeSpeaker.Model()
	}
	return PipelineDebugStatus{Providers: providers}
}

func (s *pipelineDebugService) AnalyzeFace(ctx context.Context, file PipelineDebugFile) (PipelineDebugRun, error) {
	if s.face == nil {
		return PipelineDebugRun{}, fmt.Errorf("face recognition provider is disabled")
	}
	return debugRun("face", file, func() (any, error) {
		result, err := s.face.Recognize(ctx, FaceRecognitionInput{
			FileName: file.FileName, MediaType: file.MediaType, Image: file.Content,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"provider": result.Provider, "detector": result.Detector, "model": result.Model,
			"dimensions": result.Dimensions, "faces": result.Faces,
		}, nil
	})
}

func (s *pipelineDebugService) EmbedSpeaker(ctx context.Context, file PipelineDebugFile) (PipelineDebugRun, error) {
	if s.speaker == nil {
		return PipelineDebugRun{}, fmt.Errorf("speaker embedding provider is disabled")
	}
	return debugRun("speaker", file, func() (any, error) {
		result, err := s.speaker.Embed(ctx, SpeakerEmbeddingInput{
			FileName: file.FileName, MediaType: file.MediaType, Audio: file.Content,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"model": result.Model, "dimensions": len(result.Vector), "embedding": result.Vector,
		}, nil
	})
}

func (s *pipelineDebugService) DetectActiveSpeaker(ctx context.Context, input PipelineDebugActiveSpeakerInput) (PipelineDebugRun, error) {
	if s.activeSpeaker == nil {
		return PipelineDebugRun{}, fmt.Errorf("active-speaker provider is disabled")
	}
	if strings.TrimSpace(input.RecordingID) == "" || len(input.PersonTracks) == 0 || len(input.Segments) == 0 {
		return PipelineDebugRun{}, fmt.Errorf("recording_id, person_tracks, and segments are required")
	}
	ext := filepath.Ext(input.File.FileName)
	temporary, err := os.CreateTemp("", "pipeline-debug-*"+ext)
	if err != nil {
		return PipelineDebugRun{}, fmt.Errorf("create temporary video: %w", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if _, err = temporary.Write(input.File.Content); err == nil {
		err = temporary.Close()
	}
	if err != nil {
		_ = temporary.Close()
		return PipelineDebugRun{}, fmt.Errorf("store temporary video: %w", err)
	}
	started := time.Now().UTC()
	result, err := s.activeSpeaker.DetectActiveSpeakers(ctx, ActiveSpeakerInput{
		RecordingID: input.RecordingID, VideoPath: path, FileName: input.File.FileName,
		MediaType: input.File.MediaType, PersonTracks: input.PersonTracks, Segments: input.Segments,
	})
	if err != nil {
		return PipelineDebugRun{}, err
	}
	return PipelineDebugRun{
		RunID: debugRunID(), Stage: "active_speaker", StartedAt: started,
		DurationMS: time.Since(started).Milliseconds(),
		Request: map[string]any{
			"file": debugFileSummary(input.File), "recording_id": input.RecordingID,
			"person_tracks": input.PersonTracks, "segments": input.Segments,
		},
		Response: result,
	}, nil
}

func debugRun(stage string, file PipelineDebugFile, call func() (any, error)) (PipelineDebugRun, error) {
	started := time.Now().UTC()
	result, err := call()
	if err != nil {
		return PipelineDebugRun{}, err
	}
	return PipelineDebugRun{
		RunID: debugRunID(), Stage: stage, StartedAt: started,
		DurationMS: time.Since(started).Milliseconds(), Request: debugFileSummary(file), Response: result,
	}, nil
}

func debugFileSummary(file PipelineDebugFile) map[string]any {
	return map[string]any{"file_name": file.FileName, "media_type": file.MediaType, "size_bytes": len(file.Content)}
}

func debugRunID() string {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + hex.EncodeToString(bytes)
}

var _ PipelineDebugService = (*pipelineDebugService)(nil)
