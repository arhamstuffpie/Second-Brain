package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"math"
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
	binary      string
	probeBinary string
	timeout     time.Duration
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
	probeBinary := "ffprobe"
	if strings.Contains(filepath.Base(binary), "ffmpeg") {
		probeBinary = filepath.Join(
			filepath.Dir(binary),
			strings.Replace(filepath.Base(binary), "ffmpeg", "ffprobe", 1),
		)
	}
	return &FFmpegExtractor{
		binary: binary, probeBinary: probeBinary, timeout: cfg.ExtractionTimeout,
	}, nil
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
		Path:      audioPath,
		Audio:     &temporaryReadCloser{File: audio, path: audioPath},
	}, nil
}

func (e *FFmpegExtractor) ExtractSpeakerClip(
	ctx context.Context,
	sourcePath string,
	ranges []service.AudioRange,
	maxDuration time.Duration,
) (service.SpeakerClip, error) {
	if strings.TrimSpace(sourcePath) == "" || len(ranges) == 0 || maxDuration <= 0 {
		return service.SpeakerClip{}, fmt.Errorf("speaker clip source, ranges, and maximum duration are required")
	}
	remaining := maxDuration.Seconds()
	selected := make([]service.AudioRange, 0, len(ranges))
	for _, audioRange := range ranges {
		if remaining <= 0 {
			break
		}
		if math.IsNaN(audioRange.StartTime) || math.IsNaN(audioRange.EndTime) ||
			math.IsInf(audioRange.StartTime, 0) || math.IsInf(audioRange.EndTime, 0) ||
			audioRange.StartTime < 0 || audioRange.EndTime <= audioRange.StartTime {
			continue
		}
		duration := math.Min(audioRange.EndTime-audioRange.StartTime, remaining)
		selected = append(selected, service.AudioRange{
			StartTime: audioRange.StartTime,
			EndTime:   audioRange.StartTime + duration,
		})
		remaining -= duration
	}
	if len(selected) == 0 {
		return service.SpeakerClip{}, fmt.Errorf("speaker clip has no valid ranges")
	}

	temp, err := os.CreateTemp("", "second-brain-speaker-*.wav")
	if err != nil {
		return service.SpeakerClip{}, fmt.Errorf("create speaker clip file: %w", err)
	}
	clipPath := temp.Name()
	_ = temp.Close()
	defer os.Remove(clipPath)

	filters := make([]string, 0, len(selected)+1)
	inputs := make([]string, 0, len(selected))
	totalDuration := 0.0
	for index, audioRange := range selected {
		label := fmt.Sprintf("a%d", index)
		filters = append(filters, fmt.Sprintf(
			"[0:a]atrim=start=%s:end=%s,asetpts=PTS-STARTPTS[%s]",
			strconv.FormatFloat(audioRange.StartTime, 'f', 6, 64),
			strconv.FormatFloat(audioRange.EndTime, 'f', 6, 64), label,
		))
		inputs = append(inputs, "["+label+"]")
		totalDuration += audioRange.EndTime - audioRange.StartTime
	}
	if len(inputs) == 1 {
		filters = append(filters, inputs[0]+"anull[out]")
	} else {
		filters = append(filters, strings.Join(inputs, "")+fmt.Sprintf("concat=n=%d:v=0:a=1[out]", len(inputs)))
	}

	commandCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	var stderr bytes.Buffer
	command := exec.CommandContext(
		commandCtx, e.binary,
		"-hide_banner", "-loglevel", "error", "-y", "-i", sourcePath,
		"-filter_complex", strings.Join(filters, ";"),
		"-map", "[out]", "-vn", "-ac", "1", "-ar", "16000", "-f", "wav", clipPath,
	)
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if commandCtx.Err() != nil {
			return service.SpeakerClip{}, fmt.Errorf("extract speaker clip: %w", commandCtx.Err())
		}
		return service.SpeakerClip{}, fmt.Errorf("extract speaker clip: %w: %s", err, bounded(stderr.String()))
	}
	audio, err := os.ReadFile(clipPath)
	if err != nil {
		return service.SpeakerClip{}, fmt.Errorf("read speaker clip: %w", err)
	}
	if len(audio) == 0 {
		return service.SpeakerClip{}, fmt.Errorf("speaker clip is empty")
	}
	return service.SpeakerClip{
		FileName: filepath.Base(clipPath), MediaType: "audio/wav", Audio: audio,
		DurationSeconds: totalDuration,
	}, nil
}

func (e *FFmpegExtractor) ExtractFrames(
	ctx context.Context,
	videoPath string,
	interval time.Duration,
	maxFrames int,
) (service.FrameExtraction, error) {
	if interval <= 0 || maxFrames < 2 {
		return service.FrameExtraction{}, fmt.Errorf(
			"frame interval must be positive and emergency frame ceiling must be at least two",
		)
	}
	duration, timestamps, err := e.probeVideoFrames(ctx, videoPath)
	if err != nil {
		return service.FrameExtraction{}, err
	}
	events, err := e.probeVideoEvents(ctx, videoPath, timestamps)
	if err != nil {
		return service.FrameExtraction{}, err
	}
	selected := selectedFrames(duration, interval.Seconds(), maxFrames, timestamps, events)
	if len(selected) == 0 {
		return service.FrameExtraction{}, fmt.Errorf("video produced no usable frames")
	}
	indexes := make([]int, len(selected))
	for index := range selected {
		indexes[index] = selected[index].index
	}
	outputDir, err := os.MkdirTemp("", "second-brain-frames-*")
	if err != nil {
		return service.FrameExtraction{}, fmt.Errorf("create frame directory: %w", err)
	}
	defer os.RemoveAll(outputDir)

	commandCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	var stderr bytes.Buffer
	pattern := filepath.Join(outputDir, "frame-%06d.jpg")
	predicates := make([]string, 0, len(indexes))
	for _, index := range indexes {
		predicates = append(predicates, fmt.Sprintf("eq(n\\,%d)", index))
	}
	command := exec.CommandContext(
		commandCtx,
		e.binary,
		"-hide_banner", "-loglevel", "error",
		"-i", videoPath,
		"-vf", "select="+strings.Join(predicates, "+"),
		"-vsync", "vfr",
		"-q:v", "3",
		pattern,
	)
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if commandCtx.Err() != nil {
			return service.FrameExtraction{}, fmt.Errorf("extract video frames: %w", commandCtx.Err())
		}
		return service.FrameExtraction{}, fmt.Errorf("extract video frames: %w: %s", err, bounded(stderr.String()))
	}

	paths, err := filepath.Glob(filepath.Join(outputDir, "frame-*.jpg"))
	if err != nil {
		return service.FrameExtraction{}, fmt.Errorf("list extracted frames: %w", err)
	}
	sort.Strings(paths)
	if len(paths) != len(indexes) {
		return service.FrameExtraction{}, fmt.Errorf(
			"video produced %d selected frames, expected %d", len(paths), len(indexes),
		)
	}
	frames := make([]service.VideoFrame, 0, len(paths))
	for index, path := range paths {
		image, err := os.ReadFile(path)
		if err != nil {
			return service.FrameExtraction{}, fmt.Errorf("read extracted frame: %w", err)
		}
		frames = append(frames, service.VideoFrame{
			Timestamp:       timestamps[indexes[index]],
			SelectionReason: selected[index].reason,
			ImageQuality:    imageQuality(image),
			FileName:        filepath.Base(path),
			MediaType:       "image/jpeg",
			Image:           image,
		})
	}
	return service.FrameExtraction{DurationSeconds: duration, Frames: frames}, nil
}

func (e *FFmpegExtractor) ExtractFramesAt(
	ctx context.Context,
	videoPath string,
	frames []service.VideoFrame,
) ([]service.VideoFrame, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("at least one frame timestamp is required")
	}
	_, timestamps, err := e.probeVideoFrames(ctx, videoPath)
	if err != nil {
		return nil, err
	}
	indexes := make([]int, len(frames))
	for index, frame := range frames {
		candidate := sort.Search(len(timestamps), func(i int) bool {
			return timestamps[i] >= frame.Timestamp
		})
		if candidate == len(timestamps) {
			candidate = len(timestamps) - 1
		}
		indexes[index] = candidate
	}
	images, err := e.extractFrameIndexes(ctx, videoPath, indexes)
	if err != nil {
		return nil, err
	}
	result := append([]service.VideoFrame(nil), frames...)
	for index := range result {
		result[index].Timestamp = timestamps[indexes[index]]
		result[index].Image = images[index]
		result[index].MediaType = "image/jpeg"
	}
	return result, nil
}

func (e *FFmpegExtractor) extractFrameIndexes(
	ctx context.Context,
	videoPath string,
	indexes []int,
) ([][]byte, error) {
	outputDir, err := os.MkdirTemp("", "second-brain-frames-at-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(outputDir)
	predicates := make([]string, len(indexes))
	for index, frameIndex := range indexes {
		predicates[index] = fmt.Sprintf("eq(n\\,%d)", frameIndex)
	}
	commandCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	var stderr bytes.Buffer
	command := exec.CommandContext(
		commandCtx, e.binary, "-hide_banner", "-loglevel", "error", "-i", videoPath,
		"-vf", "select="+strings.Join(predicates, "+"),
		"-vsync", "vfr", "-q:v", "3", filepath.Join(outputDir, "frame-%06d.jpg"),
	)
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("extract requested video frames: %w: %s", err, bounded(stderr.String()))
	}
	paths, err := filepath.Glob(filepath.Join(outputDir, "frame-*.jpg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) != len(indexes) {
		return nil, fmt.Errorf("extracted %d requested frames, expected %d", len(paths), len(indexes))
	}
	images := make([][]byte, len(paths))
	for index, path := range paths {
		images[index], err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}
	return images, nil
}

type videoFrameProbe struct {
	Frames []struct {
		Timestamp string `json:"best_effort_timestamp_time"`
	} `json:"frames"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

type videoEvent struct {
	index  int
	score  float64
	reason string
}

func (e *FFmpegExtractor) probeVideoEvents(
	ctx context.Context,
	videoPath string,
	timestamps []float64,
) ([]videoEvent, error) {
	escaped := strings.NewReplacer("\\", "\\\\", "'", "\\'", ":", "\\:").Replace(videoPath)
	commandCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	command := exec.CommandContext(
		commandCtx, e.probeBinary, "-v", "error", "-f", "lavfi",
		"-i", "movie=filename='"+escaped+"',select=gt(scene\\,0.04)",
		"-show_entries", "frame=pts_time:frame_tags=lavfi.scene_score", "-of", "json",
	)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("inspect video events: %w", err)
	}
	var probe struct {
		Frames []struct {
			Timestamp string            `json:"pts_time"`
			Tags      map[string]string `json:"tags"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("decode video event inspection: %w", err)
	}
	events := make([]videoEvent, 0, len(probe.Frames))
	for _, frame := range probe.Frames {
		timestamp, timestampErr := strconv.ParseFloat(frame.Timestamp, 64)
		score, scoreErr := strconv.ParseFloat(frame.Tags["lavfi.scene_score"], 64)
		if timestampErr != nil || scoreErr != nil {
			continue
		}
		index := sort.Search(len(timestamps), func(i int) bool { return timestamps[i] >= timestamp })
		if index == len(timestamps) {
			index = len(timestamps) - 1
		}
		reason := "motion_peak"
		switch {
		case score >= 0.30:
			reason = "scene_change"
		case score >= 0.12:
			// ponytail: FFmpeg pixel deltas approximate screen/text changes; add OCR-aware
			// selection only if missed transient text is measured in production.
			reason = "text_change"
		}
		events = append(events, videoEvent{index: index, score: score, reason: reason})
	}
	markMotionEdges(events, timestamps)
	return events, nil
}

func markMotionEdges(events []videoEvent, timestamps []float64) {
	for start := 0; start < len(events); {
		if events[start].reason != "motion_peak" {
			start++
			continue
		}
		end := start + 1
		peak := start
		for end < len(events) && events[end].reason == "motion_peak" &&
			timestamps[events[end].index]-timestamps[events[end-1].index] <= 1.5 {
			if events[end].score > events[peak].score {
				peak = end
			}
			end++
		}
		if end-start > 2 {
			events[start].reason = "motion_start"
			events[end-1].reason = "motion_end"
			if peak == start || peak == end-1 {
				peak = start + 1
			}
			events[peak].reason = "motion_peak"
		} else if end-start == 2 {
			events[start].reason = "motion_start"
			events[end-1].reason = "motion_end"
		}
		start = end
	}
}

func (e *FFmpegExtractor) probeVideoFrames(
	ctx context.Context,
	videoPath string,
) (float64, []float64, error) {
	commandCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	var stderr bytes.Buffer
	command := exec.CommandContext(
		commandCtx, e.probeBinary,
		"-v", "error", "-select_streams", "v:0", "-show_frames",
		"-show_entries", "frame=best_effort_timestamp_time:format=duration",
		"-of", "json", videoPath,
	)
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		if commandCtx.Err() != nil {
			return 0, nil, fmt.Errorf("inspect video frames: %w", commandCtx.Err())
		}
		return 0, nil, fmt.Errorf("inspect video frames: %w: %s", err, bounded(stderr.String()))
	}
	var probe videoFrameProbe
	if err := json.Unmarshal(output, &probe); err != nil {
		return 0, nil, fmt.Errorf("decode video frame inspection: %w", err)
	}
	duration, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, nil, fmt.Errorf("video has invalid duration %q", probe.Format.Duration)
	}
	timestamps := make([]float64, 0, len(probe.Frames))
	for _, frame := range probe.Frames {
		timestamp, parseErr := strconv.ParseFloat(frame.Timestamp, 64)
		if parseErr == nil && timestamp >= 0 && !math.IsNaN(timestamp) && !math.IsInf(timestamp, 0) {
			timestamps = append(timestamps, timestamp)
		}
	}
	if len(timestamps) == 0 {
		return 0, nil, fmt.Errorf("video produced no decoded frame timestamps")
	}
	return duration, timestamps, nil
}

func frameBudget(duration, interval float64, emergencyCeiling int) (int, int, int) {
	periodic := int(math.Ceil(duration / interval))
	if periodic < 1 {
		periodic = 1
	}
	events := min((periodic+1)/2, 30)
	maximum := periodic + events
	if maximum > emergencyCeiling {
		maximum = emergencyCeiling
	}
	return periodic, events, maximum
}

type selectedFrame struct {
	index  int
	reason string
}

func selectedFrames(
	duration, interval float64,
	emergencyCeiling int,
	timestamps []float64,
	events []videoEvent,
) []selectedFrame {
	periodic, _, maximum := frameBudget(duration, interval, emergencyCeiling)
	byIndex := make(map[int]string, maximum)
	for index := 0; index < periodic; index++ {
		target := timestamps[0] + float64(index)*interval
		frameIndex := sort.Search(len(timestamps), func(candidate int) bool {
			return timestamps[candidate] >= target
		})
		if frameIndex == len(timestamps) {
			frameIndex = len(timestamps) - 1
		}
		byIndex[frameIndex] = "periodic"
	}
	byIndex[0] = "first"
	last := len(timestamps) - 1
	byIndex[last] = "final"
	for _, preferredReason := range []string{"scene_change", "motion_start", "motion_peak", "motion_end", "text_change"} {
		for _, event := range events {
			if len(byIndex) >= maximum {
				break
			}
			if event.reason == preferredReason {
				if _, duplicate := byIndex[event.index]; !duplicate {
					byIndex[event.index] = event.reason
				}
			}
		}
	}
	indexes := make([]int, 0, len(byIndex))
	for index := range byIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	distributed := make([]int, 0, maximum)
	if len(indexes) > maximum {
		for index := 0; index < maximum; index++ {
			position := int(math.Round(float64(index) * float64(len(indexes)-1) / float64(maximum-1)))
			distributed = append(distributed, indexes[position])
		}
		indexes = distributed
	}
	selected := make([]selectedFrame, len(indexes))
	for index, frameIndex := range indexes {
		selected[index] = selectedFrame{index: frameIndex, reason: byIndex[frameIndex]}
	}
	return selected
}

func selectedFrameIndexes(duration, interval float64, emergencyCeiling int, timestamps []float64) []int {
	frames := selectedFrames(duration, interval, emergencyCeiling, timestamps, nil)
	indexes := make([]int, len(frames))
	for index := range frames {
		indexes[index] = frames[index].index
	}
	return indexes
}

func imageQuality(data []byte) float64 {
	// ponytail: luminance variance/exposure is the FFmpeg-only quality proxy;
	// replace it with a learned scorer only if citation-frame quality proves poor.
	image, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	bounds := image.Bounds()
	step := max(1, min(bounds.Dx(), bounds.Dy())/64)
	count, sum, squareSum := 0.0, 0.0, 0.0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r, g, b, _ := image.At(x, y).RGBA()
			luminance := (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 65535
			count++
			sum += luminance
			squareSum += luminance * luminance
		}
	}
	mean := sum / count
	variance := squareSum/count - mean*mean
	exposure := 1 - math.Min(1, math.Abs(mean-0.5)*2)
	return math.Round(math.Min(1, math.Sqrt(math.Max(variance, 0))*2+exposure*0.5)*100) / 100
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
var _ service.SpeakerClipper = (*FFmpegExtractor)(nil)
