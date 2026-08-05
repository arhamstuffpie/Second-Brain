package config

import (
	"testing"
	"time"
)

func TestGetEnv(t *testing.T) {
	t.Setenv("TEST_CONFIG_STRING", "configured")
	if got := GetEnv("TEST_CONFIG_STRING", "fallback"); got != "configured" {
		t.Fatalf("GetEnv() = %q, want configured", got)
	}

	t.Setenv("TEST_CONFIG_STRING", "")
	if got := GetEnv("TEST_CONFIG_STRING", "fallback"); got != "fallback" {
		t.Fatalf("GetEnv() = %q, want fallback", got)
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("TEST_CONFIG_INT", "42")
	if got := GetEnvInt("TEST_CONFIG_INT", 10); got != 42 {
		t.Fatalf("GetEnvInt() = %d, want 42", got)
	}

	t.Setenv("TEST_CONFIG_INT", "invalid")
	if got := GetEnvInt("TEST_CONFIG_INT", 10); got != 10 {
		t.Fatalf("GetEnvInt() = %d, want fallback 10", got)
	}
}

func TestGetEnvBool(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		fallback bool
		want     bool
	}{
		{name: "true", value: "true", want: true},
		{name: "one", value: "1", want: true},
		{name: "yes", value: "yes", want: true},
		{name: "false", value: "false", fallback: true, want: false},
		{name: "zero", value: "0", fallback: true, want: false},
		{name: "invalid", value: "maybe", fallback: true, want: true},
		{name: "empty", value: "", fallback: true, want: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("TEST_CONFIG_BOOL", testCase.value)
			if got := GetEnvBool("TEST_CONFIG_BOOL", testCase.fallback); got != testCase.want {
				t.Fatalf("GetEnvBool() = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestLoadUsesFallbackValues(t *testing.T) {
	t.Setenv("APP_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("APP_HTTP_PORT", "")
	t.Setenv("APP_LOG_LEVEL", "")
	t.Setenv("APP_LOG_PRETTY", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("APP_HTTP_READ_TIMEOUT", "invalid")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Port != 8181 {
		t.Fatalf("HTTP.Port = %d, want 8181", cfg.HTTP.Port)
	}
	if cfg.Log.Level != "info" {
		t.Fatalf("Log.Level = %q, want info", cfg.Log.Level)
	}
	if !cfg.Log.Pretty {
		t.Fatal("Log.Pretty = false, want development fallback true")
	}
	if cfg.HTTP.ReadTimeout != 10*time.Second {
		t.Fatalf("HTTP.ReadTimeout = %s, want 10s", cfg.HTTP.ReadTimeout)
	}
	if cfg.Video.FrameInterval != 5*time.Second || cfg.Video.MaxFrames != 12 {
		t.Fatalf("Video defaults = %+v", cfg.Video)
	}
	if cfg.Vision.Provider != "mock" || cfg.Vision.Model != "gpt-4.1-mini" {
		t.Fatalf("Vision defaults = %+v", cfg.Vision)
	}
	if cfg.Memograph.MaxConcurrentWrites != 1 {
		t.Fatalf("Memograph.MaxConcurrentWrites = %d, want 1", cfg.Memograph.MaxConcurrentWrites)
	}
}

func TestLoadUsesJSONLoggingInProductionByDefault(t *testing.T) {
	t.Setenv("APP_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_LOG_PRETTY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Log.Pretty {
		t.Fatal("Log.Pretty = true, want production fallback false")
	}
}

func TestLoadUsesConfiguredValues(t *testing.T) {
	t.Setenv("APP_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("APP_HTTP_PORT", "9090")
	t.Setenv("APP_LOG_PRETTY", "true")
	t.Setenv("APP_CORS_ALLOWED_ORIGINS", "https://one.example, https://two.example")
	t.Setenv("APP_MEMOGRAPH_MAX_CONCURRENT_WRITES", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Port != 9090 {
		t.Fatalf("HTTP.Port = %d, want 9090", cfg.HTTP.Port)
	}
	if !cfg.Log.Pretty {
		t.Fatal("Log.Pretty = false, want true")
	}
	if len(cfg.CORS.AllowedOrigins) != 2 || cfg.CORS.AllowedOrigins[1] != "https://two.example" {
		t.Fatalf("CORS.AllowedOrigins = %v", cfg.CORS.AllowedOrigins)
	}
	if cfg.Memograph.MaxConcurrentWrites != 3 {
		t.Fatalf("Memograph.MaxConcurrentWrites = %d, want 3", cfg.Memograph.MaxConcurrentWrites)
	}
}

func TestLoadRejectsShortJWTSecret(t *testing.T) {
	t.Setenv("APP_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_JWT_SECRET", "short")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
}

func TestLoadRequiresVisionKeyForOpenAI(t *testing.T) {
	t.Setenv("APP_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("APP_VISION_PROVIDER", "openai")
	t.Setenv("APP_VISION_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("APP_STT_API_KEY", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing vision API key error")
	}
}

func TestLoadUsesSTTKeyForVision(t *testing.T) {
	t.Setenv("APP_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("APP_VISION_PROVIDER", "openai")
	t.Setenv("APP_VISION_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("APP_STT_API_KEY", "shared-openai-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Vision.APIKey != "shared-openai-key" {
		t.Fatalf("Vision.APIKey = %q, want STT key fallback", cfg.Vision.APIKey)
	}
}
