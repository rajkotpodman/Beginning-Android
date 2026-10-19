package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

const (
	NonceSize         = 12
	PBKDF2Iterations  = 100000
	KeyLength         = 32
	PBKDF2Salt        = "beginning-android-salt"
	HKDFInfo          = "beginning-android-session"
)

// NormalizePairCode formats the 6-word pair code
func NormalizePairCode(pairCode string) string {
	return strings.ToLower(strings.TrimSpace(pairCode))
}

// DerivePairSeed derives a 32-byte master seed from the 6-word pair code using PBKDF2-HMAC-SHA256 (100k iters)
func DerivePairSeed(pairCode string) []byte {
	cleanCode := NormalizePairCode(pairCode)
	return pbkdf2.Key([]byte(cleanCode), []byte(PBKDF2Salt), PBKDF2Iterations, KeyLength, sha256.New)
}

// PairCodeHash computes SHA256 of the normalized pair code
func PairCodeHash(pairCode string) []byte {
	cleanCode := NormalizePairCode(pairCode)
	hash := sha256.Sum256([]byte(cleanCode))
	return hash[:]
}

// GenerateX25519KeyPair generates a new X25519 private key and exports the 32-byte public key
func GenerateX25519KeyPair() (*ecdh.PrivateKey, []byte, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return priv, priv.PublicKey().Bytes(), nil
}

// ComputeSharedSecret performs X25519 Diffie-Hellman: shared_secret = X25519(my_priv, their_pub)
func ComputeSharedSecret(myPriv *ecdh.PrivateKey, theirPubBytes []byte) ([]byte, error) {
	remotePub, err := ecdh.X25519().NewPublicKey(theirPubBytes)
	if err != nil {
		return nil, err
	}
	return myPriv.ECDH(remotePub)
}

// DeriveSessionKey derives the AES-256-GCM session key: HKDF-SHA256(shared_secret, salt=paircode_hash)
func DeriveSessionKey(sharedSecret []byte, pairCodeHash []byte) ([]byte, error) {
	hkdfReader := hkdf.New(sha256.New, sharedSecret, pairCodeHash, []byte(HKDFInfo))
	sessionKey := make([]byte, KeyLength)
	if _, err := io.ReadFull(hkdfReader, sessionKey); err != nil {
		return nil, err
	}
	return sessionKey, nil
}

// Encrypt seals plaintext using AES-256-GCM with a random 12-byte nonce
func Encrypt(key []byte, plaintext []byte) (payloadBase64 string, nonceBase64 string, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}

	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), base64.StdEncoding.EncodeToString(nonce), nil
}

// EncryptString encrypts a string
func EncryptString(key []byte, text string) (payloadBase64 string, nonceBase64 string, err error) {
	return Encrypt(key, []byte(text))
}

// Decrypt opens ciphertext using AES-256-GCM
func Decrypt(key []byte, payloadBase64 string, nonceBase64 string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(payloadBase64)
	if err != nil {
		return nil, err
	}

	nonce, err := base64.StdEncoding.DecodeString(nonceBase64)
	if err != nil {
		return nil, err
	}

	if len(nonce) != NonceSize {
		return nil, errors.New("invalid nonce length")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return aesGCM.Open(nil, nonce, ciphertext, nil)
}

// DecryptString decrypts to string
func DecryptString(key []byte, payloadBase64 string, nonceBase64 string) (string, error) {
	plaintext, err := Decrypt(key, payloadBase64, nonceBase64)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
