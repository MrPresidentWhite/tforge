package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Protector kapselt die Verschlüsselung von Daten.
// Später kann hier eine TPM-basierte Implementierung hinterlegt werden.
type Protector interface {
	Seal(plaintext []byte) ([]byte, error)
	Unseal(ciphertext []byte) ([]byte, error)
}

// SoftwareProtector ist eine erste, rein Software-basierte Implementierung.
// Sie verschlüsselt Daten mit einem zufälligen AES-256 Key, der lokal in einer
// Key-Datei liegt. Diese Datei ist der logische Platzhalter für ein später
// TPM-gebundenes Secret.
type SoftwareProtector struct {
	key []byte
}

// NewSoftwareProtector lädt oder erzeugt einen symmetrischen Key in keyFile.
func NewSoftwareProtector(configDir string) (*SoftwareProtector, error) {
	if configDir == "" {
		return nil, errors.New("configDir is empty")
	}

	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	keyPath := filepath.Join(configDir, "master.key")
	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return nil, err
	}

	return &SoftwareProtector{key: key}, nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		if len(data) != 32 {
			return nil, fmt.Errorf("unexpected key length in %s", path)
		}
		return data, nil
	}

	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}
	return key, nil
}

// Seal verschlüsselt plaintext mit AES-GCM.
// Layout: [12 Byte Nonce][4 Byte TagLen][Ciphertext+Tag]
func (p *SoftwareProtector) Seal(plaintext []byte) ([]byte, error) {
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

	// Speichere Tag-Länge explizit für spätere Kompatibilität.
	buf := make([]byte, len(nonce)+4+len(ciphertext))
	copy(buf, nonce)
	binary.BigEndian.PutUint32(buf[len(nonce):len(nonce)+4], uint32(gcm.Overhead()))
	copy(buf[len(nonce)+4:], ciphertext)
	return buf, nil
}

// Unseal entschlüsselt die von Seal erzeugten Daten.
func (p *SoftwareProtector) Unseal(data []byte) ([]byte, error) {
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
		return nil, errors.New("ciphertext too short")
	}
	nonce := data[:nonceSize]
	// tagLen := binary.BigEndian.Uint32(data[nonceSize : nonceSize+4]) // reserviert
	ciphertext := data[nonceSize+4:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

