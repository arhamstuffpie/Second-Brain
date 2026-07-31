package memograph

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
	"github.com/arham/ai-second-brain/internal/utils"
)

func TestInsertEpisodeUsesAPIKeyAndMergesCustomFields(t *testing.T) {
	client := testClient(t, "mg_live_test", "", func(request *http.Request) string {
		if request.URL.Path != "/api/v1/memory/memory-1/create" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if got := request.Header.Get("X-Api-Key"); got != "mg_live_test" {
			t.Fatalf("X-Api-Key = %q", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["confidence"] != 0.9 {
			t.Fatalf("confidence = %#v", payload["confidence"])
		}
		meta := payload["meta"].(map[string]any)
		if meta["group_id"] != "session-1" {
			t.Fatalf("meta.group_id = %#v", meta["group_id"])
		}
		return `{"data":{"episode":{"uuid":"episode-1"}}}`
	})
	_, err := client.InsertEpisode(context.Background(), "memory-1", service.EpisodeInsertRequest{
		Data:         "A said hello.",
		Meta:         map[string]any{"group_id": "session-1"},
		CustomFields: map[string]any{"confidence": 0.9},
	})
	if err != nil {
		t.Fatalf("InsertEpisode() error = %v", err)
	}
}

func TestCreateMemoryPrefersAPIKey(t *testing.T) {
	client := testClient(t, "mg_live_test", "jwt-token", func(request *http.Request) string {
		if got := request.Header.Get("X-Api-Key"); got != "mg_live_test" {
			t.Fatalf("X-Api-Key = %q", got)
		}
		if got := request.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
		return `{"data":{"id":"memory-1"}}`
	})
	_, err := client.CreateMemory(context.Background(), "project-1", service.MemoryCreateRequest{
		Name: "Voice memory", MemoryType: "graph",
		GraphConfig: service.GraphConfig{Mode: "instruction", Instruction: "Extract people."},
	})
	if err != nil {
		t.Fatalf("CreateMemory() error = %v", err)
	}
}

func TestAnswerScopesGroupThroughFilter(t *testing.T) {
	client := testClient(t, "mg_live_test", "", func(request *http.Request) string {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		filters := payload["filters"].(map[string]any)
		group := filters["group_id"].(map[string]any)
		if group["eq"] != "session-1" {
			t.Fatalf("group filter = %#v", group)
		}
		return `{"data":{"answer":"hello"}}`
	})
	_, err := client.Answer(context.Background(), "memory-1", service.MemoryAnswerRequest{
		Query: "What happened?", GroupID: "session-1",
	})
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
}

func TestAnswerStreamUsesAPIKeyAndForcesStreaming(t *testing.T) {
	client := NewClient(config.MemographConfig{
		BaseURL: "https://memograph.test", APIKey: "mg_live_test", Timeout: time.Second,
	})
	client.http = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/v1/memory/memory-1/answer" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if got := request.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("Accept = %q", got)
		}
		if got := request.Header.Get("X-Api-Key"); got != "mg_live_test" {
			t.Fatalf("X-Api-Key = %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["stream"] != true {
			t.Fatalf("stream = %#v, want true", payload["stream"])
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"event: token\ndata: {\"content\":\"hello\"}\n\n",
			)),
			Request: request,
		}, nil
	})}

	stream, err := client.AnswerStream(
		context.Background(),
		"memory-1",
		service.MemoryAnswerRequest{Query: "What happened?"},
	)
	if err != nil {
		t.Fatalf("AnswerStream() error = %v", err)
	}
	defer stream.Body.Close()
	body, err := io.ReadAll(stream.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if !strings.Contains(string(body), `"content":"hello"`) {
		t.Fatalf("stream body = %q", body)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func testClient(t *testing.T, apiKey, jwt string, handler func(*http.Request) string) *Client {
	t.Helper()
	client := NewClient(config.MemographConfig{
		BaseURL: "https://memograph.test", APIKey: apiKey, JWT: jwt, Timeout: time.Second,
	})
	client.http = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := handler(request)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	return client
}

func TestGetGraphUsesAPIKey(t *testing.T) {
	client := testClient(t, "mg_live_test", "", func(request *http.Request) string {
		if got := request.Header.Get("X-Api-Key"); got != "mg_live_test" {
			t.Fatalf("X-Api-Key = %q", got)
		}
		return `{"data":{"nodes":[],"edges":[]}}`
	})
	if _, err := client.GetGraph(context.Background(), "memory-1", "session-1"); err != nil {
		t.Fatalf("GetGraph() error = %v", err)
	}
}

func TestMemographFallsBackToJWTWhenAPIKeyIsMissing(t *testing.T) {
	client := testClient(t, "", "jwt-token", func(request *http.Request) string {
		if got := request.Header.Get("Authorization"); got != "Bearer jwt-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := request.Header.Get("X-Api-Key"); got != "" {
			t.Fatalf("X-Api-Key = %q, want empty", got)
		}
		return `{"data":[]}`
	})
	if _, err := client.Search(context.Background(), "memory-1", service.MemorySearchRequest{
		Query: "hello",
	}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestRequestAPIKeyOverridesConfiguredCredentials(t *testing.T) {
	client := testClient(t, "configured-key", "jwt-token", func(request *http.Request) string {
		if got := request.Header.Get("X-Api-Key"); got != "device-key" {
			t.Fatalf("X-Api-Key = %q", got)
		}
		if got := request.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
		return `{"data":[]}`
	})
	ctx := utils.WithMemographAPIKey(context.Background(), "device-key")
	if _, err := client.Search(ctx, "memory-1", service.MemorySearchRequest{Query: "hello"}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestEveryMemographOperationUsesAPIKey(t *testing.T) {
	expected := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/memory/project/project-1/create-full"},
		{http.MethodPost, "/api/v1/memory/memory-1/create"},
		{http.MethodPost, "/api/v1/memory/memory-1/search"},
		{http.MethodPost, "/api/v1/memory/memory-1/answer"},
		{http.MethodGet, "/api/v1/memory/memory-1/graph"},
	}
	requestIndex := 0
	client := testClient(t, "mg_live_test", "jwt-token", func(request *http.Request) string {
		if requestIndex >= len(expected) {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		want := expected[requestIndex]
		requestIndex++
		if request.Method != want.method || request.URL.Path != want.path {
			t.Fatalf("request = %s %s, want %s %s", request.Method, request.URL.Path, want.method, want.path)
		}
		if got := request.Header.Get("X-Api-Key"); got != "mg_live_test" {
			t.Fatalf("%s %s X-Api-Key = %q", request.Method, request.URL.Path, got)
		}
		if got := request.Header.Get("Authorization"); got != "" {
			t.Fatalf("%s %s Authorization = %q, want empty", request.Method, request.URL.Path, got)
		}
		return `{"data":{}}`
	})
	ctx := context.Background()
	if _, err := client.CreateMemory(ctx, "project-1", service.MemoryCreateRequest{}); err != nil {
		t.Fatalf("CreateMemory() error = %v", err)
	}
	if _, err := client.InsertEpisode(ctx, "memory-1", service.EpisodeInsertRequest{Data: "hello"}); err != nil {
		t.Fatalf("InsertEpisode() error = %v", err)
	}
	if _, err := client.Search(ctx, "memory-1", service.MemorySearchRequest{Query: "hello"}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if _, err := client.Answer(ctx, "memory-1", service.MemoryAnswerRequest{Query: "hello"}); err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if _, err := client.GetGraph(ctx, "memory-1", ""); err != nil {
		t.Fatalf("GetGraph() error = %v", err)
	}
	if requestIndex != len(expected) {
		t.Fatalf("requests = %d, want %d", requestIndex, len(expected))
	}
}
