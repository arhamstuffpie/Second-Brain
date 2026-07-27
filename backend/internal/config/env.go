package config

import (
	"log"
	"os"
	"strconv"
)

// GetEnv fetches an environment variable or returns def when it is unset or empty.
func GetEnv(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

// GetEnvInt fetches an integer environment variable or returns def when it is
// unset, empty, or invalid.
func GetEnvInt(key string, def int) int {
	if value := os.Getenv(key); value != "" {
		number, err := strconv.Atoi(value)
		if err != nil {
			log.Printf("invalid int for %s: %v", key, err)
			return def
		}
		return number
	}
	return def
}

// GetEnvBool fetches a boolean environment variable or returns def when it is
// unset, empty, or invalid.
func GetEnvBool(key string, def bool) bool {
	if value := os.Getenv(key); value != "" {
		if value == "1" || value == "true" || value == "TRUE" || value == "yes" || value == "on" {
			return true
		}
		if value == "0" || value == "false" || value == "FALSE" || value == "no" || value == "off" {
			return false
		}
	}
	return def
}
