package crypto

import (
	"encoding/hex"
	"testing"
)

// validKey is a 64-hex-char string representing a valid 32-byte AES-256 key.
const validKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func setValidKey(t *testing.T) {
	t.Helper()
	resetKeyCache()
	t.Setenv("ENCRYPTION_KEY", validKey)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	setValidKey(t)

	plaintext := "hello, world! This is sensitive data."
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("round-trip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	setValidKey(t)

	plaintext := "same input twice"
	ct1, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("first Encrypt failed: %v", err)
	}

	ct2, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("second Encrypt failed: %v", err)
	}

	if ct1 == ct2 {
		t.Error("encrypting the same plaintext twice produced identical ciphertexts; expected different nonces")
	}

	// Both should still decrypt to the same plaintext.
	d1, err := Decrypt(ct1)
	if err != nil {
		t.Fatalf("Decrypt ct1 failed: %v", err)
	}
	d2, err := Decrypt(ct2)
	if err != nil {
		t.Fatalf("Decrypt ct2 failed: %v", err)
	}
	if d1 != plaintext || d2 != plaintext {
		t.Errorf("decrypted values do not match original: d1=%q, d2=%q, want=%q", d1, d2, plaintext)
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	setValidKey(t)

	// "zzzz" is not valid hex, so Decrypt should fail at hex-decode.
	_, err := Decrypt("zzzz")
	if err == nil {
		t.Fatal("expected error when decrypting invalid hex, got nil")
	}
}

func TestDecryptTruncatedCiphertext(t *testing.T) {
	setValidKey(t)

	// GCM nonce is 12 bytes = 24 hex chars. Provide fewer than that.
	shortHex := hex.EncodeToString([]byte{0x01, 0x02, 0x03})
	_, err := Decrypt(shortHex)
	if err == nil {
		t.Fatal("expected error when decrypting truncated ciphertext, got nil")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	setValidKey(t)

	plaintext := "tamper-test data"
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decode hex, flip the last byte, re-encode.
	raw, err := hex.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("hex decode failed: %v", err)
	}
	raw[len(raw)-1] ^= 0xFF
	tampered := hex.EncodeToString(raw)

	_, err = Decrypt(tampered)
	if err == nil {
		t.Fatal("expected error when decrypting tampered ciphertext, got nil")
	}
}

func TestEncryptEmptyString(t *testing.T) {
	setValidKey(t)

	encrypted, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty string failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt empty string failed: %v", err)
	}

	if decrypted != "" {
		t.Errorf("expected empty string after round-trip, got %q", decrypted)
	}
}

func TestGetKeyMissing(t *testing.T) {
	resetKeyCache()
	t.Setenv("ENCRYPTION_KEY", "")

	_, err := Encrypt("test")
	if err == nil {
		t.Fatal("expected error when ENCRYPTION_KEY is not set, got nil")
	}
}

func TestGetKeyInvalidHex(t *testing.T) {
	resetKeyCache()
	t.Setenv("ENCRYPTION_KEY", "not-valid-hex-at-all!!")

	_, err := Encrypt("test")
	if err == nil {
		t.Fatal("expected error with non-hex ENCRYPTION_KEY, got nil")
	}
}

func TestGetKeyWrongLength(t *testing.T) {
	resetKeyCache()
	// 32 hex chars = 16 bytes, but AES-256 requires 32 bytes (64 hex chars).
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	_, err := Encrypt("test")
	if err == nil {
		t.Fatal("expected error with 16-byte key, got nil")
	}
}
