package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM with the given key.
// The key must be exactly 32 bytes. Returns base64-encoded ciphertext with nonce prepended.
func Encrypt(plaintext []byte, key string) ([]byte, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		// Try raw string if not base64
		keyBytes = []byte(key)
	}

	if len(keyBytes) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts base64-encoded ciphertext using AES-256-GCM.
// The key must be exactly 32 bytes. The nonce is extracted from the beginning of the ciphertext.
func Decrypt(ciphertext []byte, key string) ([]byte, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		// Try raw string if not base64
		keyBytes = []byte(key)
	}

	if len(keyBytes) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}