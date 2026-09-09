// Package backup implements the portable, passphrase-protected container used
// for vault backups.
//
// It deliberately does not use the machine's Protector. DPAPI is bound to the
// Windows user profile and the keyring to the local login, so a backup sealed
// that way would be worthless in exactly the situation a backup exists for: a
// reinstalled system, a dead disk, or a different machine. The only key that
// survives all of those is one the user carries themselves, so the container is
// keyed by a passphrase.
package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"

	"tforge/internal/vault"
)

const (
	fileMagic     = "TFBAK"
	formatVersion = uint8(1)

	kdfArgon2id = uint8(1)

	saltLen  = 16
	nonceLen = 12
	keyLen   = 32

	// headerLen covers everything before the ciphertext:
	//
	//	magic(5) version(1) kdf(1) time(4) memory(4) threads(1) saltLen(1)
	//	salt(16) nonce(12)
	headerLen = len(fileMagic) + 1 + 1 + 4 + 4 + 1 + 1 + saltLen + nonceLen
)

// Argon2id parameters used for new backups.
//
// They are written into every file, so raising them later does not invalidate
// existing backups -- Open uses whatever the file records. They are also part
// of the authenticated data, so an attacker cannot weaken them to make an
// offline attack cheaper without invalidating the tag.
const (
	defaultTime    = uint32(3)
	defaultMemory  = uint32(64 * 1024) // KiB
	defaultThreads = uint8(4)
)

// MinPassphraseLen is the shortest passphrase accepted for a new backup. A
// backup contains every secret the user has, and unlike the agent it can be
// copied and attacked offline at leisure, so a short passphrase is not a
// meaningful protection.
const MinPassphraseLen = 12

var (
	// ErrNotABackup is returned for a file that is not a TForge backup.
	ErrNotABackup = errors.New("not a TForge backup file")

	// ErrWrongPassphrase is returned when decryption fails. It cannot be
	// distinguished from a corrupted file: AEAD failure looks the same either
	// way, and claiming otherwise would be a guess.
	ErrWrongPassphrase = errors.New("wrong passphrase, or the backup file is damaged")

	// ErrPassphraseTooShort is returned by Seal.
	ErrPassphraseTooShort = fmt.Errorf("passphrase must be at least %d characters", MinPassphraseLen)
)

// UnsupportedVersionError reports a backup written in a newer format.
type UnsupportedVersionError struct {
	Found     uint8
	Supported uint8
}

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf(
		"backup is format version %d, but this build only understands up to %d; update TForge to restore it",
		e.Found, e.Supported,
	)
}

// params are the KDF settings recorded in a backup.
type params struct {
	Time    uint32
	Memory  uint32
	Threads uint8
}

// Seal encodes the vaults and encrypts them under the passphrase.
func Seal(vaults []*vault.Vault, passphrase []byte) ([]byte, error) {
	if len(passphrase) < MinPassphraseLen {
		return nil, ErrPassphraseTooShort
	}

	plaintext, err := json.MarshalIndent(vaults, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode vaults: %w", err)
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("salt: %w", err)
	}
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}

	p := params{Time: defaultTime, Memory: defaultMemory, Threads: defaultThreads}
	header := encodeHeader(p, salt, nonce)

	key := deriveKey(passphrase, salt, p)
	defer zero(key)

	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	// The header is the additional data, which authenticates the KDF
	// parameters along with the ciphertext.
	return gcm.Seal(header, nonce, plaintext, header), nil
}

// Open decrypts a backup produced by Seal.
func Open(data, passphrase []byte) ([]*vault.Vault, error) {
	if len(data) < headerLen || string(data[:len(fileMagic)]) != fileMagic {
		return nil, ErrNotABackup
	}

	version := data[len(fileMagic)]
	if version > formatVersion {
		return nil, &UnsupportedVersionError{Found: version, Supported: formatVersion}
	}
	if version == 0 {
		return nil, ErrNotABackup
	}

	p, salt, nonce, err := decodeHeader(data)
	if err != nil {
		return nil, err
	}

	header := data[:headerLen]
	ciphertext := data[headerLen:]

	key := deriveKey(passphrase, salt, p)
	defer zero(key)

	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, header)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	defer zero(plaintext)

	var vaults []*vault.Vault
	if err := json.Unmarshal(plaintext, &vaults); err != nil {
		return nil, fmt.Errorf("decode vaults: %w", err)
	}
	return vaults, nil
}

func encodeHeader(p params, salt, nonce []byte) []byte {
	hdr := make([]byte, 0, headerLen)
	hdr = append(hdr, fileMagic...)
	hdr = append(hdr, formatVersion, kdfArgon2id)
	hdr = binary.BigEndian.AppendUint32(hdr, p.Time)
	hdr = binary.BigEndian.AppendUint32(hdr, p.Memory)
	hdr = append(hdr, p.Threads, uint8(saltLen))
	hdr = append(hdr, salt...)
	hdr = append(hdr, nonce...)
	return hdr
}

func decodeHeader(data []byte) (p params, salt, nonce []byte, err error) {
	off := len(fileMagic) + 1 // magic + version

	if kdf := data[off]; kdf != kdfArgon2id {
		return params{}, nil, nil, fmt.Errorf("unsupported key derivation id %d", kdf)
	}
	off++

	p.Time = binary.BigEndian.Uint32(data[off : off+4])
	off += 4
	p.Memory = binary.BigEndian.Uint32(data[off : off+4])
	off += 4
	p.Threads = data[off]
	off++

	if got := int(data[off]); got != saltLen {
		return params{}, nil, nil, fmt.Errorf("unexpected salt length %d", got)
	}
	off++

	// Reject parameters argon2 would panic on rather than crashing on a
	// malformed file.
	if p.Time == 0 || p.Threads == 0 || p.Memory < 8 {
		return params{}, nil, nil, errors.New("backup records invalid key derivation parameters")
	}

	salt = data[off : off+saltLen]
	nonce = data[off+saltLen : off+saltLen+nonceLen]
	return p, salt, nonce, nil
}

func deriveKey(passphrase, salt []byte, p params) []byte {
	return argon2.IDKey(passphrase, salt, p.Time, p.Memory, p.Threads, keyLen)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return gcm, nil
}

// zero overwrites b. Go gives no guarantee the value was not copied elsewhere,
// but it keeps derived keys and plaintext out of memory that stays live.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
