// Package infrastructure implements the identity ports: argon2id password
// hashing, HMAC-signed session tokens and an in-memory login throttle.
package infrastructure

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters: OWASP's minimum recommended configuration
// (m=19 MiB, t=2, p=1). Measured on the NAS build, 64 MiB per verification
// kept ~65 MB of heap resident after each login; with a 5/min login throttle
// the lighter setting is still far beyond brute-force reach. Verification
// reads the parameters stored in each hash, so older hashes keep working.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLen    = 32
	argonSaltLen   = 16
)

// ErrInvalidHash is returned for malformed APP_PASSWORD_HASH values.
var ErrInvalidHash = errors.New("invalid argon2id hash")

// HashPassword returns an argon2id PHC string:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<key>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Argon2idVerifier verifies passwords against one PHC hash.
type Argon2idVerifier struct {
	memory  uint32
	time    uint32
	threads uint8
	salt    []byte
	key     []byte
}

// NewArgon2idVerifier parses a PHC string produced by HashPassword (or any
// compatible tool).
func NewArgon2idVerifier(phc string) (*Argon2idVerifier, error) {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return nil, fmt.Errorf("%w: unsupported version", ErrInvalidHash)
	}
	v := &Argon2idVerifier{}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &v.memory, &v.time, &v.threads); err != nil {
		return nil, fmt.Errorf("%w: parameters", ErrInvalidHash)
	}
	enc := base64.RawStdEncoding
	var err error
	if v.salt, err = enc.DecodeString(parts[4]); err != nil || len(v.salt) == 0 {
		return nil, fmt.Errorf("%w: salt", ErrInvalidHash)
	}
	if v.key, err = enc.DecodeString(parts[5]); err != nil || len(v.key) == 0 {
		return nil, fmt.Errorf("%w: key", ErrInvalidHash)
	}
	return v, nil
}

// NewArgon2idVerifierFromPlain hashes a plain password in memory (APP_PASSWORD).
func NewArgon2idVerifierFromPlain(password string) (*Argon2idVerifier, error) {
	phc, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	return NewArgon2idVerifier(phc)
}

// Verify implements application.PasswordVerifier in constant time.
func (v *Argon2idVerifier) Verify(password string) bool {
	got := argon2.IDKey([]byte(password), v.salt, v.time, v.memory, v.threads, uint32(len(v.key))) //nolint:gosec // key length is small
	return subtle.ConstantTimeCompare(got, v.key) == 1
}
