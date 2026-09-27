package main

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateTokenFormat(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{"valid base64 token", base64.StdEncoding.EncodeToString([]byte("this-is-a-long-enough-token-123456")), true},
		{"valid long token", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 30))), true},
		{"empty string", "", false},
		{"too short after decode", base64.StdEncoding.EncodeToString([]byte("short")), false},
		{"invalid base64", "!!!not-base64!!!", false},
		{"short decoded but valid base64", base64.StdEncoding.EncodeToString([]byte("12345")), false},
		// ValidateTokenFormat checks len(encoded) > 20, so 20 decoded bytes -> 28 encoded chars > 20 = valid
		{"20 decoded bytes -> 28 encoded chars > 20, valid", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 20))), true},
		{"16 decoded bytes -> 24 encoded chars > 20, valid", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 16))), true},
		{"15 decoded bytes -> 20 encoded chars == 20, invalid", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 15))), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateTokenFormat(tt.token); got != tt.want {
				t.Errorf("ValidateTokenFormat(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

func TestSecurityManager_GenerateToken(t *testing.T) {
	sm := &SecurityManager{}
	token, err := sm.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken() returned empty token")
	}
	// Should be valid base64
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("GenerateToken() returned invalid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("GenerateToken() decoded length = %d, want 32", len(decoded))
	}
}

func TestSecurityManager_GenerateToken_uniqueness(t *testing.T) {
	sm := &SecurityManager{}
	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		token, err := sm.GenerateToken()
		if err != nil {
			t.Fatalf("iteration %d: GenerateToken() error: %v", i, err)
		}
		if seen[token] {
			t.Fatalf("iteration %d: duplicate token %q", i, token)
		}
		seen[token] = true
	}
}

func TestSecurityManager_HashToken(t *testing.T) {
	sm := &SecurityManager{}
	token := "my-secret-token-12345"
	hash := sm.HashToken(token)

	// Verify it's a valid base64 SHA-256 hash
	decoded, err := base64.StdEncoding.DecodeString(hash)
	if err != nil {
		t.Fatalf("HashToken() returned invalid base64: %v", err)
	}
	if len(decoded) != sha256.Size {
		t.Fatalf("HashToken() decoded length = %d, want %d", len(decoded), sha256.Size)
	}

	// Verify deterministic
	hash2 := sm.HashToken(token)
	if hash != hash2 {
		t.Fatal("HashToken() is not deterministic")
	}

	// Different tokens should produce different hashes
	hash3 := sm.HashToken("different-token")
	if hash == hash3 {
		t.Fatal("HashToken() should produce different hashes for different inputs")
	}
}

func TestSecurityManager_EncryptDecrypt_roundTrip(t *testing.T) {
	// Use a known key for reproducible tests
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	sm := &SecurityManager{encryptionKey: key}

	tokens := []string{
		"my-api-token-12345",
		"another-secret-token",
		"a",                                   // very short
		strings.Repeat("long-token-", 100),    // very long
		"token-with-special-chars!@#$%^&*()",
	}

	for _, token := range tokens {
		t.Run("token_"+token[:min(len(token), 20)], func(t *testing.T) {
			encrypted, err := sm.EncryptToken(token)
			if err != nil {
				t.Fatalf("EncryptToken() error: %v", err)
			}
			if encrypted == "" {
				t.Fatal("EncryptToken() returned empty string")
			}
			if encrypted == token {
				t.Fatal("EncryptToken() returned plaintext (no encryption)")
			}

			decrypted, err := sm.DecryptToken(encrypted)
			if err != nil {
				t.Fatalf("DecryptToken() error: %v", err)
			}
			if decrypted != token {
				t.Fatalf("DecryptToken() = %q, want %q", decrypted, token)
			}
		})
	}
}

func TestSecurityManager_EncryptDecrypt_differentKeys(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1 // Different key

	sm1 := &SecurityManager{encryptionKey: key1}
	sm2 := &SecurityManager{encryptionKey: key2}

	token := "my-secret-token"
	encrypted, err := sm1.EncryptToken(token)
	if err != nil {
		t.Fatalf("EncryptToken() error: %v", err)
	}

	// Decrypting with a different key should fail
	_, err = sm2.DecryptToken(encrypted)
	if err == nil {
		t.Fatal("DecryptToken() with wrong key should have failed")
	}
}

func TestSecurityManager_DecryptToken_invalidInput(t *testing.T) {
	key := make([]byte, 32)
	sm := &SecurityManager{encryptionKey: key}

	tests := []struct {
		name        string
		encrypted   string
		wantErr     bool
		errContains string
	}{
		{"empty string", "", true, "decode"},
		{"invalid base64", "!!!invalid-base64!!!", true, "decode"},
		{"too short (valid base64)", base64.StdEncoding.EncodeToString([]byte("short")), true, "too short"},
		{"garbage data", base64.StdEncoding.EncodeToString([]byte("this-is-32-bytes-of-garbage!!!!")), true, "decrypt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sm.DecryptToken(tt.encrypted)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecryptToken(%q) error = %v, wantErr %v", tt.encrypted, err, tt.wantErr)
			}
		})
	}
}

func TestValidateTokenFormat_edgeCases(t *testing.T) {
	// ValidateTokenFormat checks len(encoded_token) > 20, not decoded length.
	// ceil(n/3)*4 > 20 means n >= 16 decoded bytes.
	for length := 0; length <= 20; length++ {
		input := strings.Repeat("x", length)
		encoded := base64.StdEncoding.EncodeToString([]byte(input))
		got := ValidateTokenFormat(encoded)
		expected := len(encoded) > 20
		if got != expected {
			t.Errorf("ValidateTokenFormat for %d-char decoded input (encoded=%q, len=%d) = %v, want %v",
				length, encoded, len(encoded), got, expected)
		}
	}
}

func TestGenerateEncryptionKey(t *testing.T) {
	keyStr, err := generateEncryptionKey()
	if err != nil {
		t.Fatalf("generateEncryptionKey() error: %v", err)
	}
	if keyStr == "" {
		t.Fatal("generateEncryptionKey() returned empty string")
	}
	decoded, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		t.Fatalf("generateEncryptionKey() returned invalid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("generateEncryptionKey() decoded length = %d, want 32", len(decoded))
	}
}

func TestGenerateEncryptionKey_uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 5; i++ {
		key, err := generateEncryptionKey()
		if err != nil {
			t.Fatalf("iteration %d: generateEncryptionKey() error: %v", i, err)
		}
		if seen[key] {
			t.Fatalf("iteration %d: duplicate key %q", i, key)
		}
		seen[key] = true
	}
}
