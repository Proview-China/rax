package reviewhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	CapabilitySubmitV1         = "review.submit"
	CapabilityReadV1           = "review.read"
	CapabilityClaimV1          = "review.claim"
	CapabilityAttestV1         = "review.attest"
	CapabilityCancelV1         = "review.cancel"
	CapabilityFindingV1        = "review.finding.create"
	CapabilityFeedbackV1       = "review.behavior-feedback.create"
	CapabilityEvidenceAttachV1 = "review.evidence.attach"
)

type PrincipalV1 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	SubjectID       string        `json:"subject_id"`
	Capabilities    []string      `json:"capabilities"`
	CheckedUnixNano int64         `json:"checked_unix_nano"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (p PrincipalV1) ValidateCurrent(now time.Time) error {
	if !validPathIdentifierV1(string(p.TenantID)) || strings.TrimSpace(p.SubjectID) == "" || len(p.Capabilities) == 0 || !sort.StringsAreSorted(p.Capabilities) || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidReference, "review API principal is incomplete")
	}
	for i, capability := range p.Capabilities {
		if strings.TrimSpace(capability) == "" || (i > 0 && p.Capabilities[i-1] == capability) {
			return core.NewError(core.ErrorUnauthenticated, core.ReasonDuplicateCanonicalKey, "review API principal capability is invalid")
		}
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorUnauthenticated, core.ReasonClockRegression, "review API principal clock regressed")
	}
	if now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorUnauthenticated, core.ReasonCapabilityExpired, "review API principal expired")
	}
	return nil
}

func validPathIdentifierV1(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 512 && !strings.ContainsAny(value, "/?#\x00\r\n")
}

func (p PrincipalV1) Allows(capability string) bool {
	index := sort.SearchStrings(p.Capabilities, capability)
	return index < len(p.Capabilities) && p.Capabilities[index] == capability
}

type AuthenticatorV1 interface {
	AuthenticateReviewV1(context.Context, *http.Request) (PrincipalV1, error)
}

// StaticBearerAuthenticatorV1 is a small host composition helper. Tokens are
// hashed at construction and are never returned, logged or persisted.
type StaticBearerAuthenticatorV1 struct {
	principals map[[sha256.Size]byte]PrincipalV1
}

func NewStaticBearerAuthenticatorV1(tokens map[string]PrincipalV1) (*StaticBearerAuthenticatorV1, error) {
	if len(tokens) == 0 || len(tokens) > 1024 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review bearer token set is empty or too large")
	}
	values := make(map[[sha256.Size]byte]PrincipalV1, len(tokens))
	for token, principal := range tokens {
		if len(token) < 32 || len(token) > 4096 {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review bearer token length is invalid")
		}
		principal.Capabilities = append([]string(nil), principal.Capabilities...)
		sort.Strings(principal.Capabilities)
		digest := sha256.Sum256([]byte(token))
		if _, exists := values[digest]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review bearer token is duplicated")
		}
		values[digest] = principal
	}
	return &StaticBearerAuthenticatorV1{principals: values}, nil
}

func (a *StaticBearerAuthenticatorV1) AuthenticateReviewV1(_ context.Context, request *http.Request) (PrincipalV1, error) {
	if nilcheck.IsNil(a) || request == nil {
		return PrincipalV1{}, core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidReference, "review bearer authenticator is unavailable")
	}
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || strings.ContainsAny(header, "\r\n") {
		return PrincipalV1{}, core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidReference, "review bearer credential is missing")
	}
	token := strings.TrimPrefix(header, "Bearer ")
	digest := sha256.Sum256([]byte(token))
	principal, ok := a.principals[digest]
	if !ok {
		return PrincipalV1{}, core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidReference, "review bearer credential is invalid")
	}
	principal.Capabilities = append([]string(nil), principal.Capabilities...)
	return principal, nil
}

func TokenFingerprintV1(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:8])
}
