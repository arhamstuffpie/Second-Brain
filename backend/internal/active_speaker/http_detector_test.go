package active_speaker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func detectorJSONResponse(status int, value any) *http.Response {
	var payload strings.Builder
	_ = json.NewEncoder(&payload).Encode(value)
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(payload.String())), Header: make(http.Header)}
}

func TestHTTPDetectorUploadsVideoAndEvidenceMetadata(t *testing.T) {
	detector, err := NewHTTPDetector(config.ActiveSpeakerConfig{
		Provider: "local", BaseURL: "http://127.0.0.1:8093", APIKey: "secret", Model: "active-test", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	detector.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/healthz" {
			return detectorJSONResponse(http.StatusOK, map[string]any{
				"status": "ok", "provider": "talknet", "model": "active-test", "version": "1",
			}), nil
		}
		if request.Header.Get("Authorization") != "Bearer secret" {
			return detectorJSONResponse(http.StatusUnauthorized, map[string]string{"error": "unauthorized"}), nil
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if request.FormValue("model") != "active-test" || request.FormValue("metadata") == "" {
			t.Fatalf("unexpected active-speaker form: %+v", request.MultipartForm.Value)
		}
		return detectorJSONResponse(http.StatusOK, service.ActiveSpeakerAnalysis{
			Provider: "talknet", Model: "active-test",
			Evidence: []service.ActiveSpeakerEvidence{{
				PersonTrackID: "track-1", SegmentIDs: []string{"segment-1"},
				Score: .95, VisibleMouthCoverage: .9,
			}},
		}), nil
	})
	if _, err := detector.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/video.mp4"
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := detector.DetectActiveSpeakers(context.Background(), service.ActiveSpeakerInput{
		RecordingID: "recording-1", VideoPath: path, FileName: "video.mp4",
		PersonTracks: []service.TemporalPersonTrack{{ID: "track-1"}},
		Segments:     []service.TranscriptSegment{{ID: "segment-1"}},
	})
	if err != nil || len(result.Evidence) != 1 || result.Evidence[0].PersonTrackID != "track-1" {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}
