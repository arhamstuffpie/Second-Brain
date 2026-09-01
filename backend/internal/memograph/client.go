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
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/arham/ai-second-brain/internal/utils"
)

// Client is the single thin wrapper around Memograph's HTTP API.
type Client struct {
	baseURL   string
	apiKey    string
	jwt       string
	http      *http.Client
	writeGate chan struct{}
}

type ResponseError struct {
	StatusCode int
	Message    string
	retryDelay time.Duration
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("Memograph returned %d: %s", e.StatusCode, e.Message)
}

func (e *ResponseError) RetryDelay() time.Duration { return e.retryDelay }

func NewClient(cfg config.MemographConfig) *Client {
	maxConcurrentWrites := cfg.MaxConcurrentWrites
	if maxConcurrentWrites < 1 {
		maxConcurrentWrites = 1
	}
	return &Client{
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:    cfg.APIKey,
		jwt:       cfg.JWT,
		http:      &http.Client{Timeout: cfg.Timeout},
		writeGate: make(chan struct{}, maxConcurrentWrites),
	}
}

func (c *Client) CreateMemory(ctx context.Context, projectID string, request service.MemoryCreateRequest) (json.RawMessage, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	return c.doJSON(ctx, http.MethodPost, "/api/v1/memory/project/"+url.PathEscape(projectID)+"/create-full", request)
}

func (c *Client) InsertEpisode(ctx context.Context, memoryID string, request service.EpisodeInsertRequest) (json.RawMessage, error) {
	select {
	case c.writeGate <- struct{}{}:
		defer func() { <-c.writeGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	payload := map[string]any{"data": request.Data, "meta": request.Meta}
	if request.StructuredGraph != nil {
		payload["structured_graph"] = request.StructuredGraph
	}
	if idempotencyKey := strings.TrimSpace(request.IdempotencyKey); idempotencyKey != "" {
		payload["idempotency_key"] = idempotencyKey
	}
	for key, value := range request.CustomFields {
		switch strings.ToLower(key) {
		case "data", "meta", "graph_config", "structured_graph", "image_url", "document_s3_urls", "idempotency_key":
			continue
		default:
			payload[key] = value
		}
	}
	return c.doJSONWithIdempotency(
		ctx, http.MethodPost, memoryPath(memoryID)+"/create", payload,
		request.IdempotencyKey,
	)
}

func (c *Client) Search(ctx context.Context, memoryID string, request service.MemorySearchRequest) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodPost, memoryPath(memoryID)+"/search", request)
}

func (c *Client) Answer(ctx context.Context, memoryID string, request service.MemoryAnswerRequest) (json.RawMessage, error) {
	request = scopeAnswerRequest(request)
	return c.doJSON(ctx, http.MethodPost, memoryPath(memoryID)+"/answer", request)
}

func (c *Client) AnswerStream(
	ctx context.Context,
	memoryID string,
	request service.MemoryAnswerRequest,
) (service.MemoryAnswerStream, error) {
	if c.baseURL == "" {
		return service.MemoryAnswerStream{}, fmt.Errorf("Memograph is not configured")
	}
	request = scopeAnswerRequest(request)
	request.Stream = true
	body, err := json.Marshal(request)
	if err != nil {
		return service.MemoryAnswerStream{}, fmt.Errorf("encode Memograph stream request: %w", err)
	}
	upstream, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+memoryPath(memoryID)+"/answer",
		bytes.NewReader(body),
	)
	if err != nil {
		return service.MemoryAnswerStream{}, fmt.Errorf("create Memograph stream request: %w", err)
	}
	upstream.Header.Set("Accept", "text/event-stream")
	upstream.Header.Set("Content-Type", "application/json")
	if err := c.authorize(upstream); err != nil {
		return service.MemoryAnswerStream{}, err
	}
	result, err := c.http.Do(upstream)
	if err != nil {
		return service.MemoryAnswerStream{}, fmt.Errorf("call Memograph stream: %w", err)
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		defer result.Body.Close()
		responseBody, readErr := io.ReadAll(io.LimitReader(result.Body, 1000))
		if readErr != nil {
			return service.MemoryAnswerStream{}, fmt.Errorf(
				"Memograph returned %d and its response could not be read",
				result.StatusCode,
			)
		}
		return service.MemoryAnswerStream{}, fmt.Errorf(
			"Memograph returned %d: %s",
			result.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}
	contentType := result.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "text/event-stream") {
		defer result.Body.Close()
		return service.MemoryAnswerStream{}, fmt.Errorf(
			"Memograph returned unexpected stream content type %q",
			contentType,
		)
	}
	return service.MemoryAnswerStream{Body: result.Body, ContentType: contentType}, nil
}

func (c *Client) GetGraph(ctx context.Context, memoryID, groupID string) (json.RawMessage, error) {
	path := memoryPath(memoryID) + "/graph"
	if groupID != "" {
		path += "?group_id=" + url.QueryEscape(groupID)
	}
	return c.doJSON(ctx, http.MethodGet, path, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any) (json.RawMessage, error) {
	return c.doJSONWithIdempotency(ctx, method, path, payload, "")
}

func (c *Client) doJSONWithIdempotency(
	ctx context.Context,
	method, path string,
	payload any,
	idempotencyKey string,
) (json.RawMessage, error) {
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
	if idempotencyKey = strings.TrimSpace(idempotencyKey); idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
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
		retryDelay := time.Duration(0)
		if strings.Contains(strings.ToLower(message), "too many clients already") ||
			strings.Contains(message, "SQLSTATE 53300") {
			retryDelay = 30 * time.Second
		}
		return nil, &ResponseError{
			StatusCode: response.StatusCode, Message: message, retryDelay: retryDelay,
		}
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

func scopeAnswerRequest(request service.MemoryAnswerRequest) service.MemoryAnswerRequest {
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
	return request
}

var _ service.MemographClient = (*Client)(nil)
