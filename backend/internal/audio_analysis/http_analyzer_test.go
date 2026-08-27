package audio_analysis

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

func TestAnalyzeAudioContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"recording_id":"r1","processing_version":3,"duration_seconds":2,"regions":[{"id":"g1","start_time":0,"end_time":2,"kind":"overlap","active_speaker_labels":["A","B"],"concurrent_speaker_count":2,"overlap":true,"status":"ambiguous"}],"sources":[{"id":"s1","audio_region_id":"g1","source_index":0,"separation_status":"ambiguous"}],"provenance":{"diarization_model":"pyannote","separation_model":"sepformer","runtime_version":"python/3.11","device":"cpu","configuration_profile":{}},"warnings":[]}`)
	}))
	defer server.Close()
	analyzer, err := NewHTTPAnalyzer(server.URL, "", "local", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analyzer.AnalyzeAudio(context.Background(), service.AudioAnalysisInput{
		RecordingID: "r1", ProcessingVersion: 3, FileName: "audio.wav", Audio: strings.NewReader("audio"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Regions) != 1 || !result.Regions[0].Overlap {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRejectsUnknownSourceRegion(t *testing.T) {
	input := service.AudioAnalysisInput{RecordingID: "r1", ProcessingVersion: 3}
	result := service.AudioAnalysis{
		RecordingID: "r1", ProcessingVersion: 3, DurationSeconds: 1,
		Sources: []service.AnalyzedAudioSource{{ID: "s1", AudioRegionID: "missing"}},
	}
	if err := validate(result, input); err == nil {
		t.Fatal("expected source relationship validation error")
	}
}
