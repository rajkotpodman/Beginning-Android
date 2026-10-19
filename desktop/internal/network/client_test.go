package network

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rajkotpodman/beginning-android/desktop/internal/config"
	"github.com/rajkotpodman/beginning-android/desktop/internal/crypto"
)

func TestKeyExchangeAndReplayProtection(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOME", tempHome)

	cfg := &config.Config{
		PairCode: "amber-breeze-cedar-drift-echo-frost",
		RelayURL: "ws://localhost:8080/ws",
		PCName:   "TestPC",
	}

	client := NewClient(cfg)

	// Peer (Android) keypair
	peerPriv, peerPub, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate peer keypair: %v", err)
	}

	// 1. Simulate incoming key_exchange envelope
	keyExEnv := &Envelope{
		Type:       "key_exchange",
		PublicKey:  base64.StdEncoding.EncodeToString(peerPub),
		DeviceName: "Google Pixel 8",
		Platform:   "android",
	}

	peerConnectedCalled := false
	client.OnPeerConnect = func(peerName string) {
		if peerName == "Google Pixel 8" {
			peerConnectedCalled = true
		}
	}

	client.handleIncomingEnvelope(keyExEnv)

	if !peerConnectedCalled {
		t.Errorf("Expected OnPeerConnect callback to be invoked with Google Pixel 8")
	}

	// Derive peer session key
	sharedSecret, err := crypto.ComputeSharedSecret(peerPriv, client.x25519Pub)
	if err != nil {
		t.Fatalf("ComputeSharedSecret failed: %v", err)
	}
	peerSessionKey, err := crypto.DeriveSessionKey(sharedSecret, client.pairCodeHash)
	if err != nil {
		t.Fatalf("DeriveSessionKey failed: %v", err)
	}

	// 2. Send clipboard message with seq = 10
	payload, nonce, err := crypto.EncryptString(peerSessionKey, "Test clipboard message")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	validEnv := &Envelope{
		Type:    "clipboard",
		Seq:     10,
		Payload: payload,
		Nonce:   nonce,
	}
	client.handleIncomingEnvelope(validEnv)

	if client.lastIncomingSeq != 10 {
		t.Errorf("Expected lastIncomingSeq 10, got %d", client.lastIncomingSeq)
	}

	// 3. Replay attack with seq = 10 (or lower) should be rejected
	replayedEnv := &Envelope{
		Type:    "clipboard",
		Seq:     10,
		Payload: payload,
		Nonce:   nonce,
	}
	client.handleIncomingEnvelope(replayedEnv)

	olderEnv := &Envelope{
		Type:    "clipboard",
		Seq:     9,
		Payload: payload,
		Nonce:   nonce,
	}
	client.handleIncomingEnvelope(olderEnv)
	if client.lastIncomingSeq != 10 {
		t.Errorf("Expected lastIncomingSeq to remain 10, got %d", client.lastIncomingSeq)
	}
}

func TestScreenshotEnvelopeHandling(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOME", tempHome)

	cfg := &config.Config{
		PairCode: "amber-breeze-cedar-drift-echo-frost",
		RelayURL: "ws://localhost:8080/ws",
		PCName:   "TestPC",
	}

	client := NewClient(cfg)
	expectedScreenshotsDir, err := config.GetScreenshotsDir()
	if err != nil {
		t.Fatalf("Failed to get screenshots dir: %v", err)
	}

	// Send screenshot metadata with seq = 1
	meta := map[string]interface{}{
		"name": "Screenshot_20261019_100000.png",
		"size": 1024,
		"type": "screenshot",
	}
	metaBytes, _ := json.Marshal(meta)
	payload, nonce, err := crypto.Encrypt(client.getActiveKey(), metaBytes)
	if err != nil {
		t.Fatalf("Failed to encrypt metadata: %v", err)
	}

	env := &Envelope{
		Type:    "screenshot",
		Seq:     1,
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

	// Stream 64KB chunk with seq = 2
	chunkBytes := make([]byte, 64*1024)
	chunkBytes[0] = 0xFF
	chunkPayload, chunkNonce, err := crypto.Encrypt(client.getActiveKey(), chunkBytes)
	if err != nil {
		t.Fatalf("Failed to encrypt chunk: %v", err)
	}

	chunkEnv := &Envelope{
		Type:       "file_chunk",
		Seq:        2,
		ChunkIndex: 0,
		Payload:    chunkPayload,
		Nonce:      chunkNonce,
	}
	client.handleIncomingEnvelope(chunkEnv)

	// End file with seq = 3
	endEnv := &Envelope{
		Type: "file_end",
		Seq:  3,
	}
	client.handleIncomingEnvelope(endEnv)

	if client.currentFile != nil {
		t.Errorf("Expected currentFile to be closed after file_end")
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read saved screenshot: %v", err)
	}
	if len(data) != len(chunkBytes) {
		t.Errorf("Saved content length mismatch: expected %d, got %d", len(chunkBytes), len(data))
	}
}

func TestGeneralFileEnvelopeHandling(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOME", tempHome)

	cfg := &config.Config{
		PairCode: "amber-breeze-cedar-drift-echo-frost",
		RelayURL: "ws://localhost:8080/ws",
		PCName:   "TestPC",
	}

	client := NewClient(cfg)
	expectedReceivedDir, err := config.GetReceivedDir()
	if err != nil {
		t.Fatalf("Failed to get received dir: %v", err)
	}

	meta := map[string]interface{}{
		"name": "document.pdf",
		"size": 512,
	}
	metaBytes, _ := json.Marshal(meta)
	payload, nonce, err := crypto.Encrypt(client.getActiveKey(), metaBytes)
	if err != nil {
		t.Fatalf("Failed to encrypt metadata: %v", err)
	}

	env := &Envelope{
		Type:    "file_start",
		Seq:     1,
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
