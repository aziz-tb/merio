package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

const KEKEnvVar = "LIBREDESK_SECRETS_KEK"

var (
	ErrKEKMissing = errors.New("LIBREDESK_SECRETS_KEK env var is not set")
	ErrKEKInvalid = errors.New("LIBREDESK_SECRETS_KEK is not a valid base64-encoded 32-byte key")
)

// LoadKEK reads the secrets key-encryption key from the environment.
func LoadKEK() ([]byte, error) {
	raw := os.Getenv(KEKEnvVar)
	if raw == "" {
		return nil, ErrKEKMissing
	}
	key, err := DecodeKEK(raw)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// DecodeKEK validates a base64-encoded 32-byte AES key.
func DecodeKEK(raw string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKEKInvalid, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: expected 32 bytes, got %d", ErrKEKInvalid, len(key))
	}
	return key, nil
}

// Encrypt seals plaintext with AES-GCM. A fresh random nonce is generated
// for every call, so identical plaintexts produce different ciphertexts.
func Encrypt(kek, plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt opens ciphertext produced by Encrypt.
func Decrypt(kek, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size: expected %d bytes, got %d", gcm.NonceSize(), len(nonce))
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}
