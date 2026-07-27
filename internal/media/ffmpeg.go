package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

type FFmpegExtractor struct {
	binary  string
	timeout time.Duration
}

func NewFFmpegExtractor(cfg config.VideoConfig) (*FFmpegExtractor, error) {
	if strings.TrimSpace(cfg.FFmpegPath) == "" || cfg.ExtractionTimeout <= 0 {
		return nil, fmt.Errorf("valid FFmpeg configuration is required")
	}
	binary, err := exec.LookPath(strings.TrimSpace(cfg.FFmpegPath))
	if err != nil {
		return nil, fmt.Errorf(
			"find FFmpeg executable %q: %w; install FFmpeg or set APP_VIDEO_FFMPEG_PATH",
			cfg.FFmpegPath, err,
		)
	}
	return &FFmpegExtractor{binary: binary, timeout: cfg.ExtractionTimeout}, nil
}

func (e *FFmpegExtractor) ExtractAudio(
	ctx context.Context,
	videoPath string,
) (service.ExtractedAudio, error) {
	commandCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	temp, err := os.CreateTemp("", "second-brain-audio-*.wav")
	if err != nil {
		return service.ExtractedAudio{}, fmt.Errorf("create extracted audio file: %w", err)
	}
	audioPath := temp.Name()
	_ = temp.Close()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(audioPath)
		}
	}()

	var stderr bytes.Buffer
	command := exec.CommandContext(
		commandCtx,
		e.binary,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", videoPath,
		"-map", "0:a:0?",
		"-vn", "-ac", "1", "-ar", "16000",
		"-f", "wav", audioPath,
	)
	command.Stderr = &stderr
	err = command.Run()
	if commandCtx.Err() != nil {
		return service.ExtractedAudio{}, fmt.Errorf("extract video audio: %w", commandCtx.Err())
	}
	info, statErr := os.Stat(audioPath)
	empty := statErr == nil && info.Size() == 0
	if empty && (err == nil || indicatesNoAudio(stderr.String())) {
		return service.ExtractedAudio{}, service.ErrNoAudioTrack
	}
	if err != nil {
		return service.ExtractedAudio{}, fmt.Errorf(
			"extract video audio: %w: %s", err, bounded(stderr.String()),
		)
	}
	audio, err := os.Open(audioPath)
	if err != nil {
		return service.ExtractedAudio{}, fmt.Errorf("open extracted audio: %w", err)
	}
	keep = true
	return service.ExtractedAudio{
		FileName:  strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath)) + ".wav",
		MediaType: "audio/wav",
		Audio:     &temporaryReadCloser{File: audio, path: audioPath},
	}, nil
}

func (e *FFmpegExtractor) ExtractFrames(
	ctx context.Context,
	videoPath string,
	interval time.Duration,
	maxFrames int,
) ([]service.VideoFrame, error) {
	if interval <= 0 || maxFrames < 1 {
		return nil, fmt.Errorf("frame interval and maximum frame count must be positive")
	}
	outputDir, err := os.MkdirTemp("", "second-brain-frames-*")
	if err != nil {
		return nil, fmt.Errorf("create frame directory: %w", err)
	}
	defer os.RemoveAll(outputDir)

	commandCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	var stderr bytes.Buffer
	pattern := filepath.Join(outputDir, "frame-%06d.jpg")
	framesPerSecond := 1 / interval.Seconds()
	command := exec.CommandContext(
		commandCtx,
		e.binary,
		"-hide_banner", "-loglevel", "error",
		"-i", videoPath,
		"-vf", "fps="+strconv.FormatFloat(framesPerSecond, 'f', 8, 64),
		"-frames:v", strconv.Itoa(maxFrames),
		"-q:v", "3",
		pattern,
	)
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if commandCtx.Err() != nil {
			return nil, fmt.Errorf("extract video frames: %w", commandCtx.Err())
		}
		return nil, fmt.Errorf("extract video frames: %w: %s", err, bounded(stderr.String()))
	}

	paths, err := filepath.Glob(filepath.Join(outputDir, "frame-*.jpg"))
	if err != nil {
		return nil, fmt.Errorf("list extracted frames: %w", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("video produced no frames")
	}
	frames := make([]service.VideoFrame, 0, len(paths))
	for index, path := range paths {
		image, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read extracted frame: %w", err)
		}
		frames = append(frames, service.VideoFrame{
			Timestamp: float64(index) * interval.Seconds(),
			FileName:  filepath.Base(path),
			MediaType: "image/jpeg",
			Image:     image,
		})
	}
	return frames, nil
}

func indicatesNoAudio(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "does not contain any stream") ||
		strings.Contains(lower, "matches no streams") ||
		strings.Contains(lower, "output file does not contain any stream")
}

func bounded(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}

type temporaryReadCloser struct {
	*os.File
	path string
}

func (reader *temporaryReadCloser) Close() error {
	closeErr := reader.File.Close()
	removeErr := os.Remove(reader.path)
	if closeErr != nil {
		return closeErr
	}
	if removeErr != nil && !os.IsNotExist(removeErr) {
		return removeErr
	}
	return nil
}

var _ service.MediaExtractor = (*FFmpegExtractor)(nil)
