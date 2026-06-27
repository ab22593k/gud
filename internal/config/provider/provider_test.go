package provider

import (
	"os"
	"path/filepath"
	"testing"

	"gud/internal/config"
)

func TestDefaultConfigPath(t *testing.T) {
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath() failed: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "gud", "config.json")
	if path != expected {
		t.Errorf("DefaultConfigPath() = %q, want %q", path, expected)
	}

	// Verify parent directory was created
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("config directory %q should exist", dir)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	p := NewFileProvider(path)

	original := config.Config{
		DetailLevel: config.DetailDetailed,
		Profile:     config.ProfileName("chemist"),
		Model:       "gemini-3.1-pro",
		Temperature: 0.3,
		Hint:        "focus on catalysis",
		History:     10,
		APIKey:      "sk-test-key",
		ACP:         config.ACPOpencode,
		WrapLine:    80,
	}

	if err := p.Save(original); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := p.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded != original {
		t.Errorf("Save/Load round-trip failed:\ngot  %+v\nwant %+v", loaded, original)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	p := NewFileProvider(path)

	_, err := p.Load()
	if err == nil {
		t.Fatal("Load() should fail for nonexistent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Load() error should be IsNotExist, got %v", err)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")
	p := NewFileProvider(path)

	if err := os.WriteFile(path, []byte("{invalid json}"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := p.Load()
	if err == nil {
		t.Fatal("Load() should fail for invalid JSON")
	}
}

func TestSaveAndLoadPartialConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.json")
	p := NewFileProvider(path)

	// Only set a few fields — simulates a partial config file
	original := config.Config{
		Model:    "gemini-3.1-flash-lite",
		ACP:      config.ACPOpencode,
		APIKey:   "sk-partial",
		WrapLine: 100,
	}

	if err := p.Save(original); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := p.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded != original {
		t.Errorf("Save/Load partial round-trip failed:\ngot  %+v\nwant %+v", loaded, original)
	}
}

func TestMultipleSaveCycles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.json")
	p := NewFileProvider(path)

	for i := 0; i < 5; i++ {
		cfg := config.Config{
			Model:   "model-v1",
			History: i,
		}
		if err := p.Save(cfg); err != nil {
			t.Fatalf("Save cycle %d failed: %v", i, err)
		}

		loaded, err := p.Load()
		if err != nil {
			t.Fatalf("Load cycle %d failed: %v", i, err)
		}
		if loaded.History != i {
			t.Errorf("Cycle %d: History = %d, want %d", i, loaded.History, i)
		}
	}
}
