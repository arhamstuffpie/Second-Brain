package config

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment    string
	HTTP           HTTPConfig
	Database       DatabaseConfig
	JWT            JWTConfig
	CORS           CORSConfig
	Log            LogConfig
	Voice          VoiceConfig
	Video          VideoConfig
	STT            STTConfig
	Speaker        SpeakerEmbeddingConfig
	Face           FaceRecognitionConfig
	PersonTracking PersonTrackingConfig
	ActiveSpeaker  ActiveSpeakerConfig
	Vision         VisionConfig
	Models         ModelConfig
	Memograph      MemographConfig
	Worker         WorkerConfig
	Storage        StorageConfig
	Debug          DebugConfig
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

// SpeakerEmbeddingConfig configures persistent speaker identification. Local
// and external providers intentionally use the same HTTP contract so vectors
// from different model families are never mixed in one profile index.
type SpeakerEmbeddingConfig struct {
	Provider        string
	BaseURL         string
	APIKey          string
	Model           string
	Timeout         time.Duration
	MatchThreshold  float64
	AmbiguousMargin float64
	ProvisionalTTL  time.Duration
	MinClipDuration time.Duration
	MaxClipDuration time.Duration
}

type FaceRecognitionConfig struct {
	Provider        string
	StorageDir      string
	MaxUploadBytes  int64
	BaseURL         string
	APIKey          string
	Model           string
	Timeout         time.Duration
	MatchThreshold  float64
	AmbiguousMargin float64
	ProvisionalTTL  time.Duration
	AutoConfirm     bool
}

// PersonTrackingConfig controls the independent dense face-tracking worker.
// Its model IDs must match the models loaded by person-analyzer.
type PersonTrackingConfig struct {
	Provider       string
	BaseURL        string
	APIKey         string
	DetectorModel  string
	EmbeddingModel string
	Timeout        time.Duration
	Profile        PersonTrackingProfile
}

type PersonTrackingProfile struct {
	FPS                           float64
	ConfirmationDetections        int
	ConfirmationWindowFrames      int
	LostTimeoutSeconds            float64
	ReidentificationWindowSeconds float64
	HighConfidenceThreshold       float64
	LowConfidenceThreshold        float64
	IOUThreshold                  float64
	AppearanceThreshold           float64
	MaxGallerySamples             int
}

type ActiveSpeakerConfig struct {
	Provider                   string
	BaseURL                    string
	APIKey                     string
	Model                      string
	Timeout                    time.Duration
	AutoLink                   bool
	AutoMerge                  bool
	ScoreThreshold             float64
	MinimumMouthCoverage       float64
	MinimumTemporalCoverage    float64
	MinimumSeparatedUtterances int
	MergeEvidenceCount         int
}

type VisionConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	Detail   string
	Timeout  time.Duration
}

// ModelConfig protects account-level model credentials stored for durable
// background jobs. Keep this value stable across deploys so saved credentials
// remain decryptable.
type ModelConfig struct {
	CredentialKey string
}

type MemographConfig struct {
	BaseURL             string
	APIKey              string
	JWT                 string
	Timeout             time.Duration
	MaxConcurrentWrites int
}

type WorkerConfig struct {
	Enabled      bool
	PollInterval time.Duration
	Concurrency  int
	MaxAttempts  int
}

type StorageConfig struct{ S3Bucket, S3Prefix, S3Region string }

type DebugConfig struct {
	Enabled       bool
	AdminEmail    string
	AdminPassword string
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
			MaxFrames:         GetEnvInt("APP_VIDEO_MAX_FRAMES", 120),
			FFmpegPath:        GetEnv("APP_VIDEO_FFMPEG_PATH", "ffmpeg"),
			ExtractionTimeout: getEnvDuration("APP_VIDEO_EXTRACTION_TIMEOUT", 2*time.Minute),
		},
		STT: STTConfig{
			Provider: strings.ToLower(strings.TrimSpace(GetEnv("APP_STT_PROVIDER", "mock"))),
			BaseURL:  strings.TrimRight(GetEnv("APP_STT_BASE_URL", "https://api.openai.com/v1"), "/"),
			APIKey:   GetEnv("APP_STT_API_KEY", GetEnv("OPENAI_API_KEY", "")),
			Model:    GetEnv("APP_STT_MODEL", "gpt-4o-transcribe-diarize"),
			Language: GetEnv("APP_STT_LANGUAGE", ""),
			Prompt:   GetEnv("APP_STT_PROMPT", ""),
			Timeout:  getEnvDuration("APP_STT_TIMEOUT", 2*time.Minute),
		},
		Speaker: SpeakerEmbeddingConfig{
			Provider:        strings.ToLower(strings.TrimSpace(GetEnv("APP_SPEAKER_EMBEDDING_PROVIDER", "disabled"))),
			BaseURL:         strings.TrimRight(GetEnv("APP_SPEAKER_EMBEDDING_BASE_URL", "http://127.0.0.1:8091"), "/"),
			APIKey:          GetEnv("APP_SPEAKER_EMBEDDING_API_KEY", ""),
			Model:           GetEnv("APP_SPEAKER_EMBEDDING_MODEL", "speechbrain/spkrec-ecapa-voxceleb"),
			Timeout:         getEnvDuration("APP_SPEAKER_EMBEDDING_TIMEOUT", 30*time.Second),
			MatchThreshold:  getEnvFloat("APP_SPEAKER_MATCH_THRESHOLD", 0.62),
			AmbiguousMargin: getEnvFloat("APP_SPEAKER_AMBIGUOUS_MARGIN", 0.08),
			ProvisionalTTL:  getEnvDuration("APP_SPEAKER_PROVISIONAL_TTL", 30*24*time.Hour),
			MinClipDuration: getEnvDuration("APP_SPEAKER_MIN_CLIP_DURATION", 2*time.Second),
			MaxClipDuration: getEnvDuration("APP_SPEAKER_MAX_CLIP_DURATION", 10*time.Second),
		},
		Face: FaceRecognitionConfig{
			Provider:        strings.ToLower(strings.TrimSpace(GetEnv("APP_FACE_RECOGNITION_PROVIDER", "disabled"))),
			StorageDir:      GetEnv("APP_FACE_ENROLLMENT_STORAGE_DIR", "./data/face-enrollment"),
			MaxUploadBytes:  int64(GetEnvInt("APP_FACE_MAX_UPLOAD_MB", 10)) << 20,
			BaseURL:         strings.TrimRight(GetEnv("APP_FACE_RECOGNITION_BASE_URL", "http://127.0.0.1:8092"), "/"),
			APIKey:          GetEnv("APP_FACE_RECOGNITION_API_KEY", ""),
			Model:           GetEnv("APP_FACE_RECOGNITION_MODEL", "opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79"),
			Timeout:         getEnvDuration("APP_FACE_RECOGNITION_TIMEOUT", 10*time.Second),
			MatchThreshold:  getEnvFloat("APP_FACE_MATCH_THRESHOLD", 0.50),
			AmbiguousMargin: getEnvFloat("APP_FACE_AMBIGUOUS_MARGIN", 0.10),
			ProvisionalTTL:  getEnvDuration("APP_FACE_PROVISIONAL_TTL", 30*24*time.Hour),
			AutoConfirm:     GetEnvBool("APP_FACE_AUTO_CONFIRM", false),
		},
		PersonTracking: PersonTrackingConfig{
			Provider:       strings.ToLower(strings.TrimSpace(GetEnv("APP_PERSON_ANALYZER_PROVIDER", "disabled"))),
			BaseURL:        strings.TrimRight(GetEnv("APP_PERSON_ANALYZER_BASE_URL", "http://127.0.0.1:8094"), "/"),
			APIKey:         GetEnv("APP_PERSON_ANALYZER_API_KEY", ""),
			DetectorModel:  GetEnv("APP_PERSON_ANALYZER_DETECTOR_MODEL", "opencv/yunet-2023mar"),
			EmbeddingModel: GetEnv("APP_PERSON_ANALYZER_EMBEDDING_MODEL", "opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79"),
			Timeout:        getEnvDuration("APP_PERSON_ANALYZER_TIMEOUT", 30*time.Minute),
			Profile: PersonTrackingProfile{
				FPS:                           getEnvFloat("APP_PERSON_TRACKING_FPS", 8),
				ConfirmationDetections:        GetEnvInt("APP_PERSON_TRACKING_CONFIRMATION_DETECTIONS", 3),
				ConfirmationWindowFrames:      GetEnvInt("APP_PERSON_TRACKING_CONFIRMATION_WINDOW_FRAMES", 5),
				LostTimeoutSeconds:            getEnvFloat("APP_PERSON_TRACKING_LOST_TIMEOUT_SECONDS", 1),
				ReidentificationWindowSeconds: getEnvFloat("APP_PERSON_TRACKING_REIDENTIFICATION_WINDOW_SECONDS", 10),
				HighConfidenceThreshold:       getEnvFloat("APP_PERSON_TRACKING_HIGH_CONFIDENCE_THRESHOLD", 0.8),
				LowConfidenceThreshold:        getEnvFloat("APP_PERSON_TRACKING_LOW_CONFIDENCE_THRESHOLD", 0.35),
				IOUThreshold:                  getEnvFloat("APP_PERSON_TRACKING_IOU_THRESHOLD", 0.2),
				AppearanceThreshold:           getEnvFloat("APP_PERSON_TRACKING_APPEARANCE_THRESHOLD", 0.35),
				MaxGallerySamples:             GetEnvInt("APP_PERSON_TRACKING_MAX_GALLERY_SAMPLES", 5),
			},
		},
		ActiveSpeaker: ActiveSpeakerConfig{
			Provider:                   strings.ToLower(strings.TrimSpace(GetEnv("APP_ACTIVE_SPEAKER_PROVIDER", "disabled"))),
			BaseURL:                    strings.TrimRight(GetEnv("APP_ACTIVE_SPEAKER_BASE_URL", "http://127.0.0.1:8093"), "/"),
			APIKey:                     GetEnv("APP_ACTIVE_SPEAKER_API_KEY", ""),
			Model:                      GetEnv("APP_ACTIVE_SPEAKER_MODEL", "active-speaker-v1"),
			Timeout:                    getEnvDuration("APP_ACTIVE_SPEAKER_TIMEOUT", 2*time.Minute),
			AutoLink:                   GetEnvBool("APP_ACTIVE_SPEAKER_AUTO_LINK", false),
			AutoMerge:                  GetEnvBool("APP_PERSON_AUTO_MERGE", false),
			ScoreThreshold:             getEnvFloat("APP_ACTIVE_SPEAKER_SCORE_THRESHOLD", 0.85),
			MinimumMouthCoverage:       getEnvFloat("APP_ACTIVE_SPEAKER_MIN_MOUTH_COVERAGE", 0.75),
			MinimumTemporalCoverage:    getEnvFloat("APP_ACTIVE_SPEAKER_MIN_TEMPORAL_COVERAGE", 0.75),
			MinimumSeparatedUtterances: GetEnvInt("APP_ACTIVE_SPEAKER_MIN_UTTERANCES", 2),
			MergeEvidenceCount:         GetEnvInt("APP_PERSON_MERGE_EVIDENCE_COUNT", 3),
		},
		Vision: VisionConfig{
			Provider: GetEnv("APP_VISION_PROVIDER", "mock"),
			BaseURL:  strings.TrimRight(GetEnv("APP_VISION_BASE_URL", "https://api.openai.com/v1"), "/"),
			APIKey:   GetEnv("APP_VISION_API_KEY", GetEnv("OPENAI_API_KEY", GetEnv("APP_STT_API_KEY", ""))),
			Model:    GetEnv("APP_VISION_MODEL", "gpt-4.1-mini"),
			Detail:   GetEnv("APP_VISION_DETAIL", "low"),
			Timeout:  getEnvDuration("APP_VISION_TIMEOUT", 2*time.Minute),
		},
		Models: ModelConfig{
			CredentialKey: GetEnv("APP_MODEL_CREDENTIAL_KEY", GetEnv("APP_JWT_SECRET", "K9mP2xL7vQ4wR8tY1uI3oA5sD6fG0hJ2")),
		},
		Memograph: MemographConfig{
			BaseURL:             strings.TrimRight(GetEnv("APP_MEMOGRAPH_BASE_URL", GetEnv("MEMOGRAPH_BASE_URL", "")), "/"),
			APIKey:              GetEnv("APP_MEMOGRAPH_API_KEY", GetEnv("MEMOGRAPH_API_KEY", "")),
			JWT:                 GetEnv("APP_MEMOGRAPH_JWT", GetEnv("MEMOGRAPH_JWT_TOKEN", "")),
			Timeout:             getEnvDuration("APP_MEMOGRAPH_TIMEOUT", 3*time.Minute),
			MaxConcurrentWrites: GetEnvInt("APP_MEMOGRAPH_MAX_CONCURRENT_WRITES", 1),
		},
		Worker: WorkerConfig{
			Enabled:      GetEnvBool("APP_VOICE_WORKER_ENABLED", true),
			PollInterval: getEnvDuration("APP_VOICE_WORKER_POLL_INTERVAL", time.Second),
			Concurrency:  GetEnvInt("APP_VOICE_WORKER_CONCURRENCY", 2),
			MaxAttempts:  GetEnvInt("APP_VOICE_WORKER_MAX_ATTEMPTS", 5),
		},
		Storage: StorageConfig{S3Bucket: GetEnv("APP_S3_BUCKET", ""), S3Prefix: GetEnv("APP_S3_PREFIX", "media"), S3Region: GetEnv("AWS_REGION", "us-east-1")},
		Debug: DebugConfig{
			Enabled:       GetEnvBool("APP_PIPELINE_DEBUG_ENABLED", environment != "production"),
			AdminEmail:    GetEnv("APP_PIPELINE_DEBUG_ADMIN_EMAIL", "admin@gmail.com"),
			AdminPassword: GetEnv("APP_PIPELINE_DEBUG_ADMIN_PASSWORD", "admin@123"),
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

func getEnvFloat(key string, def float64) float64 {
	value := strings.TrimSpace(GetEnv(key, strconv.FormatFloat(def, 'f', -1, 64)))
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("invalid float for %s: %v", key, err)
		return def
	}
	return parsed
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
		c.Video.FrameInterval <= 0 || c.Video.MaxFrames < 2 ||
		strings.TrimSpace(c.Video.FFmpegPath) == "" || c.Video.ExtractionTimeout <= 0 {
		return fmt.Errorf("valid video processing configuration is required")
	}
	if strings.TrimSpace(c.STT.Provider) == "" {
		return fmt.Errorf("APP_STT_PROVIDER must not be empty")
	}
	if c.STT.Provider != "mock" && strings.TrimSpace(c.STT.APIKey) == "" {
		return fmt.Errorf("APP_STT_API_KEY is required when APP_STT_PROVIDER is not mock")
	}
	if strings.TrimSpace(c.STT.BaseURL) == "" || strings.TrimSpace(c.STT.Model) == "" || c.STT.Timeout <= 0 {
		return fmt.Errorf("valid STT configuration is required")
	}
	if c.Speaker.Provider != "disabled" && c.Speaker.Provider != "local" && c.Speaker.Provider != "external" {
		return fmt.Errorf("APP_SPEAKER_EMBEDDING_PROVIDER must be disabled, local, or external")
	}
	if c.Speaker.Provider != "disabled" {
		if strings.TrimSpace(c.Speaker.BaseURL) == "" || strings.TrimSpace(c.Speaker.Model) == "" || c.Speaker.Timeout <= 0 {
			return fmt.Errorf("valid speaker embedding configuration is required")
		}
		if c.Speaker.Provider == "external" && strings.TrimSpace(c.Speaker.APIKey) == "" {
			return fmt.Errorf("APP_SPEAKER_EMBEDDING_API_KEY is required for the external provider")
		}
		if c.Speaker.MatchThreshold <= 0 || c.Speaker.MatchThreshold > 1 ||
			c.Speaker.AmbiguousMargin < 0 || c.Speaker.AmbiguousMargin >= 1 {
			return fmt.Errorf("speaker matching threshold and ambiguity margin are invalid")
		}
		if c.Speaker.ProvisionalTTL <= 0 || c.Speaker.MinClipDuration < 2*time.Second ||
			c.Speaker.MaxClipDuration < c.Speaker.MinClipDuration || c.Speaker.MaxClipDuration > 10*time.Second {
			return fmt.Errorf("speaker clip duration must be between 2 and 10 seconds")
		}
	}
	if c.Face.Provider != "disabled" && c.Face.Provider != "local" && c.Face.Provider != "external" {
		return fmt.Errorf("APP_FACE_RECOGNITION_PROVIDER must be disabled, local, or external")
	}
	if c.Face.Provider != "disabled" {
		if strings.TrimSpace(c.Face.StorageDir) == "" || c.Face.MaxUploadBytes < 1 ||
			strings.TrimSpace(c.Face.BaseURL) == "" || strings.TrimSpace(c.Face.Model) == "" || c.Face.Timeout <= 0 {
			return fmt.Errorf("valid face recognition configuration is required")
		}
		if c.Face.Provider == "external" && strings.TrimSpace(c.Face.APIKey) == "" {
			return fmt.Errorf("APP_FACE_RECOGNITION_API_KEY is required for the external provider")
		}
		if c.Face.MatchThreshold <= 0 || c.Face.MatchThreshold > 1 ||
			c.Face.AmbiguousMargin < 0 || c.Face.AmbiguousMargin >= 1 || c.Face.ProvisionalTTL <= 0 {
			return fmt.Errorf("face matching threshold, margin, and provisional TTL are invalid")
		}
	}
	if c.PersonTracking.Provider != "disabled" && c.PersonTracking.Provider != "local" && c.PersonTracking.Provider != "external" {
		return fmt.Errorf("APP_PERSON_ANALYZER_PROVIDER must be disabled, local, or external")
	}
	if c.PersonTracking.Provider != "disabled" {
		p := c.PersonTracking.Profile
		if strings.TrimSpace(c.PersonTracking.BaseURL) == "" || strings.TrimSpace(c.PersonTracking.DetectorModel) == "" ||
			strings.TrimSpace(c.PersonTracking.EmbeddingModel) == "" || c.PersonTracking.Timeout <= 0 {
			return fmt.Errorf("valid person analyzer configuration is required")
		}
		if c.PersonTracking.Provider == "external" && strings.TrimSpace(c.PersonTracking.APIKey) == "" {
			return fmt.Errorf("APP_PERSON_ANALYZER_API_KEY is required for the external provider")
		}
		if p.FPS <= 0 || p.FPS > 30 || p.ConfirmationDetections < 2 || p.ConfirmationDetections > 10 ||
			p.ConfirmationWindowFrames < 2 || p.ConfirmationWindowFrames > 30 ||
			p.LostTimeoutSeconds <= 0 || p.LostTimeoutSeconds > 30 ||
			p.ReidentificationWindowSeconds <= 0 || p.ReidentificationWindowSeconds > 120 ||
			p.LowConfidenceThreshold < 0 || p.HighConfidenceThreshold > 1 ||
			p.LowConfidenceThreshold > p.HighConfidenceThreshold || p.IOUThreshold < 0 || p.IOUThreshold > 1 ||
			p.AppearanceThreshold < -1 || p.AppearanceThreshold > 1 || p.MaxGallerySamples < 1 || p.MaxGallerySamples > 20 {
			return fmt.Errorf("person tracking profile is invalid")
		}
	}
	if c.ActiveSpeaker.Provider != "disabled" && c.ActiveSpeaker.Provider != "local" && c.ActiveSpeaker.Provider != "external" {
		return fmt.Errorf("APP_ACTIVE_SPEAKER_PROVIDER must be disabled, local, or external")
	}
	if c.ActiveSpeaker.Provider != "disabled" {
		if strings.TrimSpace(c.ActiveSpeaker.BaseURL) == "" || strings.TrimSpace(c.ActiveSpeaker.Model) == "" ||
			c.ActiveSpeaker.Timeout <= 0 {
			return fmt.Errorf("valid active-speaker configuration is required")
		}
		if c.ActiveSpeaker.Provider == "external" && strings.TrimSpace(c.ActiveSpeaker.APIKey) == "" {
			return fmt.Errorf("APP_ACTIVE_SPEAKER_API_KEY is required for the external provider")
		}
		if c.ActiveSpeaker.ScoreThreshold <= 0 || c.ActiveSpeaker.ScoreThreshold > 1 ||
			c.ActiveSpeaker.MinimumMouthCoverage <= 0 || c.ActiveSpeaker.MinimumMouthCoverage > 1 ||
			c.ActiveSpeaker.MinimumTemporalCoverage <= 0 || c.ActiveSpeaker.MinimumTemporalCoverage > 1 ||
			c.ActiveSpeaker.MinimumSeparatedUtterances < 2 || c.ActiveSpeaker.MergeEvidenceCount < 2 {
			return fmt.Errorf("active-speaker identity thresholds are invalid")
		}
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
	if len(c.Models.CredentialKey) < 32 {
		return fmt.Errorf("APP_MODEL_CREDENTIAL_KEY must be at least 32 characters")
	}
	if c.Worker.Concurrency < 1 || c.Worker.MaxAttempts < 1 || c.Worker.PollInterval <= 0 {
		return fmt.Errorf("valid voice worker configuration is required")
	}
	if c.Memograph.BaseURL != "" && c.Memograph.APIKey == "" && c.Memograph.JWT == "" {
		return fmt.Errorf("APP_MEMOGRAPH_API_KEY or APP_MEMOGRAPH_JWT is required when Memograph is configured")
	}
	if c.Memograph.Timeout <= 0 || c.Memograph.MaxConcurrentWrites < 1 {
		return fmt.Errorf("APP_MEMOGRAPH_TIMEOUT and APP_MEMOGRAPH_MAX_CONCURRENT_WRITES must be positive")
	}
	if c.Debug.Enabled && (strings.TrimSpace(c.Debug.AdminEmail) == "" || len(c.Debug.AdminPassword) < 8) {
		return fmt.Errorf("pipeline debug admin email and password are required")
	}
	return nil
}
