package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDefaultsAndSaveLoad(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOME", tempHome)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.RelayURL != "ws://localhost:8080/ws" {
		t.Errorf("Expected default RelayURL ws://localhost:8080/ws, got %q", cfg.RelayURL)
	}
	if cfg.PairCode != "" {
		t.Errorf("Expected empty initial PairCode, got %q", cfg.PairCode)
	}

	cfg.PairCode = "alpha-bravo-charlie-delta-echo-foxtrot"
	cfg.RelayURL = "wss://relay.example.com/ws"

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loadedCfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig after save failed: %v", err)
	}

	if loadedCfg.PairCode != cfg.PairCode {
		t.Errorf("Expected PairCode %q, got %q", cfg.PairCode, loadedCfg.PairCode)
	}
	if loadedCfg.RelayURL != cfg.RelayURL {
		t.Errorf("Expected RelayURL %q, got %q", cfg.RelayURL, loadedCfg.RelayURL)
	}
}

func TestDirectoryCreation(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOME", tempHome)

	shotsDir, err := GetScreenshotsDir()
	if err != nil {
		t.Fatalf("GetScreenshotsDir failed: %v", err)
	}
	if _, err := os.Stat(shotsDir); os.IsNotExist(err) {
		t.Errorf("Screenshots dir was not created: %s", shotsDir)
	}

	rxDir, err := GetReceivedDir()
	if err != nil {
		t.Fatalf("GetReceivedDir failed: %v", err)
	}
	if _, err := os.Stat(rxDir); os.IsNotExist(err) {
		t.Errorf("Received dir was not created: %s", rxDir)
	}

	cfgFile, err := GetConfigFile()
	if err != nil {
		t.Fatalf("GetConfigFile failed: %v", err)
	}
	expectedParent := filepath.Join(tempHome, ".beginningandroid")
	if filepath.Dir(cfgFile) != expectedParent {
		t.Errorf("Expected config dir %s, got %s", expectedParent, filepath.Dir(cfgFile))
	}
}
