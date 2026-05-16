package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const (
	encryptionKeyLength = 32 // AES-256
	tokenLength        = 32 // 256 bits
)

// SecurityManager handles encryption and token generation
type SecurityManager struct {
	encryptionKey []byte
}

// NewSecurityManager creates a new security manager with encryption key
func NewSecurityManager() (*SecurityManager, error) {
	key, err := getOrCreateEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("error getting encryption key: %v", err)
	}
	return &SecurityManager{encryptionKey: key}, nil
}

// getOrCreateEncryptionKey gets existing key or generates a new one
func getOrCreateEncryptionKey() ([]byte, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	// Generate new key if not exists
	if config.Security.EncryptionKey == "" {
		key, err := generateEncryptionKey()
		if err != nil {
			return nil, err
		}
		config.Security.EncryptionKey = key
		if err := saveConfig(config); err != nil {
			return nil, err
		}
		return []byte(key), nil
	}

	// Decode existing key
	key, err := base64.StdEncoding.DecodeString(config.Security.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("error decoding encryption key: %v", err)
	}

	return key, nil
}

// generateEncryptionKey generates a new encryption key
func generateEncryptionKey() (string, error) {
	key := make([]byte, encryptionKeyLength)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("error generating encryption key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// GenerateToken generates a secure random token
func (s *SecurityManager) GenerateToken() (string, error) {
	token := make([]byte, tokenLength)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("error generating token: %v", err)
	}
	return base64.StdEncoding.EncodeToString(token), nil
}

// EncryptToken encrypts a token using AES-256-GCM
func (s *SecurityManager) EncryptToken(token string) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("error creating cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("error creating GCM: %v", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("error generating nonce: %v", err)
	}

	encrypted := gcm.Seal(nonce, nonce, []byte(token), nil)
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// DecryptToken decrypts an encrypted token
func (s *SecurityManager) DecryptToken(encryptedToken string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedToken)
	if err != nil {
		return "", fmt.Errorf("error decoding encrypted token: %v", err)
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("error creating cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("error creating GCM: %v", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("encrypted token too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("error decrypting token: %v", err)
	}

	return string(decrypted), nil
}

// HashToken creates a hash of a token for storage/comparison
func (s *SecurityManager) HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// ValidateTokenFormat checks if token format is valid
func ValidateTokenFormat(token string) bool {
	_, err := base64.StdEncoding.DecodeString(token)
	return err == nil && len(token) > 20
}

// InitializeSecurityConfig initializes security config if not present
func InitializeSecurityConfig() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	if config.Security.EncryptionKey == "" {
		config.Security = DefaultSecurityConfig()
		// Generate encryption key
		key, err := generateEncryptionKey()
		if err != nil {
			return err
		}
		config.Security.EncryptionKey = key
		return saveConfig(config)
	}

	return nil
}
