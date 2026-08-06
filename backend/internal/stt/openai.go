package stt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

type OpenAITranscriber struct {
	provider string
	baseURL  string
	apiKey   string
	model    string
	language string
	prompt   string
	client   *http.Client
}

func NewOpenAI(cfg config.STTConfig) (*OpenAITranscriber, error) {
	return NewCompatible("openai", cfg)
}

// NewCompatible creates a transcriber for APIs that implement OpenAI's
// /audio/transcriptions multipart contract. provider is retained as an
// account-facing label and persisted with each transcript.
func NewCompatible(provider string, cfg config.STTConfig) (*OpenAITranscriber, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("compatible STT base URL, API key, and model are required")
	}
	if provider == "" {
		return nil, fmt.Errorf("compatible STT provider is required")
	}
	return &OpenAITranscriber{
		provider: provider, baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey: cfg.APIKey, model: cfg.Model,
		language: cfg.Language, prompt: cfg.Prompt,
		client: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

func (t *OpenAITranscriber) Provider() string { return t.provider }
func (t *OpenAITranscriber) Model() string    { return t.model }

func (t *OpenAITranscriber) Transcribe(ctx context.Context, input service.TranscriptionInput) (service.Transcript, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(filepath.Base(input.FileName))))
	if input.MediaType != "" {
		header.Set("Content-Type", input.MediaType)
	}
	filePart, err := writer.CreatePart(header)
	if err != nil {
		return service.Transcript{}, fmt.Errorf("create transcription multipart file: %w", err)
	}
	if _, err := io.Copy(filePart, input.Audio); err != nil {
		return service.Transcript{}, fmt.Errorf("copy transcription audio: %w", err)
	}
	if err := writer.WriteField("model", t.model); err != nil {
		return service.Transcript{}, err
	}
	if strings.Contains(strings.ToLower(t.model), "diarize") {
		_ = writer.WriteField("response_format", "diarized_json")
		_ = writer.WriteField("chunking_strategy", "auto")
		for _, reference := range input.KnownSpeakers {
			if strings.TrimSpace(reference.ProviderLabel) == "" || len(reference.Audio) == 0 {
				continue
			}
			mediaType := strings.TrimSpace(reference.MediaType)
			if mediaType == "" {
				mediaType = "audio/wav"
			}
			_ = writer.WriteField("known_speaker_names[]", reference.ProviderLabel)
			_ = writer.WriteField(
				"known_speaker_references[]",
				"data:"+mediaType+";base64,"+base64.StdEncoding.EncodeToString(reference.Audio),
			)
		}
	} else {
		if t.model == "whisper-1" {
			_ = writer.WriteField("response_format", "verbose_json")
			_ = writer.WriteField("timestamp_granularities[]", "segment")
		} else {
			_ = writer.WriteField("response_format", "json")
		}
		if t.prompt != "" {
			_ = writer.WriteField("prompt", t.prompt)
		}
	}
	if t.language != "" {
		_ = writer.WriteField("language", t.language)
	}
	if err := writer.Close(); err != nil {
		return service.Transcript{}, fmt.Errorf("close transcription multipart body: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return service.Transcript{}, fmt.Errorf("create transcription request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+t.apiKey)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	response, err := t.client.Do(request)
	if err != nil {
		return service.Transcript{}, fmt.Errorf("call transcription API: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return service.Transcript{}, fmt.Errorf("read transcription response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return service.Transcript{}, fmt.Errorf("transcription API returned %d: %s", response.StatusCode, boundedMessage(responseBody))
	}

	var payload struct {
		Text     string  `json:"text"`
		Language string  `json:"language"`
		Duration float64 `json:"duration"`
		Segments []struct {
			ID         string   `json:"id"`
			Start      float64  `json:"start"`
			End        float64  `json:"end"`
			Speaker    string   `json:"speaker"`
			Text       string   `json:"text"`
			AvgLogProb *float64 `json:"avg_logprob"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return service.Transcript{}, fmt.Errorf("decode transcription response: %w", err)
	}
	segments := make([]service.TranscriptSegment, 0, len(payload.Segments))
	for _, segment := range payload.Segments {
		speaker := strings.TrimSpace(segment.Speaker)
		if speaker == "" {
			speaker = "speaker"
		}
		var confidence *float64
		if segment.AvgLogProb != nil {
			value := probabilityFromLog(*segment.AvgLogProb)
			confidence = &value
		}
		segments = append(segments, service.TranscriptSegment{
			ID: segment.ID, StartTime: segment.Start, EndTime: segment.End, Speaker: speaker,
			Text: strings.TrimSpace(segment.Text), Confidence: confidence,
		})
	}
	if len(segments) == 0 && strings.TrimSpace(payload.Text) != "" {
		segments = []service.TranscriptSegment{{
			StartTime: 0, EndTime: payload.Duration, Speaker: "speaker", Text: strings.TrimSpace(payload.Text),
		}}
	}
	return service.Transcript{
		Text: strings.TrimSpace(payload.Text), Language: payload.Language,
		Duration: payload.Duration, Segments: segments, Provider: t.provider, Model: t.model,
	}, nil
}

func escapeQuotes(value string) string {
	return strings.NewReplacer(`"`, "", "\r", "", "\n", "").Replace(value)
}

func boundedMessage(body []byte) string {
	message := strings.TrimSpace(string(body))
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}

func probabilityFromLog(value float64) float64 {
	confidence := math.Exp(value)
	if confidence < 0 {
		return 0
	}
	if confidence > 1 {
		return 1
	}
	return confidence
}

var _ service.Transcriber = (*OpenAITranscriber)(nil)
