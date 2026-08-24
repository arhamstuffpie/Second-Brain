package vision

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

func TestOpenAIAnalyzerSendsFramesAndParsesStructuredOutput(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/responses" {
			t.Errorf("path = %q, want /v1/responses", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		var payload struct {
			Model string `json:"model"`
			Input []struct {
				Content []struct {
					Type     string `json:"type"`
					ImageURL string `json:"image_url"`
					Detail   string `json:"detail"`
				} `json:"content"`
			} `json:"input"`
			Text struct {
				Format struct {
					Type   string `json:"type"`
					Strict bool   `json:"strict"`
				} `json:"format"`
			} `json:"text"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.Model != "vision-model" || len(payload.Input) != 1 ||
			len(payload.Input[0].Content) != 3 {
			t.Errorf("payload = %+v", payload)
		}
		if payload.Input[0].Content[1].Type != "input_image" ||
			payload.Input[0].Content[1].Detail != "low" ||
			payload.Input[0].Content[1].ImageURL == "" {
			t.Errorf("first image content = %+v", payload.Input[0].Content[1])
		}
		if payload.Text.Format.Type != "json_schema" || !payload.Text.Format.Strict {
			t.Errorf("structured output format = %+v", payload.Text.Format)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
		  "output": [{
		    "content": [{
		      "type": "output_text",
		      "text": "{\"observations\":[{\"start_time\":99,\"end_time\":100,\"objects\":[{\"name\":\"laptop\",\"confidence\":0.9}],\"text_detected\":[{\"text\":\"Plan\",\"confidence\":0.8}],\"activity\":\"planning\",\"location_guess\":\"office\",\"summary\":\"A planning document is open.\",\"confidence\":0.85},{\"start_time\":100,\"end_time\":101,\"objects\":[],\"text_detected\":[],\"activity\":\"planning\",\"location_guess\":\"office\",\"summary\":\"The planning view continues.\",\"confidence\":0.8}]}"
		    }]
		  }]
		}`)),
		}, nil
	})

	analyzer, err := NewOpenAI(config.VisionConfig{
		BaseURL: "https://api.openai.test/v1", APIKey: "test-key", Model: "vision-model",
		Detail: "low", Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewOpenAI() error = %v", err)
	}
	analyzer.client.Transport = transport
	result, err := analyzer.Analyze(context.Background(), service.VisualAnalysisInput{
		Frames: []service.VideoFrame{
			{Timestamp: 0, MediaType: "image/jpeg", Image: []byte("one")},
			{Timestamp: 5, MediaType: "image/jpeg", Image: []byte("two")},
		},
		WindowDuration: 5,
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %+v", result.Observations)
	}
	observation := result.Observations[0]
	if observation.StartTime != 0 || observation.EndTime != 5 ||
		observation.Objects[0].Name != "laptop" {
		t.Fatalf("observation = %+v", observation)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestNormalizeObservationsClampsConfidenceAndTimestamps(t *testing.T) {
	confidence := 2.0
	analysis := service.VisualAnalysis{Observations: []service.VideoObservation{{
		StartTime: 100, EndTime: 100, Confidence: &confidence,
	}}}
	normalizeObservations(&analysis, service.VisualAnalysisInput{
		Frames: []service.VideoFrame{{Timestamp: 10}}, WindowDuration: 4,
	})
	if analysis.Observations[0].StartTime != 10 ||
		analysis.Observations[0].EndTime != 14 ||
		*analysis.Observations[0].Confidence != 1 {
		t.Fatalf("normalized observation = %+v", analysis.Observations[0])
	}
}
