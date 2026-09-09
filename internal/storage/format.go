package storage

import (
	"fmt"

	"tforge/internal/secure"
)

// On-disk framing for vaults.bin.
//
// The payload itself is opaque -- whatever the configured Protector produced --
// so before this header the file carried no information about itself at all.
// A failed decrypt could not be told apart from a corrupt file, and changing
// the format later would have meant guessing at what a given file contained.
//
// Layout:
//
//	0..4  magic "TFVLT"
//	5     format version (uint8)
//	6     secure.Kind of the protector that sealed the payload (uint8)
//	7..   sealed payload
//
// The header is deliberately outside the AEAD, so it is advisory rather than
// authenticated: tampering with the kind byte can only produce a misleading
// error message, never a successful decrypt. Treat it as a diagnostic hint.
const (
	fileMagic     = "TFVLT"
	formatVersion = uint8(1)
	headerLen     = len(fileMagic) + 2
)

// fileHeader is the parsed form of the framing above.
type fileHeader struct {
	Version uint8
	Kind    secure.Kind
}

// encodeHeader builds the header for a payload sealed by the given protector.
func encodeHeader(kind secure.Kind) []byte {
	hdr := make([]byte, 0, headerLen)
	hdr = append(hdr, fileMagic...)
	hdr = append(hdr, formatVersion, byte(kind))
	return hdr
}

// splitFile separates the header from the sealed payload.
//
// Files written before the header existed are still valid: they start straight
// into the sealed payload, and are reported with ok == false so the caller can
// fall back to a plain unseal. Such a legacy payload begins with either a
// random AES-GCM nonce or a DPAPI blob, so mistaking one for a header would
// require five specific random bytes plus a known version and kind. Even then
// nothing is lost -- loading never writes, and the mis-read payload simply
// fails to decrypt.
func splitFile(data []byte) (hdr fileHeader, payload []byte, ok bool) {
	if len(data) < headerLen || string(data[:len(fileMagic)]) != fileMagic {
		return fileHeader{}, data, false
	}

	hdr = fileHeader{
		Version: data[len(fileMagic)],
		Kind:    secure.Kind(data[len(fileMagic)+1]),
	}

	// Require a plausible version as part of the recognition, so a chance
	// magic match in legacy ciphertext is not treated as framing. An unknown
	// *kind* is still accepted, so that a file from a future build reports the
	// version problem rather than a confusing kind mismatch.
	if hdr.Version == 0 || hdr.Version > formatVersion+16 {
		return fileHeader{}, data, false
	}

	return hdr, data[headerLen:], true
}

// UnsupportedVersionError reports a vault file written in a format this build
// does not know how to read.
type UnsupportedVersionError struct {
	Found     uint8
	Supported uint8
}

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf(
		"vault file is format version %d, but this build only understands up to %d; update TForge",
		e.Found, e.Supported,
	)
}

// ProtectorMismatchError reports that the vault file was sealed by a different
// protector than the one currently in use. This is the common case behind an
// unreadable vault: a machine that used to fall back to a master.key file and
// now runs under DPAPI, or the other way round.
type ProtectorMismatchError struct {
	Sealed  secure.Kind
	Current secure.Kind
	Err     error
}

func (e *ProtectorMismatchError) Error() string {
	return fmt.Sprintf(
		"vault file was sealed with %s but this build is using %s; the data is intact but cannot be decrypted with the current key (%v)",
		e.Sealed, e.Current, e.Err,
	)
}

func (e *ProtectorMismatchError) Unwrap() error { return e.Err }
