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
	Secret string
	Issuer string
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

func Load() (Config, error) {
	environment := GetEnv("APP_ENV", "development")
	cfg := Config{
		Environment: environment,
		HTTP: HTTPConfig{
			Host:              GetEnv("APP_HTTP_HOST", "0.0.0.0"),
			Port:              GetEnvInt("APP_HTTP_PORT", 8080),
			TrustedProxies:    getEnvCSV("APP_HTTP_TRUSTED_PROXIES", ""),
			ReadHeaderTimeout: getEnvDuration("APP_HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       getEnvDuration("APP_HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:      getEnvDuration("APP_HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:       getEnvDuration("APP_HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   getEnvDuration("APP_HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
			MaxHeaderBytes:    GetEnvInt("APP_HTTP_MAX_HEADER_BYTES", 1<<20),
		},
		Database: DatabaseConfig{
			URL:             GetEnv("APP_DATABASE_URL", "postgresql://postgres:mysecretpassword@localhost:5432/mysecondbrain"),
			MaxOpenConns:    GetEnvInt("APP_DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    GetEnvInt("APP_DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("APP_DB_CONN_MAX_LIFETIME", 30*time.Minute),
			ConnMaxIdleTime: getEnvDuration("APP_DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
			ConnectTimeout:  getEnvDuration("APP_DB_CONNECT_TIMEOUT", 5*time.Second),
		},
		JWT: JWTConfig{
			Secret: GetEnv("APP_JWT_SECRET", "K9mP2xL7vQ4wR8tY1uI3oA5sD6fG0hJ2"),
			Issuer: GetEnv("APP_JWT_ISSUER", "ai-second-brain"),
		},
		CORS: CORSConfig{
			AllowedOrigins: getEnvCSV("APP_CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
			AllowedMethods: getEnvCSV("APP_CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
			AllowedHeaders: getEnvCSV("APP_CORS_ALLOWED_HEADERS", "Authorization,Content-Type,X-Request-ID"),
			MaxAge:         GetEnvInt("APP_CORS_MAX_AGE_SECONDS", 600),
		},
		Log: LogConfig{
			Level:  GetEnv("APP_LOG_LEVEL", "info"),
			Pretty: GetEnvBool("APP_LOG_PRETTY", environment != "production"),
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
	if c.Database.MaxOpenConns < 1 {
		return fmt.Errorf("APP_DB_MAX_OPEN_CONNS must be positive")
	}
	if c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("APP_DB_MAX_IDLE_CONNS must be between 0 and APP_DB_MAX_OPEN_CONNS")
	}
	return nil
}
