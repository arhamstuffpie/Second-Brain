package media

import (
	"os"
	"testing"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

func TestIndicatesNoAudio(t *testing.T) {
	for _, message := range []string{
		"Stream map '0:a:0' matches no streams",
		"Output file does not contain any stream",
	} {
		if !indicatesNoAudio(message) {
			t.Fatalf("indicatesNoAudio(%q) = false", message)
		}
	}
	if indicatesNoAudio("invalid video data") {
		t.Fatal("corrupt input must not be classified as a video without audio")
	}
}

func TestTemporaryReadCloserRemovesExtractedFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "audio-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	reader := &temporaryReadCloser{File: file, path: path}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("os.Stat(%q) error = %v, want not exist", path, err)
	}
}

func TestNewFFmpegExtractorRejectsMissingExecutable(t *testing.T) {
	_, err := NewFFmpegExtractor(config.VideoConfig{
		FFmpegPath:        "/definitely/missing/ffmpeg",
		ExtractionTimeout: time.Second,
	})
	if err == nil {
		t.Fatal("NewFFmpegExtractor() error = nil, want missing executable error")
	}
}

func TestNewFFmpegExtractorResolvesExecutable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	extractor, err := NewFFmpegExtractor(config.VideoConfig{
		FFmpegPath:        executable,
		ExtractionTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewFFmpegExtractor() error = %v", err)
	}
	if extractor.binary != executable {
		t.Fatalf("binary = %q, want %q", extractor.binary, executable)
	}
}
