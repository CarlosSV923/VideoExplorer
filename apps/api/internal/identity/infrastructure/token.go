package infrastructure

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/domain"
)

// HMACTokenCodec encodes sessions as "<payload>.<signature>", both base64url,
// where payload is "v1.<issuedAt unix>.<expiresAt unix>" and the signature is
// HMAC-SHA256 with SESSION_SECRET. Stateless on purpose: there is one user, and
// rotating SESSION_SECRET revokes every session.
type HMACTokenCodec struct {
	key []byte
}

// NewHMACTokenCodec builds the codec.
func NewHMACTokenCodec(secret []byte) *HMACTokenCodec {
	return &HMACTokenCodec{key: secret}
}

var b64 = base64.RawURLEncoding

// Encode implements application.TokenCodec.
func (c *HMACTokenCodec) Encode(s domain.Session) string {
	payload := fmt.Sprintf("v1.%d.%d", s.IssuedAt.Unix(), s.ExpiresAt.Unix())
	return b64.EncodeToString([]byte(payload)) + "." + b64.EncodeToString(c.sign([]byte(payload)))
}

// Decode implements application.TokenCodec.
func (c *HMACTokenCodec) Decode(token string) (domain.Session, error) {
	p64, s64, ok := strings.Cut(token, ".")
	if !ok {
		return domain.Session{}, domain.ErrInvalidSession
	}
	payload, err1 := b64.DecodeString(p64)
	sig, err2 := b64.DecodeString(s64)
	if err1 != nil || err2 != nil || !hmac.Equal(sig, c.sign(payload)) {
		return domain.Session{}, domain.ErrInvalidSession
	}

	parts := strings.Split(string(payload), ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return domain.Session{}, domain.ErrInvalidSession
	}
	iat, err1 := strconv.ParseInt(parts[1], 10, 64)
	exp, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return domain.Session{}, domain.ErrInvalidSession
	}
	return domain.Session{IssuedAt: time.Unix(iat, 0).UTC(), ExpiresAt: time.Unix(exp, 0).UTC()}, nil
}

func (c *HMACTokenCodec) sign(payload []byte) []byte {
	m := hmac.New(sha256.New, c.key)
	m.Write(payload)
	return m.Sum(nil)
}
