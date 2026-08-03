package audio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// FFprobeInspector validates media duration without coupling the voice service
// to the process invocation used by the local deployment.
type FFprobeInspector struct {
	binary  string
	timeout time.Duration
}

func NewFFprobeInspector(binary string, timeout time.Duration) (*FFprobeInspector, error) {
	if strings.TrimSpace(binary) == "" || timeout <= 0 {
		return nil, fmt.Errorf("valid ffprobe configuration is required")
	}
	resolved, err := exec.LookPath(strings.TrimSpace(binary))
	if err != nil {
		return nil, fmt.Errorf("find ffprobe executable %q: %w", binary, err)
	}
	return &FFprobeInspector{binary: resolved, timeout: timeout}, nil
}

func (i *FFprobeInspector) Duration(ctx context.Context, path string) (float64, error) {
	commandCtx, cancel := context.WithTimeout(ctx, i.timeout)
	defer cancel()
	var stderr bytes.Buffer
	command := exec.CommandContext(
		commandCtx,
		i.binary,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	command.Stderr = &stderr
	output, err := command.Output()
	if commandCtx.Err() != nil {
		return 0, fmt.Errorf("inspect audio duration: %w", commandCtx.Err())
	}
	if err != nil {
		return 0, fmt.Errorf("inspect audio duration: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("inspect audio duration: invalid duration %q", strings.TrimSpace(string(output)))
	}
	return duration, nil
}
