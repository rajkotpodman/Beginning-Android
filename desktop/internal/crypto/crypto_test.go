package crypto

import (
	"bytes"
	"fmt"
	"testing"
)

func TestPBKDF2DerivePairSeed(t *testing.T) {
	code1 := "amber-breeze-cedar-drift-echo-frost"
	code2 := "  AMBER-BREEZE-CEDAR-DRIFT-ECHO-FROST  "
	code3 := "other-words-here-test-code-alpha"

	seed1 := DerivePairSeed(code1)
	seed2 := DerivePairSeed(code2)
	seed3 := DerivePairSeed(code3)

	if len(seed1) != 32 {
		t.Fatalf("Expected seed length 32, got %d", len(seed1))
	}
	if !bytes.Equal(seed1, seed2) {
		t.Errorf("DerivePairSeed should normalize whitespace and case")
	}
	if bytes.Equal(seed1, seed3) {
		t.Errorf("Different pair codes must yield different seeds")
	}

	expectedSeedHex := "87527f6425b7012ff14a6c8e85c5a7430f3f3acdfe2ed28bd99c7d04ed22f4c1"
	if fmt.Sprintf("%x", seed1) != expectedSeedHex {
		t.Errorf("Expected seed hex %s, got %x", expectedSeedHex, seed1)
	}

	expectedHashHex := "a867a68f78ea440b4d222da476f3ea5aadeac260d72c0960b12fd2187f31aa01"
	if fmt.Sprintf("%x", PairCodeHash(code1)) != expectedHashHex {
		t.Errorf("Expected hash hex %s, got %x", expectedHashHex, PairCodeHash(code1))
	}
}

func TestX25519KeyExchangeAndHKDF(t *testing.T) {
	pairCode := "amber-breeze-cedar-drift-echo-frost"
	pairHash := PairCodeHash(pairCode)

	// Device A (Android)
	privA, pubA, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair A failed: %v", err)
	}

	// Device B (Desktop)
	privB, pubB, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair B failed: %v", err)
	}

	// Diffie-Hellman Key Exchange
	sharedSecretA, err := ComputeSharedSecret(privA, pubB)
	if err != nil {
		t.Fatalf("ComputeSharedSecret A failed: %v", err)
	}

	sharedSecretB, err := ComputeSharedSecret(privB, pubA)
	if err != nil {
		t.Fatalf("ComputeSharedSecret B failed: %v", err)
	}

	if !bytes.Equal(sharedSecretA, sharedSecretB) {
		t.Fatalf("X25519 shared secrets do not match!")
	}

	// Derive Session Key via HKDF-SHA256
	sessionKeyA, err := DeriveSessionKey(sharedSecretA, pairHash)
	if err != nil {
		t.Fatalf("DeriveSessionKey A failed: %v", err)
	}

	sessionKeyB, err := DeriveSessionKey(sharedSecretB, pairHash)
	if err != nil {
		t.Fatalf("DeriveSessionKey B failed: %v", err)
	}

	if !bytes.Equal(sessionKeyA, sessionKeyB) {
		t.Fatalf("Derived HKDF session keys do not match!")
	}

	// Encrypt with Device A, Decrypt with Device B
	msg := "Confidential clipboard & screenshot payload 🚀"
	payload, nonce, err := EncryptString(sessionKeyA, msg)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	decrypted, err := DecryptString(sessionKeyB, payload, nonce)
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}

	if decrypted != msg {
		t.Fatalf("Decrypted message %q does not match original %q", decrypted, msg)
	}
}

func TestDecryptWithTamperedCiphertext(t *testing.T) {
	key := DerivePairSeed("tamper-test-code")
	payload, nonce, err := EncryptString(key, "Tamper resistance test")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	tampered := "A" + payload[1:]
	_, err = DecryptString(key, tampered, nonce)
	if err == nil {
		t.Errorf("Expected error when decrypting tampered payload")
	}
}
