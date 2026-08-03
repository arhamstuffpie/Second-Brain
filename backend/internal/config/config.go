package config

import (
	"fmt"
	"log"
	"strings"
	"time"
)

type Config struct {
	Environment string
	HTTP        HTTPConfig
	Database    DatabaseConfig
	JWT         JWTConfig
	CORS        CORSConfig
	Log         LogConfig
	Voice       VoiceConfig
	Video       VideoConfig
	STT         STTConfig
	Vision      VisionConfig
	Memograph   MemographConfig
	Worker      WorkerConfig
}

type HTTPConfig struct {
	Host              string
	Port              int
	TrustedProxies    []string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
}

func (c HTTPConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	ConnectTimeout  time.Duration
}

type JWTConfig struct {
	Secret         string
	Issuer         string
	AccessTokenTTL time.Duration
}

type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         int
}

type LogConfig struct {
	Level  string
	Pretty bool
}

type VoiceConfig struct {
	StorageDir               string
	EnrollmentStorageDir     string
	MaxUploadBytes           int64
	EnrollmentMaxUploadBytes int64
	EnrollmentMinDuration    time.Duration
	EnrollmentMaxDuration    time.Duration
	FFprobePath              string
	InspectionTimeout        time.Duration
	EpisodeDuration          time.Duration
	EpisodeSilenceGap        time.Duration
	EpisodeMaxDuration       time.Duration
}

type VideoConfig struct {
	StorageDir        string
	MaxUploadBytes    int64
	EpisodeDuration   time.Duration
	FrameInterval     time.Duration
	MaxFrames         int
	FFmpegPath        string
	ExtractionTimeout time.Duration
}

type STTConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	Language string
	Prompt   string
	Timeout  time.Duration
}

type VisionConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	Detail   string
	Timeout  time.Duration
}

type MemographConfig struct {
	BaseURL string
	APIKey  string
	JWT     string
	Timeout time.Duration
}

type WorkerConfig struct {
	Enabled      bool
	PollInterval time.Duration
	Concurrency  int
	MaxAttempts  int
}

func Load() (Config, error) {
	environment := GetEnv("APP_ENV", "development")
	cfg := Config{
		Environment: environment,
		HTTP: HTTPConfig{
			Host:              GetEnv("APP_HTTP_HOST", "0.0.0.0"),
			Port:              GetEnvInt("APP_HTTP_PORT", 8181),
			TrustedProxies:    getEnvCSV("APP_HTTP_TRUSTED_PROXIES", ""),
			ReadHeaderTimeout: getEnvDuration("APP_HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       getEnvDuration("APP_HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:      getEnvDuration("APP_HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:       getEnvDuration("APP_HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   getEnvDuration("APP_HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
			MaxHeaderBytes:    GetEnvInt("APP_HTTP_MAX_HEADER_BYTES", 1<<20),
		},
		Database: DatabaseConfig{
			URL:             GetEnv("APP_DATABASE_URL", "postgresql://postgres:mysecretpassword@localhost:5433/mysecondbrain"),
			MaxOpenConns:    GetEnvInt("APP_DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    GetEnvInt("APP_DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("APP_DB_CONN_MAX_LIFETIME", 30*time.Minute),
			ConnMaxIdleTime: getEnvDuration("APP_DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
			ConnectTimeout:  getEnvDuration("APP_DB_CONNECT_TIMEOUT", 5*time.Second),
		},
		JWT: JWTConfig{
			Secret:         GetEnv("APP_JWT_SECRET", "K9mP2xL7vQ4wR8tY1uI3oA5sD6fG0hJ2"),
			Issuer:         GetEnv("APP_JWT_ISSUER", "ai-second-brain"),
			AccessTokenTTL: getEnvDuration("APP_JWT_ACCESS_TOKEN_TTL", 24*time.Hour),
		},
		CORS: CORSConfig{
			AllowedOrigins: getEnvCSV("APP_CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
			AllowedMethods: getEnvCSV("APP_CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
			AllowedHeaders: getEnvCSV("APP_CORS_ALLOWED_HEADERS", "Authorization,Content-Type,X-Request-ID,X-Memograph-Api-Key"),
			MaxAge:         GetEnvInt("APP_CORS_MAX_AGE_SECONDS", 600),
		},
		Log: LogConfig{
			Level:  GetEnv("APP_LOG_LEVEL", "info"),
			Pretty: GetEnvBool("APP_LOG_PRETTY", environment != "production"),
		},
		Voice: VoiceConfig{
			StorageDir:               GetEnv("APP_VOICE_STORAGE_DIR", "./data/audio"),
			EnrollmentStorageDir:     GetEnv("APP_VOICE_ENROLLMENT_STORAGE_DIR", "./data/voice-enrollment"),
			MaxUploadBytes:           int64(GetEnvInt("APP_VOICE_MAX_UPLOAD_MB", 25)) << 20,
			EnrollmentMaxUploadBytes: int64(GetEnvInt("APP_VOICE_ENROLLMENT_MAX_UPLOAD_MB", 10)) << 20,
			EnrollmentMinDuration:    getEnvDuration("APP_VOICE_ENROLLMENT_MIN_DURATION", 2*time.Second),
			EnrollmentMaxDuration:    getEnvDuration("APP_VOICE_ENROLLMENT_MAX_DURATION", 10*time.Second),
			FFprobePath:              GetEnv("APP_VOICE_FFPROBE_PATH", "ffprobe"),
			InspectionTimeout:        getEnvDuration("APP_VOICE_INSPECTION_TIMEOUT", 15*time.Second),
			EpisodeDuration:          getEnvDuration("APP_VOICE_EPISODE_DURATION", 30*time.Second),
			EpisodeSilenceGap:        getEnvDuration("APP_VOICE_EPISODE_SILENCE_GAP", 8*time.Second),
			EpisodeMaxDuration:       getEnvDuration("APP_VOICE_EPISODE_MAX_DURATION", 2*time.Minute),
		},
		Video: VideoConfig{
			StorageDir:        GetEnv("APP_VIDEO_STORAGE_DIR", "./data/video"),
			MaxUploadBytes:    int64(GetEnvInt("APP_VIDEO_MAX_UPLOAD_MB", 250)) << 20,
			EpisodeDuration:   getEnvDuration("APP_VIDEO_EPISODE_DURATION", 30*time.Second),
			FrameInterval:     getEnvDuration("APP_VIDEO_FRAME_INTERVAL", 5*time.Second),
			MaxFrames:         GetEnvInt("APP_VIDEO_MAX_FRAMES", 12),
			FFmpegPath:        GetEnv("APP_VIDEO_FFMPEG_PATH", "ffmpeg"),
			ExtractionTimeout: getEnvDuration("APP_VIDEO_EXTRACTION_TIMEOUT", 2*time.Minute),
		},
		STT: STTConfig{
			Provider: GetEnv("APP_STT_PROVIDER", "mock"),
			BaseURL:  strings.TrimRight(GetEnv("APP_STT_BASE_URL", "https://api.openai.com/v1"), "/"),
			APIKey:   GetEnv("APP_STT_API_KEY", GetEnv("OPENAI_API_KEY", "")),
			Model:    GetEnv("APP_STT_MODEL", "gpt-4o-transcribe-diarize"),
			Language: GetEnv("APP_STT_LANGUAGE", ""),
			Prompt:   GetEnv("APP_STT_PROMPT", ""),
			Timeout:  getEnvDuration("APP_STT_TIMEOUT", 2*time.Minute),
		},
		Vision: VisionConfig{
			Provider: GetEnv("APP_VISION_PROVIDER", "mock"),
			BaseURL:  strings.TrimRight(GetEnv("APP_VISION_BASE_URL", "https://api.openai.com/v1"), "/"),
			APIKey:   GetEnv("APP_VISION_API_KEY", GetEnv("OPENAI_API_KEY", GetEnv("APP_STT_API_KEY", ""))),
			Model:    GetEnv("APP_VISION_MODEL", "gpt-4.1-mini"),
			Detail:   GetEnv("APP_VISION_DETAIL", "low"),
			Timeout:  getEnvDuration("APP_VISION_TIMEOUT", 2*time.Minute),
		},
		Memograph: MemographConfig{
			BaseURL: strings.TrimRight(GetEnv("APP_MEMOGRAPH_BASE_URL", GetEnv("MEMOGRAPH_BASE_URL", "")), "/"),
			APIKey:  GetEnv("APP_MEMOGRAPH_API_KEY", GetEnv("MEMOGRAPH_API_KEY", "")),
			JWT:     GetEnv("APP_MEMOGRAPH_JWT", GetEnv("MEMOGRAPH_JWT_TOKEN", "")),
			Timeout: getEnvDuration("APP_MEMOGRAPH_TIMEOUT", 3*time.Minute),
		},
		Worker: WorkerConfig{
			Enabled:      GetEnvBool("APP_VOICE_WORKER_ENABLED", true),
			PollInterval: getEnvDuration("APP_VOICE_WORKER_POLL_INTERVAL", time.Second),
			Concurrency:  GetEnvInt("APP_VOICE_WORKER_CONCURRENCY", 2),
			MaxAttempts:  GetEnvInt("APP_VOICE_WORKER_MAX_ATTEMPTS", 5),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	value := GetEnv(key, def.String())
	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("invalid duration for %s: %v", key, err)
		return def
	}
	return duration
}

func getEnvCSV(key, def string) []string {
	value := GetEnv(key, def)
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func (c Config) Validate() error {
	if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
		return fmt.Errorf("APP_HTTP_PORT must be between 1 and 65535")
	}
	if c.HTTP.MaxHeaderBytes < 1 {
		return fmt.Errorf("APP_HTTP_MAX_HEADER_BYTES must be positive")
	}
	if strings.TrimSpace(c.Database.URL) == "" {
		return fmt.Errorf("APP_DATABASE_URL is required")
	}
	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("APP_JWT_SECRET must be at least 32 characters")
	}
	if strings.TrimSpace(c.JWT.Issuer) == "" {
		return fmt.Errorf("APP_JWT_ISSUER must not be empty")
	}
	if c.JWT.AccessTokenTTL <= 0 {
		return fmt.Errorf("APP_JWT_ACCESS_TOKEN_TTL must be positive")
	}
	if c.Database.MaxOpenConns < 1 {
		return fmt.Errorf("APP_DB_MAX_OPEN_CONNS must be positive")
	}
	if c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("APP_DB_MAX_IDLE_CONNS must be between 0 and APP_DB_MAX_OPEN_CONNS")
	}
	if strings.TrimSpace(c.Voice.StorageDir) == "" {
		return fmt.Errorf("APP_VOICE_STORAGE_DIR must not be empty")
	}
	if c.Voice.MaxUploadBytes < 1 {
		return fmt.Errorf("APP_VOICE_MAX_UPLOAD_MB must be positive")
	}
	if c.Voice.EpisodeDuration <= 0 {
		return fmt.Errorf("APP_VOICE_EPISODE_DURATION must be positive")
	}
	if strings.TrimSpace(c.Voice.EnrollmentStorageDir) == "" ||
		c.Voice.EnrollmentMaxUploadBytes < 1 ||
		c.Voice.EnrollmentMinDuration < 2*time.Second ||
		c.Voice.EnrollmentMaxDuration < c.Voice.EnrollmentMinDuration ||
		c.Voice.EnrollmentMaxDuration > 10*time.Second ||
		strings.TrimSpace(c.Voice.FFprobePath) == "" || c.Voice.InspectionTimeout <= 0 {
		return fmt.Errorf("valid voice enrollment configuration is required")
	}
	if c.Voice.EpisodeSilenceGap <= 0 || c.Voice.EpisodeMaxDuration <= c.Voice.EpisodeSilenceGap {
		return fmt.Errorf("valid voice episode assembly configuration is required")
	}
	if strings.TrimSpace(c.Video.StorageDir) == "" {
		return fmt.Errorf("APP_VIDEO_STORAGE_DIR must not be empty")
	}
	if c.Video.MaxUploadBytes < 1 || c.Video.EpisodeDuration <= 0 ||
		c.Video.FrameInterval <= 0 || c.Video.MaxFrames < 1 ||
		strings.TrimSpace(c.Video.FFmpegPath) == "" || c.Video.ExtractionTimeout <= 0 {
		return fmt.Errorf("valid video processing configuration is required")
	}
	if c.STT.Provider != "openai" && c.STT.Provider != "mock" {
		return fmt.Errorf("APP_STT_PROVIDER must be openai or mock")
	}
	if c.STT.Provider == "openai" && strings.TrimSpace(c.STT.APIKey) == "" {
		return fmt.Errorf("APP_STT_API_KEY is required when APP_STT_PROVIDER=openai")
	}
	if strings.TrimSpace(c.STT.BaseURL) == "" || strings.TrimSpace(c.STT.Model) == "" || c.STT.Timeout <= 0 {
		return fmt.Errorf("valid STT configuration is required")
	}
	if c.Vision.Provider != "openai" && c.Vision.Provider != "mock" {
		return fmt.Errorf("APP_VISION_PROVIDER must be openai or mock")
	}
	if c.Vision.Provider == "openai" && strings.TrimSpace(c.Vision.APIKey) == "" {
		return fmt.Errorf(
			"APP_VISION_API_KEY, OPENAI_API_KEY, or APP_STT_API_KEY is required when APP_VISION_PROVIDER=openai",
		)
	}
	if strings.TrimSpace(c.Vision.BaseURL) == "" || strings.TrimSpace(c.Vision.Model) == "" ||
		c.Vision.Timeout <= 0 {
		return fmt.Errorf("valid vision configuration is required")
	}
	if c.Vision.Detail != "low" && c.Vision.Detail != "high" && c.Vision.Detail != "auto" &&
		c.Vision.Detail != "original" {
		return fmt.Errorf("APP_VISION_DETAIL must be low, high, auto, or original")
	}
	if c.Worker.Concurrency < 1 || c.Worker.MaxAttempts < 1 || c.Worker.PollInterval <= 0 {
		return fmt.Errorf("valid voice worker configuration is required")
	}
	if c.Memograph.BaseURL != "" && c.Memograph.APIKey == "" && c.Memograph.JWT == "" {
		return fmt.Errorf("APP_MEMOGRAPH_API_KEY or APP_MEMOGRAPH_JWT is required when Memograph is configured")
	}
	if c.Memograph.Timeout <= 0 {
		return fmt.Errorf("APP_MEMOGRAPH_TIMEOUT must be positive")
	}
	return nil
}
