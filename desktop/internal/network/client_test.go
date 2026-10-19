package network

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rajkotpodman/beginning-android/desktop/internal/config"
	"github.com/rajkotpodman/beginning-android/desktop/internal/crypto"
)

func TestScreenshotEnvelopeHandling(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOME", tempHome)

	cfg := &config.Config{
		PairCode: "test-pair-code",
		RelayURL: "ws://localhost:8080/ws",
		PCName:   "TestPC",
	}

	client := NewClient(cfg)

	// Verify Screenshots directory resolution
	expectedScreenshotsDir, err := config.GetScreenshotsDir()
	if err != nil {
		t.Fatalf("Failed to get screenshots dir: %v", err)
	}

	// 1. Simulate incoming screenshot envelope with type "screenshot"
	meta := map[string]interface{}{
		"name": "Screenshot_20261019_100000.png",
		"size": 1024,
		"type": "screenshot",
	}
	metaBytes, _ := json.Marshal(meta)
	payload, nonce, err := crypto.Encrypt(client.aesKey, metaBytes)
	if err != nil {
		t.Fatalf("Failed to encrypt metadata: %v", err)
	}

	env := &Envelope{
		Type:    "screenshot",
		Payload: payload,
		Nonce:   nonce,
	}

	client.handleIncomingEnvelope(env)

	if client.currentFile == nil {
		t.Fatalf("Expected currentFile to be open for incoming screenshot")
	}

	destPath := client.currentFile.Name()
	if !strings.HasPrefix(destPath, expectedScreenshotsDir) {
		t.Errorf("Expected destination in %s, got %s", expectedScreenshotsDir, destPath)
	}

	// 2. Stream chunk
	chunkBytes := []byte("fake-screenshot-binary-data")
	chunkPayload, chunkNonce, err := crypto.Encrypt(client.aesKey, chunkBytes)
	if err != nil {
		t.Fatalf("Failed to encrypt chunk: %v", err)
	}

	chunkEnv := &Envelope{
		Type:    "file_chunk",
		Payload: chunkPayload,
		Nonce:   chunkNonce,
	}
	client.handleIncomingEnvelope(chunkEnv)

	// 3. End file
	endEnv := &Envelope{
		Type: "file_end",
	}
	client.handleIncomingEnvelope(endEnv)

	if client.currentFile != nil {
		t.Errorf("Expected currentFile to be closed after file_end")
	}

	// Verify file was written
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read saved screenshot: %v", err)
	}
	if string(data) != string(chunkBytes) {
		t.Errorf("Saved content mismatch: expected %s, got %s", string(chunkBytes), string(data))
	}
}

func TestGeneralFileEnvelopeHandling(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOME", tempHome)

	cfg := &config.Config{
		PairCode: "test-pair-code",
		RelayURL: "ws://localhost:8080/ws",
		PCName:   "TestPC",
	}

	client := NewClient(cfg)

	expectedReceivedDir, err := config.GetReceivedDir()
	if err != nil {
		t.Fatalf("Failed to get received dir: %v", err)
	}

	// Normal document file
	meta := map[string]interface{}{
		"name": "document.pdf",
		"size": 512,
	}
	metaBytes, _ := json.Marshal(meta)
	payload, nonce, err := crypto.Encrypt(client.aesKey, metaBytes)
	if err != nil {
		t.Fatalf("Failed to encrypt metadata: %v", err)
	}

	env := &Envelope{
		Type:    "file_start",
		Payload: payload,
		Nonce:   nonce,
	}

	client.handleIncomingEnvelope(env)

	if client.currentFile == nil {
		t.Fatalf("Expected currentFile to be open for incoming document")
	}

	destPath := client.currentFile.Name()
	if filepath.Dir(destPath) != expectedReceivedDir {
		t.Errorf("Expected destination in %s, got %s", expectedReceivedDir, destPath)
	}

	client.currentFile.Close()
	client.currentFile = nil
}
