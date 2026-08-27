package person_tracking

import (
	"context"
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

const maxResponseBytes = 128 << 20

type HTTPAnalyzer struct {
	baseURL        string
	apiKey         string
	detectorModel  string
	embeddingModel string
	client         *http.Client
}

func NewHTTPAnalyzer(baseURL, apiKey, detectorModel, embeddingModel, provider string, timeout time.Duration) (*HTTPAnalyzer, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("person analyzer base URL is invalid")
	}
	if provider == "external" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("external person analyzer base URL must use HTTPS")
	}
	if provider == "local" && parsed.Scheme == "http" && !loopback(parsed.Hostname()) {
		return nil, fmt.Errorf("unencrypted local person analyzer URL must use a loopback host")
	}
	if provider != "local" && provider != "external" {
		return nil, fmt.Errorf("person analyzer provider must be local or external")
	}
	if strings.TrimSpace(detectorModel) == "" || strings.TrimSpace(embeddingModel) == "" || timeout <= 0 {
		return nil, fmt.Errorf("person analyzer models and timeout are required")
	}
	return &HTTPAnalyzer{
		baseURL: strings.TrimRight(parsed.String(), "/"), apiKey: strings.TrimSpace(apiKey),
		detectorModel: detectorModel, embeddingModel: embeddingModel,
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
		return service.ModelProvenance{}, fmt.Errorf("person analyzer readiness request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return service.ModelProvenance{}, err
	}
	if response.StatusCode/100 != 2 {
		return service.ModelProvenance{}, fmt.Errorf("person analyzer readiness returned %d: %s", response.StatusCode, bounded(body))
	}
	var payload struct {
		Status string `json:"status"`
		service.ModelProvenance
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return service.ModelProvenance{}, fmt.Errorf("decode person analyzer readiness: %w", err)
	}
	if payload.Status != "ok" || payload.DetectorModel != a.detectorModel || payload.EmbeddingModel != a.embeddingModel || payload.RuntimeVersion == "" || payload.Device == "" {
		return service.ModelProvenance{}, fmt.Errorf("person analyzer readiness is incompatible with configured models")
	}
	return payload.ModelProvenance, nil
}

func (a *HTTPAnalyzer) AnalyzePeople(ctx context.Context, input service.DensePersonAnalysisInput) (service.DensePersonAnalysis, error) {
	if strings.TrimSpace(input.RecordingID) == "" || input.ProcessingVersion < 1 || input.Video == nil {
		return service.DensePersonAnalysis{}, fmt.Errorf("recording ID, processing version, and video are required")
	}
	metadata, err := json.Marshal(map[string]any{
		"recording_id": input.RecordingID, "processing_version": input.ProcessingVersion,
		"detector_model": a.detectorModel, "embedding_model": a.embeddingModel,
		"profile": input.Profile,
	})
	if err != nil {
		return service.DensePersonAnalysis{}, err
	}
	name := filepath.Base(strings.TrimSpace(input.FileName))
	if name == "" || name == "." {
		name = "recording.mp4"
	}
	body, contentType := streamingMultipart(input.Video, name, metadata)
	defer body.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/person-tracks", body)
	if err != nil {
		return service.DensePersonAnalysis{}, err
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", "application/json")
	a.authorize(request)
	response, err := a.client.Do(request)
	if err != nil {
		return service.DensePersonAnalysis{}, fmt.Errorf("person analyzer request: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return service.DensePersonAnalysis{}, err
	}
	if len(payload) > maxResponseBytes {
		return service.DensePersonAnalysis{}, fmt.Errorf("person analyzer response exceeds %d bytes", maxResponseBytes)
	}
	if response.StatusCode/100 != 2 {
		return service.DensePersonAnalysis{}, fmt.Errorf("person analyzer returned %d: %s", response.StatusCode, bounded(payload))
	}
	var result service.DensePersonAnalysis
	if err := json.Unmarshal(payload, &result); err != nil {
		return service.DensePersonAnalysis{}, fmt.Errorf("decode person analyzer response: %w", err)
	}
	if err := validate(result, input); err != nil {
		return service.DensePersonAnalysis{}, err
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

func validate(result service.DensePersonAnalysis, input service.DensePersonAnalysisInput) error {
	if result.RecordingID != input.RecordingID || result.ProcessingVersion != input.ProcessingVersion || result.DurationSeconds < 0 || result.AnalyzedFPS <= 0 {
		return fmt.Errorf("person analyzer response has incompatible identity or timing")
	}
	if result.Provenance.DetectorModel == "" || result.Provenance.EmbeddingModel == "" || result.Provenance.RuntimeVersion == "" || result.Provenance.Device == "" {
		return fmt.Errorf("person analyzer response is missing model provenance")
	}
	seenTracks := make(map[string]struct{}, len(result.Tracks))
	seenObservations := make(map[string]struct{})
	for _, track := range result.Tracks {
		if track.ID == "" || !oneOf(track.LifecycleStatus, "tentative", "confirmed", "lost", "ended") || track.FirstFrame < 0 || track.LastFrame < track.FirstFrame || track.StartTime < 0 || track.EndTime < track.StartTime || track.EndTime > result.DurationSeconds || track.ObservationCount != len(track.Observations) || !unit(track.TrackingConfidence) || !unit(track.Quality.Mean) || !unit(track.Quality.Maximum) {
			return fmt.Errorf("person analyzer response contains an invalid track")
		}
		if _, exists := seenTracks[track.ID]; exists {
			return fmt.Errorf("person analyzer response contains duplicate track %q", track.ID)
		}
		seenTracks[track.ID] = struct{}{}
		trackObservations := make(map[string]struct{}, len(track.Observations))
		for _, observation := range track.Observations {
			if observation.ObservationID == "" || observation.FrameIndex < track.FirstFrame || observation.FrameIndex > track.LastFrame || observation.Timestamp < track.StartTime || observation.Timestamp > track.EndTime || observation.Box.Width <= 0 || observation.Box.Height <= 0 || !unit(observation.DetectionScore) || !unit(observation.Quality.Score) || !unit(observation.MouthActivity) {
				return fmt.Errorf("person analyzer response contains an invalid observation")
			}
			for _, value := range observation.Embedding {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return fmt.Errorf("person analyzer response contains a non-finite embedding")
				}
			}
			if _, exists := seenObservations[observation.ObservationID]; exists {
				return fmt.Errorf("person analyzer response contains duplicate observation %q", observation.ObservationID)
			}
			seenObservations[observation.ObservationID] = struct{}{}
			trackObservations[observation.ObservationID] = struct{}{}
		}
		for _, observationID := range track.GalleryObservationIDs {
			if _, exists := trackObservations[observationID]; !exists {
				return fmt.Errorf("track %q references an unknown gallery observation", track.ID)
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

var _ service.DensePersonAnalyzer = (*HTTPAnalyzer)(nil)
