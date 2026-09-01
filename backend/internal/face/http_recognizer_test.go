package face

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

const testModel = "opencv/sface-test"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func jsonResponse(status int, value any) *http.Response {
	var payload strings.Builder
	_ = json.NewEncoder(&payload).Encode(value)
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(payload.String())), Header: make(http.Header)}
}

func TestHTTPRecognizer(t *testing.T) {
	recognizer, err := NewHTTPRecognizer(config.FaceRecognitionConfig{
		Provider: "local", BaseURL: "http://127.0.0.1:8092", APIKey: "secret", Model: testModel, Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	recognizer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/healthz" {
			return jsonResponse(http.StatusOK, map[string]any{
				"status": "ok", "provider": "opencv", "detector": "yunet", "model": testModel,
				"dimensions": 2, "detector_sha256": "detector", "model_sha256": "model",
			}), nil
		}
		if request.Header.Get("Authorization") != "Bearer secret" {
			return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "unauthorized"}), nil
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if request.FormValue("model") != testModel || request.FormValue("single_face") != "true" {
			t.Fatalf("unexpected form: %#v", request.MultipartForm.Value)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"provider": "opencv", "detector": "yunet", "model": testModel, "dimensions": 2,
			"faces": []any{map[string]any{
				"box":             map[string]any{"x": 1, "y": 2, "width": 10, "height": 10},
				"landmarks":       [][]float64{{1, 1}, {2, 1}, {1.5, 2}, {1, 3}, {2, 3}},
				"detection_score": .99, "quality": map[string]any{"usable": true, "reasons": []string{}, "score": .9},
				"pose":      map[string]any{"yaw": 0, "pitch": 0, "roll": 0, "bucket": "frontal"},
				"embedding": []float64{.6, .8},
			}},
		}), nil
	})
	metadata, err := recognizer.Validate(context.Background())
	if err != nil || metadata.Dimensions != 2 {
		t.Fatalf("metadata = %#v, error = %v", metadata, err)
	}
	result, err := recognizer.Recognize(context.Background(), service.FaceRecognitionInput{
		FileName: "face.jpg", Image: []byte("image"), SingleFace: true,
	})
	if err != nil || len(result.Faces) != 1 || len(result.Faces[0].Embedding) != 2 {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}

func TestHTTPRecognizerClassifiesProviderErrors(t *testing.T) {
	recognizer, err := NewHTTPRecognizer(config.FaceRecognitionConfig{
		Provider: "local", BaseURL: "http://127.0.0.1:8092", Model: testModel, Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	recognizer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusServiceUnavailable, map[string]string{"error": "busy"}), nil
	})
	_, err = recognizer.Recognize(context.Background(), service.FaceRecognitionInput{Image: []byte("image")})
	var providerError *ProviderError
	if !errors.As(err, &providerError) || !providerError.Retryable {
		t.Fatalf("error = %#v, want retryable provider error", err)
	}
}
