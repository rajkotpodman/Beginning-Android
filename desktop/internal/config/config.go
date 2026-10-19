package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	PairCode string `json:"pair_code"`
	RelayURL string `json:"relay_url"`
	PCName   string `json:"pc_name"`
}

func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".beginningandroid")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func GetConfigFile() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func GetScreenshotsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "BeginningAndroid", "Screenshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func GetReceivedDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "BeginningAndroid", "Received")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func LoadConfig() (*Config, error) {
	path, err := GetConfigFile()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "PC"
		}
		return &Config{
			PairCode: "",
			RelayURL: "ws://localhost:8080/ws",
			PCName:   hostname,
		}, nil
	}

	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.RelayURL == "" || cfg.RelayURL == "wss://relay.beginningandroid.com/ws" {
		cfg.RelayURL = "ws://localhost:8080/ws"
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	path, err := GetConfigFile()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
