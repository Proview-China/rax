package verify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MaxWebhookBytesV1 = core.MaxCanonicalDocumentBytes

func Secret(secret []byte) error {
	if len(secret) < 16 || len(secret) > 4096 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "platform webhook secret length is invalid")
	}
	return nil
}
func Payload(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxWebhookBytesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "platform webhook payload is empty or too large")
	}
	return nil
}
func FreshMillis(value int64, now time.Time, maxSkew time.Duration) error {
	if value <= 0 || now.IsZero() || maxSkew <= 0 {
		return core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidReference, "platform webhook timestamp is invalid")
	}
	observed := time.UnixMilli(value)
	delta := now.Sub(observed)
	if delta < 0 {
		delta = -delta
	}
	if delta > maxSkew {
		return core.NewError(core.ErrorUnauthenticated, core.ReasonReviewVerdictStale, "platform webhook timestamp is outside replay window")
	}
	return nil
}
func FreshSeconds(raw string, now time.Time, maxSkew time.Duration) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidReference, "platform webhook timestamp is invalid")
	}
	if err := FreshMillis(value*1000, now, maxSkew); err != nil {
		return 0, err
	}
	return value, nil
}
func HMACSHA256(secret, payload []byte, given string, prefix string) error {
	if err := Secret(secret); err != nil {
		return err
	}
	if err := Payload(payload); err != nil {
		return err
	}
	if !strings.HasPrefix(given, prefix) {
		return core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidDigest, "platform webhook signature method is unsupported")
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(given, prefix))
	if err != nil || len(decoded) != sha256.Size {
		return core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidDigest, "platform webhook signature is malformed")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(decoded, mac.Sum(nil)) {
		return core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidDigest, "platform webhook signature mismatch")
	}
	return nil
}
