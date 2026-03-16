//go:build windows

package secure

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPIProtector uses Windows DPAPI (CryptProtectData / CryptUnprotectData)
// to encrypt and decrypt data bound to the current user account.
//
// It deliberately does not persist any additional key material on disk;
// protection is delegated to the OS.
type DPAPIProtector struct{}

// NewDPAPIProtector constructs a Protector backed by Windows DPAPI.
func NewDPAPIProtector() (*DPAPIProtector, error) {
	// There is no heavy initialisation needed here; this mainly serves
	// as an explicit constructor in case we want to add validation later.
	return &DPAPIProtector{}, nil
}

// Seal encrypts plaintext using Windows DPAPI for the current user.
func (p *DPAPIProtector) Seal(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return []byte{}, nil
	}

	var in windows.DataBlob
	in.Size = uint32(len(plaintext))
	in.Data = &plaintext[0]

	var out windows.DataBlob
	// We do not pass any description, entropy or prompt structure here.
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))

	if out.Size == 0 || out.Data == nil {
		return nil, fmt.Errorf("CryptProtectData returned empty output")
	}

	// Copy the DPAPI-managed buffer into a Go-managed slice.
	ciphertext := unsafe.Slice(out.Data, out.Size)
	buf := make([]byte, len(ciphertext))
	copy(buf, ciphertext)
	return buf, nil
}

// Unseal decrypts data produced by Seal using Windows DPAPI.
func (p *DPAPIProtector) Unseal(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return []byte{}, nil
	}

	var in windows.DataBlob
	in.Size = uint32(len(ciphertext))
	in.Data = &ciphertext[0]

	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))

	if out.Size == 0 || out.Data == nil {
		return nil, fmt.Errorf("CryptUnprotectData returned empty output")
	}

	plaintextBuf := unsafe.Slice(out.Data, out.Size)
	plaintext := make([]byte, len(plaintextBuf))
	copy(plaintext, plaintextBuf)
	return plaintext, nil
}

