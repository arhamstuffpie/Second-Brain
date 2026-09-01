package audio_analysis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
)

const maxResponseBytes = 256 << 20

type HTTPAnalyzer struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewHTTPAnalyzer(baseURL, apiKey, provider string, timeout time.Duration) (*HTTPAnalyzer, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("audio analyzer base URL is invalid")
	}
	if provider == "external" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("external audio analyzer base URL must use HTTPS")
	}
	if provider == "local" && parsed.Scheme == "http" && !loopback(parsed.Hostname()) {
		return nil, fmt.Errorf("unencrypted local audio analyzer URL must use a loopback host")
	}
	if provider != "local" && provider != "external" || timeout <= 0 {
		return nil, fmt.Errorf("valid audio analyzer provider and timeout are required")
	}
	return &HTTPAnalyzer{
		baseURL: strings.TrimRight(parsed.String(), "/"), apiKey: strings.TrimSpace(apiKey),
		client: &http.Client{Timeout: timeout},
	}, nil
}

func (a *HTTPAnalyzer) Validate(ctx context.Context) (service.ModelProvenance, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/readyz", nil)
	if err != nil {
		return service.ModelProvenance{}, err
	}
	a.authorize(request)
	response, err := a.client.Do(request)
	if err != nil {
		return service.ModelProvenance{}, fmt.Errorf("audio analyzer readiness request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return service.ModelProvenance{}, err
	}
	if response.StatusCode/100 != 2 {
		return service.ModelProvenance{}, fmt.Errorf("audio analyzer readiness returned %d: %s", response.StatusCode, bounded(body))
	}
	var payload struct {
		Status string `json:"status"`
		service.ModelProvenance
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return service.ModelProvenance{}, fmt.Errorf("decode audio analyzer readiness: %w", err)
	}
	if payload.Status != "ok" || payload.DiarizationModel == "" || payload.SeparationModel == "" || payload.RuntimeVersion == "" || payload.Device == "" {
		return service.ModelProvenance{}, fmt.Errorf("audio analyzer readiness response is incomplete")
	}
	return payload.ModelProvenance, nil
}

func (a *HTTPAnalyzer) AnalyzeAudio(ctx context.Context, input service.AudioAnalysisInput) (service.AudioAnalysis, error) {
	if strings.TrimSpace(input.RecordingID) == "" || input.ProcessingVersion < 1 || input.Audio == nil {
		return service.AudioAnalysis{}, fmt.Errorf("recording ID, processing version, and audio are required")
	}
	metadata, err := json.Marshal(map[string]any{
		"recording_id":       input.RecordingID,
		"processing_version": input.ProcessingVersion,
		"profile":            input.Profile,
	})
	if err != nil {
		return service.AudioAnalysis{}, err
	}
	name := filepath.Base(strings.TrimSpace(input.FileName))
	if name == "" || name == "." {
		name = "audio.wav"
	}
	body, contentType := streamingMultipart(input.Audio, name, metadata)
	defer body.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/audio-analysis", body)
	if err != nil {
		return service.AudioAnalysis{}, err
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", "application/json")
	a.authorize(request)
	response, err := a.client.Do(request)
	if err != nil {
		return service.AudioAnalysis{}, fmt.Errorf("audio analyzer request: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return service.AudioAnalysis{}, err
	}
	if len(payload) > maxResponseBytes {
		return service.AudioAnalysis{}, fmt.Errorf("audio analyzer response exceeds %d bytes", maxResponseBytes)
	}
	if response.StatusCode/100 != 2 {
		return service.AudioAnalysis{}, fmt.Errorf("audio analyzer returned %d: %s", response.StatusCode, bounded(payload))
	}
	var result service.AudioAnalysis
	if err := json.Unmarshal(payload, &result); err != nil {
		return service.AudioAnalysis{}, fmt.Errorf("decode audio analyzer response: %w", err)
	}
	if err := validate(result, input); err != nil {
		return service.AudioAnalysis{}, err
	}
	return result, nil
}

func streamingMultipart(source io.Reader, filename string, metadata []byte) (io.ReadCloser, string) {
	reader, pipe := io.Pipe()
	writer := multipart.NewWriter(pipe)
	contentType := writer.FormDataContentType()
	go func() {
		part, err := writer.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, source)
		}
		if err == nil {
			err = writer.WriteField("metadata", string(metadata))
		}
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		_ = pipe.CloseWithError(err)
	}()
	return reader, contentType
}

func validate(result service.AudioAnalysis, input service.AudioAnalysisInput) error {
	if result.RecordingID != input.RecordingID || result.ProcessingVersion != input.ProcessingVersion || result.DurationSeconds < 0 {
		return fmt.Errorf("audio analyzer response has incompatible identity or timing")
	}
	if result.Provenance.DiarizationModel == "" || result.Provenance.SeparationModel == "" || result.Provenance.RuntimeVersion == "" || result.Provenance.Device == "" {
		return fmt.Errorf("audio analyzer response is missing model provenance")
	}
	regions := make(map[string]struct{}, len(result.Regions))
	previousEnd := 0.0
	for _, region := range result.Regions {
		validShape := region.Kind == "silence" && region.ConcurrentSpeakerCount == 0 && !region.Overlap ||
			region.Kind == "speech" && region.ConcurrentSpeakerCount == 1 && !region.Overlap ||
			region.Kind == "overlap" && region.ConcurrentSpeakerCount > 1 && region.Overlap
		if region.ID == "" || region.StartTime < previousEnd || region.EndTime <= region.StartTime || region.EndTime > result.DurationSeconds || region.ConcurrentSpeakerCount != len(region.ActiveSpeakerLabels) || !validShape || !oneOf(region.Status, "queued", "processing", "completed", "retryable_failed", "dead", "budget_exhausted", "ambiguous") {
			return fmt.Errorf("audio analyzer response contains an invalid region")
		}
		if region.DiarizationConfidence != nil && !unit(*region.DiarizationConfidence) {
			return fmt.Errorf("audio analyzer response contains an invalid diarization confidence")
		}
		if _, exists := regions[region.ID]; exists {
			return fmt.Errorf("audio analyzer response contains duplicate region %q", region.ID)
		}
		regions[region.ID] = struct{}{}
		previousEnd = region.EndTime
	}
	sources := make(map[string]struct{}, len(result.Sources))
	for _, source := range result.Sources {
		if source.ID == "" || source.SourceIndex < 0 || !oneOf(source.SeparationStatus, "not_required", "accepted", "ambiguous", "rejected", "failed", "budget_exhausted") {
			return fmt.Errorf("audio analyzer response contains an invalid source")
		}
		if _, exists := regions[source.AudioRegionID]; !exists {
			return fmt.Errorf("audio source %q references an unknown region", source.ID)
		}
		if _, exists := sources[source.ID]; exists {
			return fmt.Errorf("audio analyzer response contains duplicate source %q", source.ID)
		}
		sources[source.ID] = struct{}{}
		for _, score := range []*float64{source.SeparationConfidence, source.ReconstructionScore} {
			if score != nil && !unit(*score) {
				return fmt.Errorf("audio source %q contains an invalid confidence", source.ID)
			}
		}
		for _, score := range []*float64{source.SpeakerMatchScore, source.SpeakerRunnerUpScore} {
			if score != nil && (math.IsNaN(*score) || math.IsInf(*score, 0) || *score < -1 || *score > 1) {
				return fmt.Errorf("audio source %q contains an invalid speaker score", source.ID)
			}
		}
		if source.AudioBase64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(source.AudioBase64)
			if err != nil || len(decoded) < 44 || string(decoded[:4]) != "RIFF" || string(decoded[8:12]) != "WAVE" {
				return fmt.Errorf("audio source %q contains invalid WAV data", source.ID)
			}
		}
	}
	return nil
}

func unit(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func (a *HTTPAnalyzer) authorize(request *http.Request) {
	if a.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func bounded(body []byte) string {
	message := strings.TrimSpace(string(body))
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}

var _ service.OverlapAudioAnalyzer = (*HTTPAnalyzer)(nil)
