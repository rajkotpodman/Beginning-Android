package crypto

import (
	"bytes"
	"testing"
)

func TestDeriveKey(t *testing.T) {
	code1 := "apple-bravo-charlie"
	code2 := "  APPLE-BRAVO-CHARLIE  "
	code3 := "different-code"

	key1 := DeriveKey(code1)
	key2 := DeriveKey(code2)
	key3 := DeriveKey(code3)

	if len(key1) != 32 {
		t.Fatalf("Expected key length 32, got %d", len(key1))
	}
	if !bytes.Equal(key1, key2) {
		t.Errorf("DeriveKey should be case-insensitive and trim spaces")
	}
	if bytes.Equal(key1, key3) {
		t.Errorf("Different codes should produce different keys")
	}
}

func TestEncryptDecryptString(t *testing.T) {
	key := DeriveKey("test-secret-key-123")
	originalText := "Hello, Android & Desktop! 🚀 Привет 123"

	payload, nonce, err := EncryptString(key, originalText)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}
	if payload == "" || nonce == "" {
		t.Fatalf("Payload or nonce is empty")
	}

	decrypted, err := DecryptString(key, payload, nonce)
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}
	if decrypted != originalText {
		t.Errorf("Expected decrypted %q, got %q", originalText, decrypted)
	}
}

func TestEncryptDecryptBytes(t *testing.T) {
	key := DeriveKey("test-secret-bytes-key")
	originalBytes := []byte{0x00, 0xFF, 0x42, 0xAA, 0x55, 0x12, 0x34}

	payload, nonce, err := Encrypt(key, originalBytes)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(key, payload, nonce)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if !bytes.Equal(decrypted, originalBytes) {
		t.Errorf("Decrypted bytes do not match original bytes")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1 := DeriveKey("correct-key")
	key2 := DeriveKey("wrong-key")

	payload, nonce, err := EncryptString(key1, "Secret Message")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	_, err = DecryptString(key2, payload, nonce)
	if err == nil {
		t.Errorf("Expected error when decrypting with wrong key, got nil")
	}
}

func TestDecryptWithTamperedCiphertext(t *testing.T) {
	key := DeriveKey("tamper-test-key")
	payload, nonce, err := EncryptString(key, "Integrity check message")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	tamperedPayload := "A" + payload[1:]
	_, err = DecryptString(key, tamperedPayload, nonce)
	if err == nil {
		t.Errorf("Expected error when decrypting tampered payload, got nil")
	}
}

func TestDecryptWithInvalidNonce(t *testing.T) {
	key := DeriveKey("nonce-test-key")
	payload, _, err := EncryptString(key, "Test")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	_, err = DecryptString(key, payload, "AAAA") // Short nonce
	if err == nil {
		t.Errorf("Expected error for invalid nonce length")
	}
}
