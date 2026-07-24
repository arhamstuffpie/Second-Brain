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

func TestCreateMemoryUsesJWT(t *testing.T) {
	client := testClient(t, "mg_live_test", "jwt-token", func(request *http.Request) string {
		if got := request.Header.Get("Authorization"); got != "Bearer jwt-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := request.Header.Get("X-Api-Key"); got != "" {
			t.Fatalf("X-Api-Key = %q, want empty", got)
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

func TestGetGraphRequiresJWT(t *testing.T) {
	client := NewClient(config.MemographConfig{
		BaseURL: "https://example.invalid", APIKey: "mg_live_test", Timeout: time.Second,
	})
	if _, err := client.GetGraph(context.Background(), "memory-1", "session-1"); err == nil {
		t.Fatal("GetGraph() error = nil, want missing JWT error")
	}
}
