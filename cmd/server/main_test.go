package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	const loadedKey = "TEST_DOTENV_LOADED_VALUE"
	const existingKey = "TEST_DOTENV_EXISTING_VALUE"

	originalLoaded, loadedWasSet := os.LookupEnv(loadedKey)
	t.Cleanup(func() {
		if loadedWasSet {
			_ = os.Setenv(loadedKey, originalLoaded)
		} else {
			_ = os.Unsetenv(loadedKey)
		}
	})
	if err := os.Unsetenv(loadedKey); err != nil {
		t.Fatal(err)
	}
	t.Setenv(existingKey, "from-process")

	filename := filepath.Join(t.TempDir(), ".env")
	content := []byte(loadedKey + "=from-file\n" + existingKey + "=from-file\n")
	if err := os.WriteFile(filename, content, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := loadDotEnv(filename); err != nil {
		t.Fatalf("loadDotEnv() error = %v", err)
	}
	if got := os.Getenv(loadedKey); got != "from-file" {
		t.Fatalf("%s = %q, want from-file", loadedKey, got)
	}
	if got := os.Getenv(existingKey); got != "from-process" {
		t.Fatalf("%s = %q, want process environment to take precedence", existingKey, got)
	}
}

func TestLoadDotEnvAllowsMissingFile(t *testing.T) {
	if err := loadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("loadDotEnv() error = %v", err)
	}
}
