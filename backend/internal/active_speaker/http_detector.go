package active_speaker

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
	"os"
	"path/filepath"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

const maxResponseBytes = 8 << 20

type HTTPDetector struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewHTTPDetector(cfg config.ActiveSpeakerConfig) (*HTTPDetector, error) {
	baseURL, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("active-speaker base URL is invalid")
	}
	if cfg.Provider == "external" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("external active-speaker URL must use HTTPS")
	}
	if cfg.Provider == "local" && baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("local active-speaker URL must use HTTP or HTTPS")
	}
	if cfg.Provider == "local" && baseURL.Scheme == "http" && !loopback(baseURL.Hostname()) {
		return nil, fmt.Errorf("unencrypted local active-speaker URL must use a loopback host")
	}
	if strings.TrimSpace(cfg.Model) == "" || cfg.Timeout <= 0 {
		return nil, fmt.Errorf("active-speaker model and timeout are required")
	}
	return &HTTPDetector{
		baseURL: strings.TrimRight(baseURL.String(), "/"), apiKey: strings.TrimSpace(cfg.APIKey),
		model: strings.TrimSpace(cfg.Model), client: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

func (d *HTTPDetector) Provider() string { return "active-speaker-http" }
func (d *HTTPDetector) Model() string    { return d.model }

func (d *HTTPDetector) Validate(ctx context.Context) (service.ProviderMetadata, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/healthz", nil)
	if err != nil {
		return service.ProviderMetadata{}, err
	}
	d.authorize(request)
	response, err := d.client.Do(request)
	if err != nil {
		return service.ProviderMetadata{}, fmt.Errorf("active-speaker health request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return service.ProviderMetadata{}, fmt.Errorf("active-speaker health returned %d", response.StatusCode)
	}
	var decoded struct {
		Status string `json:"status"`
		service.ProviderMetadata
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&decoded); err != nil {
		return service.ProviderMetadata{}, fmt.Errorf("decode active-speaker health: %w", err)
	}
	if decoded.Status != "ok" || decoded.Model != d.model || decoded.Provider == "" {
		return service.ProviderMetadata{}, fmt.Errorf("active-speaker health response is incompatible")
	}
	return decoded.ProviderMetadata, nil
}

func (d *HTTPDetector) DetectActiveSpeakers(ctx context.Context, input service.ActiveSpeakerInput) (service.ActiveSpeakerAnalysis, error) {
	file, err := os.Open(input.VideoPath)
	if err != nil {
		return service.ActiveSpeakerAnalysis{}, fmt.Errorf("open active-speaker video: %w", err)
	}
	defer file.Close()
	metadata, err := json.Marshal(struct {
		RecordingID  string                        `json:"recording_id"`
		PersonTracks []service.TemporalPersonTrack `json:"person_tracks"`
		Segments     []service.TranscriptSegment   `json:"segments"`
	}{input.RecordingID, input.PersonTracks, input.Segments})
	if err != nil {
		return service.ActiveSpeakerAnalysis{}, fmt.Errorf("encode active-speaker metadata: %w", err)
	}

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go func() {
		var uploadErr error
		defer func() {
			if uploadErr == nil {
				uploadErr = multipartWriter.Close()
			}
			_ = writer.CloseWithError(uploadErr)
		}()
		if uploadErr = multipartWriter.WriteField("model", d.model); uploadErr != nil {
			return
		}
		if uploadErr = multipartWriter.WriteField("metadata", string(metadata)); uploadErr != nil {
			return
		}
		name := filepath.Base(strings.TrimSpace(input.FileName))
		if name == "" || name == "." {
			name = "recording.mp4"
		}
		var part io.Writer
		part, uploadErr = multipartWriter.CreateFormFile("file", name)
		if uploadErr == nil {
			_, uploadErr = io.Copy(part, file)
		}
	}()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/v1/active-speakers", reader)
	if err != nil {
		return service.ActiveSpeakerAnalysis{}, err
	}
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	request.Header.Set("Accept", "application/json")
	d.authorize(request)
	response, err := d.client.Do(request)
	if err != nil {
		return service.ActiveSpeakerAnalysis{}, fmt.Errorf("active-speaker request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return service.ActiveSpeakerAnalysis{}, fmt.Errorf("active-speaker service returned %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(payload) > maxResponseBytes {
		return service.ActiveSpeakerAnalysis{}, fmt.Errorf("read active-speaker response")
	}
	var result service.ActiveSpeakerAnalysis
	if err := json.Unmarshal(payload, &result); err != nil {
		return service.ActiveSpeakerAnalysis{}, fmt.Errorf("decode active-speaker response: %w", err)
	}
	if result.Provider == "" || result.Model != d.model {
		return service.ActiveSpeakerAnalysis{}, fmt.Errorf("active-speaker response metadata is incompatible")
	}
	for _, evidence := range result.Evidence {
		if evidence.PersonTrackID == "" || len(evidence.SegmentIDs) == 0 ||
			!unit(evidence.Score) || !unit(evidence.VisibleMouthCoverage) {
			return service.ActiveSpeakerAnalysis{}, fmt.Errorf("active-speaker response contains invalid evidence")
		}
	}
	return result, nil
}

func (d *HTTPDetector) authorize(request *http.Request) {
	if d.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+d.apiKey)
	}
}

func unit(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

var _ service.ActiveSpeakerDetector = (*HTTPDetector)(nil)
