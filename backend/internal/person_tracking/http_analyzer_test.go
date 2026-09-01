package person_tracking

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
)

func TestAnalyzePeopleContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		if request.FormValue("metadata") == "" {
			http.Error(writer, "metadata missing", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"recording_id":"r1","processing_version":3,"duration_seconds":1,"analyzed_fps":8,"tracks":[{"id":"t1","provider_track_reference":"t1","lifecycle_status":"ended","first_frame":0,"last_frame":2,"start_time":0,"end_time":0.25,"observation_count":1,"tracking_confidence":0.9,"quality":{"mean":0.8,"maximum":0.8,"usable_observations":1},"gallery_observation_ids":["o1"],"observations":[{"observation_id":"o1","frame_index":0,"timestamp":0,"box":{"x":1,"y":2,"width":80,"height":80},"landmarks":[],"detection_score":0.9,"quality":{"usable":true,"reasons":[],"score":0.8},"pose":{"yaw":0,"pitch":0,"roll":0,"bucket":"frontal"},"embedding":[0.1,0.2],"mouth_visible":true,"mouth_activity":0.1}]}],"provenance":{"detector_model":"yunet","embedding_model":"sface","runtime_version":"python/3.11","device":"cpu","configuration_profile":{}},"warnings":[]}`)
	}))
	defer server.Close()
	analyzer, err := NewHTTPAnalyzer(server.URL, "secret", "yunet", "sface", "local", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analyzer.AnalyzePeople(context.Background(), service.DensePersonAnalysisInput{
		RecordingID: "r1", ProcessingVersion: 3, FileName: "video.mp4",
		Video: strings.NewReader("video"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tracks) != 1 || result.Tracks[0].ID != "t1" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRejectsNonLoopbackPlainHTTP(t *testing.T) {
	if _, err := NewHTTPAnalyzer("http://example.com", "", "yunet", "sface", "local", time.Second); err == nil {
		t.Fatal("expected local plaintext URL validation error")
	}
}
