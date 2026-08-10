package speaker

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

const maxEmbeddingResponseBytes = 1 << 20

type HTTPEmbedder struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

func NewHTTPEmbedder(cfg config.SpeakerEmbeddingConfig) (*HTTPEmbedder, error) {
	baseURL, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("speaker embedding base URL is invalid")
	}
	if cfg.Provider == "external" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("external speaker embedding base URL must use HTTPS")
	}
	if cfg.Provider == "local" && baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("local speaker embedding base URL must use HTTP or HTTPS")
	}
	if cfg.Provider == "local" && baseURL.Scheme == "http" && !isLoopbackHost(baseURL.Hostname()) {
		return nil, fmt.Errorf("unencrypted local speaker embedding URL must use a loopback host")
	}
	if strings.TrimSpace(cfg.Model) == "" || cfg.Timeout <= 0 {
		return nil, fmt.Errorf("speaker embedding model and timeout are required")
	}
	return &HTTPEmbedder{
		endpoint: strings.TrimRight(baseURL.String(), "/") + "/v1/embeddings",
		apiKey:   strings.TrimSpace(cfg.APIKey),
		model:    strings.TrimSpace(cfg.Model),
		client:   &http.Client{Timeout: cfg.Timeout},
	}, nil
}

func (e *HTTPEmbedder) Embed(ctx context.Context, input service.SpeakerEmbeddingInput) (service.SpeakerEmbedding, error) {
	if len(input.Audio) == 0 {
		return service.SpeakerEmbedding{}, fmt.Errorf("speaker embedding audio is empty")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileName := filepath.Base(strings.TrimSpace(input.FileName))
	if fileName == "" || fileName == "." {
		fileName = "speaker.wav"
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return service.SpeakerEmbedding{}, fmt.Errorf("create speaker embedding upload: %w", err)
	}
	if _, err := part.Write(input.Audio); err != nil {
		return service.SpeakerEmbedding{}, fmt.Errorf("write speaker embedding upload: %w", err)
	}
	if err := writer.WriteField("model", e.model); err != nil {
		return service.SpeakerEmbedding{}, fmt.Errorf("write speaker embedding model: %w", err)
	}
	if err := writer.Close(); err != nil {
		return service.SpeakerEmbedding{}, fmt.Errorf("finalize speaker embedding upload: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, &body)
	if err != nil {
		return service.SpeakerEmbedding{}, fmt.Errorf("create speaker embedding request: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Accept", "application/json")
	if e.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+e.apiKey)
	}
	response, err := e.client.Do(request)
	if err != nil {
		return service.SpeakerEmbedding{}, fmt.Errorf("speaker embedding request: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxEmbeddingResponseBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return service.SpeakerEmbedding{}, fmt.Errorf("read speaker embedding response: %w", err)
	}
	if len(payload) > maxEmbeddingResponseBytes {
		return service.SpeakerEmbedding{}, fmt.Errorf("speaker embedding response exceeds 1 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return service.SpeakerEmbedding{}, fmt.Errorf("speaker embedding service returned %s: %s", response.Status, boundedMessage(payload))
	}
	var decoded struct {
		Embedding  []float64 `json:"embedding"`
		Model      string    `json:"model"`
		Dimensions int       `json:"dimensions"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return service.SpeakerEmbedding{}, fmt.Errorf("decode speaker embedding response: %w", err)
	}
	if strings.TrimSpace(decoded.Model) != e.model {
		return service.SpeakerEmbedding{}, fmt.Errorf("speaker embedding model mismatch: requested %q, received %q", e.model, decoded.Model)
	}
	if len(decoded.Embedding) == 0 || (decoded.Dimensions != 0 && decoded.Dimensions != len(decoded.Embedding)) {
		return service.SpeakerEmbedding{}, fmt.Errorf("speaker embedding response has invalid dimensions")
	}
	var norm float64
	for _, value := range decoded.Embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return service.SpeakerEmbedding{}, fmt.Errorf("speaker embedding response contains a non-finite value")
		}
		norm += value * value
	}
	if norm <= 0 {
		return service.SpeakerEmbedding{}, fmt.Errorf("speaker embedding response has zero magnitude")
	}
	return service.SpeakerEmbedding{Vector: decoded.Embedding, Model: decoded.Model}, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func boundedMessage(payload []byte) string {
	message := strings.TrimSpace(string(payload))
	if len(message) > 300 {
		message = message[:300]
	}
	if message == "" {
		return "empty response"
	}
	return message
}

var _ service.SpeakerEmbedder = (*HTTPEmbedder)(nil)
