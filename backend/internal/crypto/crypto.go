package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
)

// cachedKey holds the parsed encryption key, resolved once via sync.Once.
// ENCRYPTION_KEY does not change during process lifetime.
var (
	cachedKey []byte
	keyOnce   sync.Once
	keyErr    error
)

func getKey() ([]byte, error) {
	keyOnce.Do(func() {
		keyHex := os.Getenv("ENCRYPTION_KEY")
		if keyHex == "" {
			keyErr = fmt.Errorf("ENCRYPTION_KEY not set")
			return
		}
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			keyErr = fmt.Errorf("invalid ENCRYPTION_KEY: %w", err)
			return
		}
		if len(key) != 32 {
			keyErr = fmt.Errorf("ENCRYPTION_KEY must be 32 bytes (64 hex chars)")
			return
		}
		cachedKey = key
	})
	return cachedKey, keyErr
}

// resetKeyCache resets the cached encryption key so it will be re-read from
// the environment on the next call to getKey(). This exists solely for tests
// that need to exercise different ENCRYPTION_KEY values within the same process.
func resetKeyCache() {
	keyOnce = sync.Once{}
	cachedKey = nil
	keyErr = nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a hex-encoded string
func Encrypt(plaintext string) (string, error) {
	key, err := getKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a hex-encoded AES-256-GCM ciphertext and returns the plaintext
func Decrypt(encrypted string) (string, error) {
	key, err := getKey()
	if err != nil {
		return "", err
	}

	data, err := hex.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("invalid ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}
