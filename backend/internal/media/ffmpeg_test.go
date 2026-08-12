package media

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestFrameBudgetUsesDurationAndEventAllowance(t *testing.T) {
	for _, test := range []struct {
		duration, interval        float64
		periodic, events, maximum int
	}{
		{30, 5, 6, 3, 9},
		{30, 2, 15, 8, 23},
		{30, 1, 30, 15, 45},
		{60, 5, 12, 6, 18},
	} {
		periodic, events, maximum := frameBudget(test.duration, test.interval, 120)
		if periodic != test.periodic || events != test.events || maximum != test.maximum {
			t.Fatalf(
				"frameBudget(%v, %v) = %d, %d, %d; want %d, %d, %d",
				test.duration, test.interval, periodic, events, maximum,
				test.periodic, test.events, test.maximum,
			)
		}
	}
}

func TestSelectedFrameIndexesCoverCompleteTimeline(t *testing.T) {
	timestamps := make([]float64, 60)
	for index := range timestamps {
		timestamps[index] = float64(index)
	}

	selected := selectedFrameIndexes(60, 1, 10, timestamps)
	if len(selected) != 10 || selected[0] != 0 || selected[len(selected)-1] != 59 {
		t.Fatalf("selected indexes = %v, want ten distributed frames including 0 and 59", selected)
	}
	for index := 1; index < len(selected); index++ {
		if selected[index] <= selected[index-1] {
			t.Fatalf("selected indexes are not ordered: %v", selected)
		}
	}
}

func TestExtractFramesUsesDecodedTimestampsAndFinalFrame(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}
	videoPath := filepath.Join(t.TempDir(), "coverage.mp4")
	command := exec.Command(
		ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=2:duration=4",
		"-pix_fmt", "yuv420p", videoPath,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create test video: %v: %s", err, output)
	}
	extractor, err := NewFFmpegExtractor(config.VideoConfig{
		FFmpegPath: ffmpeg, ExtractionTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	extraction, err := extractor.ExtractFrames(
		t.Context(), videoPath, time.Second, 120,
	)
	if err != nil {
		t.Fatal(err)
	}
	if extraction.DurationSeconds < 3.9 || extraction.DurationSeconds > 4.1 {
		t.Fatalf("duration = %v, want about four seconds", extraction.DurationSeconds)
	}
	frames := extraction.Frames
	if len(frames) < 2 || frames[0].Timestamp != 0 || frames[len(frames)-1].Timestamp < 3.5 {
		t.Fatalf("frame timestamps = %+v, want first and near-final decoded PTS", frames)
	}
	if frames[0].SelectionReason != "first" || frames[len(frames)-1].SelectionReason != "final" {
		t.Fatalf("boundary reasons = %q/%q, want first/final", frames[0].SelectionReason, frames[len(frames)-1].SelectionReason)
	}
	for _, frame := range frames {
		if frame.ImageQuality < 0 || frame.ImageQuality > 1 {
			t.Fatalf("image quality = %v, want 0..1", frame.ImageQuality)
		}
	}
}

func TestSelectedFramesAddsEventsWithoutLosingCoverage(t *testing.T) {
	timestamps := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	frames := selectedFrames(20, 5, 20, timestamps, []videoEvent{
		{index: 2, reason: "scene_change"}, {index: 3, reason: "motion_peak"},
		{index: 4, reason: "text_change"},
	})
	reasons := make(map[string]bool)
	for _, frame := range frames {
		reasons[frame.reason] = true
	}
	for _, reason := range []string{"first", "final", "scene_change"} {
		if !reasons[reason] {
			t.Fatalf("selected frames = %+v, missing %s", frames, reason)
		}
	}
}
