package memograph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/arham/ai-second-brain/internal/utils"
)

// Client is the single thin wrapper around Memograph's HTTP API.
type Client struct {
	baseURL string
	apiKey  string
	jwt     string
	http    *http.Client
}

func NewClient(cfg config.MemographConfig) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		jwt:     cfg.JWT,
		http:    &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) CreateMemory(ctx context.Context, projectID string, request service.MemoryCreateRequest) (json.RawMessage, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	return c.doJSON(ctx, http.MethodPost, "/api/v1/memory/project/"+url.PathEscape(projectID)+"/create-full", request)
}

func (c *Client) InsertEpisode(ctx context.Context, memoryID string, request service.EpisodeInsertRequest) (json.RawMessage, error) {
	payload := map[string]any{"data": request.Data, "meta": request.Meta}
	for key, value := range request.CustomFields {
		switch strings.ToLower(key) {
		case "data", "meta", "graph_config", "image_url", "document_s3_urls":
			continue
		default:
			payload[key] = value
		}
	}
	return c.doJSON(ctx, http.MethodPost, memoryPath(memoryID)+"/create", payload)
}

func (c *Client) Search(ctx context.Context, memoryID string, request service.MemorySearchRequest) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodPost, memoryPath(memoryID)+"/search", request)
}

func (c *Client) Answer(ctx context.Context, memoryID string, request service.MemoryAnswerRequest) (json.RawMessage, error) {
	// The currently deployed Memograph answer handler accepts filters but not a
	// dedicated group_id field. Keep group_id in the payload for forward
	// compatibility and also enforce it through the graph-search filter.
	if request.GroupID != "" {
		groupFilter := map[string]any{"group_id": map[string]any{"eq": request.GroupID}}
		if len(request.Filters) == 0 {
			request.Filters = groupFilter
		} else {
			request.Filters = map[string]any{"AND": []any{request.Filters, groupFilter}}
		}
	}
	return c.doJSON(ctx, http.MethodPost, memoryPath(memoryID)+"/answer", request)
}

func (c *Client) GetGraph(ctx context.Context, memoryID, groupID string) (json.RawMessage, error) {
	path := memoryPath(memoryID) + "/graph"
	if groupID != "" {
		path += "?group_id=" + url.QueryEscape(groupID)
	}
	return c.doJSON(ctx, http.MethodGet, path, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any) (json.RawMessage, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("Memograph is not configured")
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode Memograph request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create Memograph request: %w", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if err := c.authorize(request); err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Memograph: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read Memograph response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(responseBody))
		if len(message) > 1000 {
			message = message[:1000]
		}
		return nil, fmt.Errorf("Memograph returned %d: %s", response.StatusCode, message)
	}
	if len(responseBody) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(responseBody) {
		return nil, fmt.Errorf("Memograph returned invalid JSON")
	}
	return json.RawMessage(responseBody), nil
}

func (c *Client) authorize(request *http.Request) error {
	if apiKey := strings.TrimSpace(utils.MemographAPIKeyFromContext(request.Context())); apiKey != "" {
		request.Header.Set("X-Api-Key", apiKey)
		return nil
	}
	if c.apiKey != "" {
		request.Header.Set("X-Api-Key", c.apiKey)
		return nil
	}
	if c.jwt != "" {
		request.Header.Set("Authorization", "Bearer "+c.jwt)
		return nil
	}
	return fmt.Errorf("Memograph API key or JWT is required")
}

func memoryPath(memoryID string) string {
	return "/api/v1/memory/" + url.PathEscape(memoryID)
}

var _ service.MemographClient = (*Client)(nil)
