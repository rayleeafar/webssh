package crypto

import (
	"testing"
)

func TestGenerateSalt(t *testing.T) {
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}
	if len(salt) == 0 {
		t.Fatal("Generated salt is empty")
	}
}

func TestDeriveKey(t *testing.T) {
	salt, _ := GenerateSalt()
	key, err := DeriveKey("password123", salt)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("Expected key length 32, got %d", len(key))
	}

	key2, _ := DeriveKey("password123", salt)
	if string(key) != string(key2) {
		t.Fatal("Same password and salt should produce same key")
	}

	key3, _ := DeriveKey("different", salt)
	if string(key) == string(key3) {
		t.Fatal("Different passwords should produce different keys")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	salt, _ := GenerateSalt()
	key, _ := DeriveKey("password123", salt)

	plaintext := "my secret ssh password"
	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if ciphertext == plaintext {
		t.Fatal("Ciphertext should not equal plaintext")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("Expected %q, got %q", plaintext, decrypted)
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	salt, _ := GenerateSalt()
	key1, _ := DeriveKey("password1", salt)
	key2, _ := DeriveKey("password2", salt)

	plaintext := "secret"
	ciphertext, _ := Encrypt(plaintext, key1)

	_, err := Decrypt(ciphertext, key2)
	if err == nil {
		t.Fatal("Decrypt with wrong key should fail")
	}
}
