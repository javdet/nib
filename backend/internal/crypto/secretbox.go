package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// ErrNoKey is returned when the secrets encryption key is not configured.
var ErrNoKey = errors.New("secrets encryption key not configured")

// Cipher encrypts and decrypts secret values with AES-256-GCM.
// Ciphertext format: nonce || sealed payload.
type Cipher struct {
	gcm cipher.AEAD
}

// NewCipher builds a Cipher from a 32-byte AES-256 key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) == 0 {
		return nil, ErrNoKey
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets encryption key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	return &Cipher{gcm: gcm}, nil
}

// Encrypt seals plaintext with a random nonce prepended to the ciphertext.
func (c *Cipher) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	return c.gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt opens a nonce-prefixed ciphertext produced by Encrypt.
func (c *Cipher) Decrypt(ct []byte) (string, error) {
	nonceSize := c.gcm.NonceSize()
	if len(ct) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ct[:nonceSize], ct[nonceSize:]
	plaintext, err := c.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
