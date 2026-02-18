package state

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const encryptedTokenPrefix = "enc:v1:"

type tokenCodec struct {
	aead cipher.AEAD
}

func newTokenCodec(rawKey string) (*tokenCodec, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, errors.New("empty credentials key")
	}
	keyBytes, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil {
		return nil, fmt.Errorf("decode base64 credentials key: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("credentials key must decode to 32 bytes, got %d", len(keyBytes))
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return &tokenCodec{aead: aead}, nil
}

func (c *tokenCodec) Encrypt(plaintext string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", errors.New("empty token")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	blob := append(nonce, ciphertext...)
	return encryptedTokenPrefix + base64.StdEncoding.EncodeToString(blob), nil
}

func (c *tokenCodec) Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, encryptedTokenPrefix) {
		// Backward-compatibility for previously stored plaintext rows.
		return ciphertext, nil
	}
	payload := strings.TrimPrefix(ciphertext, encryptedTokenPrefix)
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce := raw[:nonceSize]
	enc := raw[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, enc, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt ciphertext: %w", err)
	}
	return string(plaintext), nil
}
