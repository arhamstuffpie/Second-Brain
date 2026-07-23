package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/arham/ai-second-brain/internal/config"
)

func TestPrettyLoggerUsesConsoleStructure(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var output bytes.Buffer
	logger, err := newLogger(config.LogConfig{Level: "info", Pretty: true}, "development", &output)
	if err != nil {
		t.Fatalf("newLogger() error = %v", err)
	}

	logger.Info().Str("method", "GET").Str("path", "/health").Int("status", 200).Msg("http request")
	line := output.String()
	if strings.HasPrefix(line, "{") {
		t.Fatalf("pretty log is JSON: %s", line)
	}
	for _, want := range []string{"INF", "http request", "method=GET", "path=/health", "status=200"} {
		if !strings.Contains(line, want) {
			t.Fatalf("pretty log %q does not contain %q", line, want)
		}
	}
}

func TestJSONLoggerRemainsMachineReadable(t *testing.T) {
	var output bytes.Buffer
	logger, err := newLogger(config.LogConfig{Level: "info", Pretty: false}, "production", &output)
	if err != nil {
		t.Fatalf("newLogger() error = %v", err)
	}

	logger.Info().Msg("server started")
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode JSON log: %v", err)
	}
	if event["environment"] != "production" || event["message"] != "server started" {
		t.Fatalf("JSON event = %v", event)
	}
}
