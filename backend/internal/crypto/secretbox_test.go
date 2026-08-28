package crypto

import (
	"bytes"
	"testing"
)

func testKey() []byte {
	return bytes.Repeat([]byte{0x42}, 32)
}

func TestNewCipher(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		c, err := NewCipher(testKey())
		if err != nil {
			t.Fatalf("NewCipher() error = %v", err)
		}
		if c == nil {
			t.Fatal("NewCipher() returned nil cipher")
		}
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := NewCipher(nil)
		if err != ErrNoKey {
			t.Fatalf("NewCipher() error = %v, want %v", err, ErrNoKey)
		}
	})

	t.Run("wrong key length", func(t *testing.T) {
		_, err := NewCipher([]byte("short"))
		if err == nil {
			t.Fatal("NewCipher() expected error for short key")
		}
	})
}

func TestCipherEncryptDecryptRoundTrip(t *testing.T) {
	c, err := NewCipher(testKey())
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}

	plaintext := "super-secret-token"
	ct, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if len(ct) == 0 {
		t.Fatal("Encrypt() returned empty ciphertext")
	}

	got, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != plaintext {
		t.Fatalf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestCipherDecryptWrongKey(t *testing.T) {
	c1, err := NewCipher(testKey())
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}

	ct, err := c1.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	wrongKey := bytes.Repeat([]byte{0x01}, 32)
	c2, err := NewCipher(wrongKey)
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}

	_, err = c2.Decrypt(ct)
	if err == nil {
		t.Fatal("Decrypt() with wrong key expected error")
	}
}

func TestCipherEncryptUniqueCiphertext(t *testing.T) {
	c, err := NewCipher(testKey())
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}

	ct1, err := c.Encrypt("same-value")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	ct2, err := c.Encrypt("same-value")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if bytes.Equal(ct1, ct2) {
		t.Fatal("Encrypt() produced identical ciphertext for same plaintext")
	}
}
