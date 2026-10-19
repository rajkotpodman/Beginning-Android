package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const NonceSize = 12

func DeriveKey(pairCode string) []byte {
	cleanCode := strings.ToLower(strings.TrimSpace(pairCode))
	hash := sha256.Sum256([]byte(cleanCode))
	return hash[:]
}

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

func EncryptString(key []byte, text string) (payloadBase64 string, nonceBase64 string, err error) {
	return Encrypt(key, []byte(text))
}

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

func DecryptString(key []byte, payloadBase64 string, nonceBase64 string) (string, error) {
	plaintext, err := Decrypt(key, payloadBase64, nonceBase64)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
