package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

type OpenAIAnalyzer struct {
	baseURL string
	apiKey  string
	model   string
	detail  string
	client  *http.Client
}

func NewOpenAI(cfg config.VisionConfig) (*OpenAIAnalyzer, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" ||
		strings.TrimSpace(cfg.Model) == "" || cfg.Timeout <= 0 {
		return nil, fmt.Errorf("OpenAI vision base URL, API key, model, and timeout are required")
	}
	return &OpenAIAnalyzer{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		detail:  cfg.Detail,
		client:  &http.Client{Timeout: cfg.Timeout},
	}, nil
}

func (a *OpenAIAnalyzer) Provider() string { return "openai" }
func (a *OpenAIAnalyzer) Model() string    { return a.model }

func (a *OpenAIAnalyzer) Analyze(
	ctx context.Context,
	input service.VisualAnalysisInput,
) (service.VisualAnalysis, error) {
	if len(input.Frames) == 0 {
		return service.VisualAnalysis{}, fmt.Errorf("at least one video frame is required")
	}
	content := make([]map[string]any, 0, len(input.Frames)+1)
	timestamps := make([]string, 0, len(input.Frames))
	for _, frame := range input.Frames {
		timestamps = append(timestamps, strconv.FormatFloat(frame.Timestamp, 'f', 2, 64)+"s")
	}
	content = append(content, map[string]any{
		"type": "input_text",
		"text": "Analyze the following ordered video frames at timestamps " +
			strings.Join(timestamps, ", ") + `. Return one observation per frame in the same order. ` +
			`Detect important visible objects, transcribe readable text exactly, identify the current ` +
			`activity/scenario, make a conservative location guess, and summarize only what is visually ` +
			`grounded. Use each supplied timestamp as start_time and the next timestamp as end_time; ` +
			`for the last frame use the configured window duration. Confidence values must be between 0 and 1.`,
	})
	for _, frame := range input.Frames {
		mediaType := strings.TrimSpace(frame.MediaType)
		if mediaType == "" {
			mediaType = "image/jpeg"
		}
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(frame.Image),
			"detail":    a.detail,
		})
	}

	payload := map[string]any{
		"model": a.model,
		"input": []map[string]any{{
			"role":    "user",
			"content": content,
		}},
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "video_visual_analysis",
				"strict": true,
				"schema": visualAnalysisSchema(),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("encode vision request: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, a.baseURL+"/responses", bytes.NewReader(body),
	)
	if err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("create vision request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := a.client.Do(request)
	if err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("call vision API: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("read vision response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return service.VisualAnalysis{}, fmt.Errorf(
			"vision API returned %d: %s", response.StatusCode, boundedMessage(responseBody),
		)
	}
	var result struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("decode vision response: %w", err)
	}
	var outputText string
	for _, output := range result.Output {
		for _, item := range output.Content {
			if item.Type == "output_text" && strings.TrimSpace(item.Text) != "" {
				outputText = item.Text
				break
			}
		}
	}
	if outputText == "" {
		return service.VisualAnalysis{}, fmt.Errorf("vision response contained no structured output")
	}
	var analysis service.VisualAnalysis
	if err := json.Unmarshal([]byte(outputText), &analysis); err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("decode structured visual analysis: %w", err)
	}
	if len(analysis.Observations) == 0 {
		return service.VisualAnalysis{}, fmt.Errorf("vision analysis returned no observations")
	}
	normalizeObservations(&analysis, input)
	return analysis, nil
}

func normalizeObservations(
	analysis *service.VisualAnalysis,
	input service.VisualAnalysisInput,
) {
	for index := range analysis.Observations {
		observation := &analysis.Observations[index]
		if index < len(input.Frames) {
			observation.StartTime = input.Frames[index].Timestamp
		}
		if index+1 < len(input.Frames) {
			observation.EndTime = input.Frames[index+1].Timestamp
		} else {
			window := input.WindowDuration
			if window <= 0 {
				window = 5
			}
			observation.EndTime = observation.StartTime + window
		}
		if observation.Confidence != nil {
			if *observation.Confidence < 0 {
				*observation.Confidence = 0
			}
			if *observation.Confidence > 1 {
				*observation.Confidence = 1
			}
		}
	}
}

func visualAnalysisSchema() map[string]any {
	confidence := map[string]any{"type": []string{"number", "null"}, "minimum": 0, "maximum": 1}
	objectSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"name":       map[string]any{"type": "string"},
			"confidence": confidence,
		},
		"required": []string{"name", "confidence"},
	}
	textSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"text":       map[string]any{"type": "string"},
			"confidence": confidence,
		},
		"required": []string{"text", "confidence"},
	}
	observationSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"start_time":     map[string]any{"type": "number"},
			"end_time":       map[string]any{"type": "number"},
			"objects":        map[string]any{"type": "array", "items": objectSchema},
			"text_detected":  map[string]any{"type": "array", "items": textSchema},
			"activity":       map[string]any{"type": "string"},
			"location_guess": map[string]any{"type": "string"},
			"summary":        map[string]any{"type": "string"},
			"confidence":     confidence,
		},
		"required": []string{
			"start_time", "end_time", "objects", "text_detected", "activity",
			"location_guess", "summary", "confidence",
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"observations": map[string]any{
				"type": "array", "items": observationSchema,
			},
		},
		"required": []string{"observations"},
	}
}

func boundedMessage(body []byte) string {
	message := strings.TrimSpace(string(body))
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}

var _ service.VisualAnalyzer = (*OpenAIAnalyzer)(nil)
