package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEmptyConfig(t *testing.T) {
	// Create a temporary config directory
	tmpDir := t.TempDir()
	origConfigDir := configDir
	configDir = tmpDir
	defer func() { configDir = origConfigDir }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should return default cache path
	want := defaultCacheDir()
	if cfg.TypstCachePkgPath != want {
		t.Errorf("Load() = %v, want %v", cfg.TypstCachePkgPath, want)
	}
}

func TestLoadWithSavedPath(t *testing.T) {
	tmpDir := t.TempDir()
	origConfigDir := configDir
	configDir = tmpDir
	defer func() { configDir = origConfigDir }()

	// Write config file with a saved path
	savedPath := filepath.Join(tmpDir, "cache")
	os.MkdirAll(savedPath, 0755)
	configPath := filepath.Join(tmpDir, configFilename)
	os.WriteFile(configPath, []byte(`{"typstCachePkgPath":"`+savedPath+`"}`), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.TypstCachePkgPath != savedPath {
		t.Errorf("Load() = %v, want %v", cfg.TypstCachePkgPath, savedPath)
	}
}

func TestLoadWithEnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	origConfigDir := configDir
	configDir = tmpDir
	defer func() { configDir = origConfigDir }()

	envPath := filepath.Join(tmpDir, "env-cache")
	os.MkdirAll(envPath, 0755)
	t.Setenv(cachePathEnv, envPath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	absPath, _ := filepath.Abs(envPath)
	if cfg.TypstCachePkgPath != absPath {
		t.Errorf("Load() = %v, want %v", cfg.TypstCachePkgPath, absPath)
	}
}

func TestLoadWithInvalidEnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	origConfigDir := configDir
	configDir = tmpDir
	defer func() { configDir = origConfigDir }()

	t.Setenv(cachePathEnv, "/nonexistent/path")

	_, err := Load()
	if err == nil {
		t.Error("Load() expected error for invalid env var")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	origConfigDir := configDir
	configDir = tmpDir
	defer func() { configDir = origConfigDir }()

	cfg := Config{
		TypstCachePkgPath: filepath.Join(tmpDir, "saved-cache"),
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loadedCfg.TypstCachePkgPath != cfg.TypstCachePkgPath {
		t.Errorf("Load() after Save = %v, want %v", loadedCfg.TypstCachePkgPath, cfg.TypstCachePkgPath)
	}
}

func TestSaveEmptyPath(t *testing.T) {
	tmpDir := t.TempDir()
	origConfigDir := configDir
	configDir = tmpDir
	defer func() { configDir = origConfigDir }()

	cfg := Config{
		TypstCachePkgPath: "",
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should use default cache dir
	want := defaultCacheDir()
	if loadedCfg.TypstCachePkgPath != want {
		t.Errorf("Load() = %v, want %v", loadedCfg.TypstCachePkgPath, want)
	}
}
