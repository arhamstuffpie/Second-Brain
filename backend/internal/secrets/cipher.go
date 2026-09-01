package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// Cipher encrypts account model credentials before they are written to the
// database. A random nonce makes identical API keys produce different values.
type Cipher struct {
	aead cipher.AEAD
}

func NewCipher(secret string) (*Cipher, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("credential encryption key must be at least 32 characters")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Seal(plaintext string) (string, error) {
	if c == nil || c.aead == nil {
		return "", fmt.Errorf("credential cipher is not configured")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("create credential nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *Cipher) Open(encoded string) (string, error) {
	if c == nil || c.aead == nil {
		return "", fmt.Errorf("credential cipher is not configured")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode credential: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(sealed) < nonceSize {
		return "", fmt.Errorf("encrypted credential is invalid")
	}
	plaintext, err := c.aead.Open(nil, sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt credential: %w", err)
	}
	return string(plaintext), nil
}
