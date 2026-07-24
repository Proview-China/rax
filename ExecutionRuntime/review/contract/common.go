package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ContractVersionV1 = "praxis.review/v1"
	canonicalDomainV1 = "praxis.review"
	MaxListItemsV1    = 128
	MaxStringBytesV1  = 4096
)

type FactIdentityV1 struct {
	ContractVersion string        `json:"contract_version"`
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	CreatedUnixNano int64         `json:"created_unix_nano"`
	UpdatedUnixNano int64         `json:"updated_unix_nano"`
}

func (f FactIdentityV1) ValidateShape() error {
	if f.ContractVersion != ContractVersionV1 || blank(string(f.TenantID)) || invalidID(f.ID) || f.Revision == 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review fact identity is incomplete")
	}
	return nil
}

func ValidateExpires(created, expires int64) error {
	if created <= 0 || expires <= created {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review fact requires a bounded future expiry")
	}
	return nil
}

func ValidateNow(now time.Time, created, expires int64) error {
	if now.IsZero() || now.UnixNano() < created {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review currentness clock regressed")
	}
	if now.UnixNano() >= expires {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review fact expired")
	}
	return nil
}

func seal(discriminator string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest(canonicalDomainV1, ContractVersionV1, discriminator, value)
}

func validateSealed(discriminator string, value any, actual core.Digest) error {
	if err := actual.Validate(); err != nil {
		return err
	}
	expected, err := seal(discriminator, value)
	if err != nil {
		return err
	}
	if expected != actual {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review fact digest does not bind exact content")
	}
	return nil
}

func invalidID(value string) bool {
	return blank(value) || len(value) > 512
}

func invalidText(value string) bool {
	return blank(value) || len(value) > MaxStringBytesV1
}

func blank(value string) bool { return strings.TrimSpace(value) == "" }

func validateDigestList(values []core.Digest, allowEmpty bool) error {
	if (!allowEmpty && len(values) == 0) || len(values) > MaxListItemsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review digest list is empty or exceeds its bound")
	}
	seen := make(map[core.Digest]struct{}, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		if _, ok := seen[value]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review digest list contains a duplicate")
		}
		seen[value] = struct{}{}
	}
	return nil
}
