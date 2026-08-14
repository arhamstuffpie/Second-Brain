package face

import (
	"bytes"
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

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

const maxResponseBytes = 8 << 20

type ProviderError struct {
	Cause     error
	Retryable bool
}

func (e *ProviderError) Error() string { return e.Cause.Error() }
func (e *ProviderError) Unwrap() error { return e.Cause }

type HTTPRecognizer struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewHTTPRecognizer(cfg config.FaceRecognitionConfig) (*HTTPRecognizer, error) {
	baseURL, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("face recognition base URL is invalid")
	}
	if cfg.Provider == "external" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("external face recognition base URL must use HTTPS")
	}
	if cfg.Provider == "local" && baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("local face recognition base URL must use HTTP or HTTPS")
	}
	if cfg.Provider == "local" && baseURL.Scheme == "http" && !loopback(baseURL.Hostname()) {
		return nil, fmt.Errorf("unencrypted local face recognition URL must use a loopback host")
	}
	if strings.TrimSpace(cfg.Model) == "" || cfg.Timeout <= 0 {
		return nil, fmt.Errorf("face recognition model and timeout are required")
	}
	return &HTTPRecognizer{
		baseURL: strings.TrimRight(baseURL.String(), "/"), apiKey: strings.TrimSpace(cfg.APIKey),
		model: strings.TrimSpace(cfg.Model), client: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

func (r *HTTPRecognizer) Provider() string { return "opencv" }
func (r *HTTPRecognizer) Model() string    { return r.model }

func (r *HTTPRecognizer) Validate(ctx context.Context) (service.FaceProviderMetadata, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/healthz", nil)
	if err != nil {
		return service.FaceProviderMetadata{}, permanent(err)
	}
	request.Header.Set("Accept", "application/json")
	if r.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+r.apiKey)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return service.FaceProviderMetadata{}, transient(fmt.Errorf("face recognition health request: %w", err))
	}
	defer response.Body.Close()
	payload, err := readBounded(response.Body)
	if err != nil {
		return service.FaceProviderMetadata{}, permanent(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return service.FaceProviderMetadata{}, statusError(response.StatusCode, payload)
	}
	var decoded struct {
		Status string `json:"status"`
		service.FaceProviderMetadata
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return service.FaceProviderMetadata{}, permanent(fmt.Errorf("decode face recognition health response: %w", err))
	}
	if decoded.Status != "ok" || decoded.Model != r.model || decoded.Dimensions < 1 || decoded.Provider == "" || decoded.Detector == "" {
		return service.FaceProviderMetadata{}, permanent(fmt.Errorf("face recognition health response is incompatible with configured model"))
	}
	return decoded.FaceProviderMetadata, nil
}

func (r *HTTPRecognizer) Recognize(ctx context.Context, input service.FaceRecognitionInput) (service.FaceRecognition, error) {
	if len(input.Image) == 0 {
		return service.FaceRecognition{}, permanent(fmt.Errorf("face recognition image is empty"))
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileName := filepath.Base(strings.TrimSpace(input.FileName))
	if fileName == "" || fileName == "." {
		fileName = "face.jpg"
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err == nil {
		_, err = part.Write(input.Image)
	}
	if err == nil {
		err = writer.WriteField("model", r.model)
	}
	if err == nil {
		err = writer.WriteField("single_face", fmt.Sprint(input.SingleFace))
	}
	if err == nil {
		err = writer.Close()
	}
	if err != nil {
		return service.FaceRecognition{}, permanent(fmt.Errorf("create face recognition upload: %w", err))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/embeddings", &body)
	if err != nil {
		return service.FaceRecognition{}, permanent(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Accept", "application/json")
	if r.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+r.apiKey)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return service.FaceRecognition{}, transient(fmt.Errorf("face recognition request: %w", err))
	}
	defer response.Body.Close()
	payload, err := readBounded(response.Body)
	if err != nil {
		return service.FaceRecognition{}, permanent(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return service.FaceRecognition{}, statusError(response.StatusCode, payload)
	}
	var decoded struct {
		Provider   string                 `json:"provider"`
		Detector   string                 `json:"detector"`
		Model      string                 `json:"model"`
		Dimensions int                    `json:"dimensions"`
		Faces      []service.DetectedFace `json:"faces"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return service.FaceRecognition{}, permanent(fmt.Errorf("decode face recognition response: %w", err))
	}
	if decoded.Model != r.model || decoded.Provider == "" || decoded.Detector == "" || decoded.Dimensions < 1 {
		return service.FaceRecognition{}, permanent(fmt.Errorf("face recognition response metadata is incompatible"))
	}
	for _, detected := range decoded.Faces {
		if detected.Box.Width <= 0 || detected.Box.Height <= 0 || detected.DetectionScore < 0 || detected.DetectionScore > 1 || len(detected.Landmarks) != 5 {
			return service.FaceRecognition{}, permanent(fmt.Errorf("face recognition response contains invalid detection data"))
		}
		if detected.Quality.Usable {
			if len(detected.Embedding) != decoded.Dimensions {
				return service.FaceRecognition{}, permanent(fmt.Errorf("face recognition response has invalid embedding dimensions"))
			}
			norm := 0.0
			for _, value := range detected.Embedding {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return service.FaceRecognition{}, permanent(fmt.Errorf("face recognition response contains a non-finite embedding"))
				}
				norm += value * value
			}
			if math.Abs(math.Sqrt(norm)-1) > 0.01 {
				return service.FaceRecognition{}, permanent(fmt.Errorf("face recognition embedding is not normalized"))
			}
		} else if len(detected.Embedding) != 0 {
			return service.FaceRecognition{}, permanent(fmt.Errorf("unusable face must not include an embedding"))
		}
	}
	return service.FaceRecognition{
		Provider: decoded.Provider, Detector: decoded.Detector, Model: decoded.Model,
		Dimensions: decoded.Dimensions, Faces: decoded.Faces,
	}, nil
}

func readBounded(body io.Reader) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read face recognition response: %w", err)
	}
	if len(payload) > maxResponseBytes {
		return nil, fmt.Errorf("face recognition response exceeds 8 MiB")
	}
	return payload, nil
}

func statusError(code int, payload []byte) error {
	message := strings.TrimSpace(string(payload))
	if len(message) > 300 {
		message = message[:300]
	}
	return &ProviderError{
		Cause:     fmt.Errorf("face recognition service returned %d: %s", code, message),
		Retryable: code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500,
	}
}

func transient(err error) error { return &ProviderError{Cause: err, Retryable: true} }
func permanent(err error) error { return &ProviderError{Cause: err, Retryable: false} }

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

var _ service.FaceRecognizer = (*HTTPRecognizer)(nil)
