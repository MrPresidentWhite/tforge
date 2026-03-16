//go:build !windows

package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "tforge"
	keyringUser    = "vault-master-key"
)

// KeyringProtector stores the symmetric encryption key in the OS keyring
// (e.g. Secret Service on Linux, Keychain on macOS) instead of a file on disk.
// It uses the same AES-GCM layout as SoftwareProtector so the ciphertext format
// stays compatible.
type KeyringProtector struct {
	key []byte
}

// NewKeyringProtector loads or creates a symmetric key stored in the OS keyring.
func NewKeyringProtector() (*KeyringProtector, error) {
	key, err := loadOrCreateKeyFromKeyring()
	if err != nil {
		return nil, err
	}
	return &KeyringProtector{key: key}, nil
}

func loadOrCreateKeyFromKeyring() ([]byte, error) {
	// Try to read existing key.
	if b64, err := keyring.Get(keyringService, keyringUser); err == nil {
		key, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("decode key from keyring: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("unexpected key length from keyring")
		}
		return key, nil
	}

	// Not found or inaccessible: create a new key and store it.
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(key)
	if err := keyring.Set(keyringService, keyringUser, b64); err != nil {
		return nil, fmt.Errorf("store key in keyring: %w", err)
	}
	return key, nil
}

// Seal encrypts plaintext using AES-GCM with a key stored in the keyring.
// Layout: [12 Byte Nonce][4 Byte TagLen][Ciphertext+Tag]
func (p *KeyringProtector) Seal(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	buf := make([]byte, len(nonce)+4+len(ciphertext))
	copy(buf, nonce)
	// Tag length is currently constant (gcm.Overhead()), but we keep the field
	// for forwards compatibility with the SoftwareProtector format.
	tagLen := uint32(gcm.Overhead())
	buf[len(nonce)] = byte(tagLen >> 24)
	buf[len(nonce)+1] = byte(tagLen >> 16)
	buf[len(nonce)+2] = byte(tagLen >> 8)
	buf[len(nonce)+3] = byte(tagLen)
	copy(buf[len(nonce)+4:], ciphertext)
	return buf, nil
}

// Unseal decrypts data produced by Seal.
func (p *KeyringProtector) Unseal(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize+4 {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize+4:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

