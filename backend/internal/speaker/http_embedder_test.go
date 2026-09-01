package speaker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

func TestHTTPEmbedderSendsAuthenticatedMultipartRequest(t *testing.T) {
	const model = "speechbrain/spkrec-ecapa-voxceleb"
	embedder := &HTTPEmbedder{
		endpoint: "http://127.0.0.1:8091/v1/embeddings", apiKey: "secret", model: model,
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost || request.URL.Path != "/v1/embeddings" {
				t.Fatalf("request = %s %s", request.Method, request.URL.Path)
			}
			if got := request.Header.Get("Authorization"); got != "Bearer secret" {
				t.Fatalf("Authorization = %q", got)
			}
			if err := request.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm() error = %v", err)
			}
			if got := request.FormValue("model"); got != model {
				t.Fatalf("model = %q", got)
			}
			file, header, err := request.FormFile("file")
			if err != nil {
				t.Fatalf("FormFile() error = %v", err)
			}
			_ = file.Close()
			if header.Filename != "speaker.wav" {
				t.Fatalf("filename = %q", header.Filename)
			}
			payload := fmt.Sprintf(`{"embedding":[0.6,0.8],"model":%q,"dimensions":2}`, model)
			return jsonResponse(http.StatusOK, payload), nil
		})},
	}
	result, err := embedder.Embed(context.Background(), service.SpeakerEmbeddingInput{
		FileName: "speaker.wav", MediaType: "audio/wav", Audio: []byte("audio"),
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if result.Model != model || len(result.Vector) != 2 {
		t.Fatalf("Embed() = %#v", result)
	}
}

func TestHTTPEmbedderRejectsModelMismatch(t *testing.T) {
	embedder := &HTTPEmbedder{
		endpoint: "http://127.0.0.1:8091/v1/embeddings", model: "expected",
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"embedding":[1],"model":"different","dimensions":1}`,
			), nil
		})},
	}
	_, err := embedder.Embed(context.Background(), service.SpeakerEmbeddingInput{
		FileName: "speaker.wav", Audio: []byte("audio"),
	})
	if err == nil || !strings.Contains(err.Error(), "model mismatch") {
		t.Fatalf("Embed() error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func jsonResponse(statusCode int, payload string) *http.Response {
	return &http.Response{
		StatusCode: statusCode, Status: fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewBufferString(payload)),
	}
}

func TestExternalHTTPEmbedderRequiresHTTPS(t *testing.T) {
	_, err := NewHTTPEmbedder(config.SpeakerEmbeddingConfig{
		Provider: "external", BaseURL: "http://embedder.example.com",
		Model: "model", Timeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("NewHTTPEmbedder() error = %v", err)
	}
}
