package stt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

func TestOpenAITranscriberParsesDiarizedSegments(t *testing.T) {
	handler := func(request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer stt-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if request.FormValue("model") != "gpt-4o-transcribe-diarize" {
			t.Fatalf("model = %q", request.FormValue("model"))
		}
		if request.FormValue("response_format") != "diarized_json" {
			t.Fatalf("response_format = %q", request.FormValue("response_format"))
		}
		if request.FormValue("chunking_strategy") != "auto" {
			t.Fatalf("chunking_strategy = %q", request.FormValue("chunking_strategy"))
		}
		if got := request.Form["known_speaker_names[]"]; len(got) != 1 || got[0] != "owner_ref" {
			t.Fatalf("known speaker names = %#v", got)
		}
		if got := request.Form["known_speaker_references[]"]; len(got) != 1 ||
			!strings.HasPrefix(got[0], "data:audio/wav;base64,") {
			t.Fatalf("known speaker references = %#v", got)
		}
	}

	transcriber, err := NewOpenAI(config.STTConfig{
		BaseURL: "https://openai.test/v1", APIKey: "stt-key",
		Model: "gpt-4o-transcribe-diarize", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	transcriber.client = &http.Client{Transport: sttRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		handler(request)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"text":"Hello there.",
				"duration":2.5,
				"segments":[{"id":"seg-1","start":0.2,"end":2.1,"speaker":"A","text":"Hello there."}]
			}`)),
			Request: request,
		}, nil
	})}
	result, err := transcriber.Transcribe(context.Background(), service.TranscriptionInput{
		FileName: "voice.wav", MediaType: "audio/wav", Audio: strings.NewReader("audio"),
		KnownSpeakers: []service.KnownSpeakerReference{{
			ID: "sample-1", ProviderLabel: "owner_ref", MediaType: "audio/wav", Audio: []byte("owner"),
		}},
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if len(result.Segments) != 1 || result.Segments[0].ID != "seg-1" || result.Segments[0].Speaker != "A" ||
		result.Segments[0].StartTime != 0.2 || result.Segments[0].EndTime != 2.1 {
		t.Fatalf("segments = %+v", result.Segments)
	}
}

type sttRoundTripFunc func(*http.Request) (*http.Response, error)

func (function sttRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
