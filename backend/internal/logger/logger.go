package logger

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/rs/zerolog"
)

func New(cfg config.LogConfig, environment string) (zerolog.Logger, error) {
	return newLogger(cfg, environment, os.Stdout)
}

func newLogger(cfg config.LogConfig, environment string, writer io.Writer) (zerolog.Logger, error) {
	level, err := zerolog.ParseLevel(strings.ToLower(cfg.Level))
	if err != nil {
		return zerolog.Logger{}, fmt.Errorf("parse log level: %w", err)
	}

	output := writer
	if cfg.Pretty {
		output = zerolog.NewConsoleWriter(func(writer *zerolog.ConsoleWriter) {
			writer.Out = output
			writer.TimeFormat = "15:04:05.000"
			writer.PartsOrder = []string{
				zerolog.TimestampFieldName,
				zerolog.LevelFieldName,
				zerolog.MessageFieldName,
			}
			writer.FieldsOrder = []string{
				"method",
				"path",
				"status",
				"latency",
				"request_id",
				"client_ip",
				"response_bytes",
				"request_bytes",
				"query",
				"address",
				"environment",
			}
		})
	}

	return zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Str("environment", environment).
		Logger(), nil
}
