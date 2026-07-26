package contract

import (
	"fmt"
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const MaxSettledToolResultInlineBytesV2 = 64 * 1024

// SettledToolResultSourceV2 embeds the Tool-owned public projection rather than
// mirroring its fields. Runtime/Application governance remains opaque to Context.
type SettledToolResultSourceV2 struct {
	ContractVersion string                                     `json:"contract_version"`
	Projection      toolcontract.SettledToolResultProjectionV1 `json:"projection"`
	CheckedUnixNano int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                      `json:"expires_unix_nano"`
	Digest          Digest                                     `json:"digest"`
}

func (s SettledToolResultSourceV2) digestValue() (Digest, error) {
	copy := s
	copy.Digest = ""
	return DigestJSON(copy)
}

func (s SettledToolResultSourceV2) ValidateCurrent(result toolcontract.ToolResultV2, now time.Time) error {
	if ValidateContract(s.ContractVersion) != nil || now.IsZero() || s.CheckedUnixNano != now.UnixNano() || s.ExpiresUnixNano <= s.CheckedUnixNano || s.Digest.Validate() != nil {
		return fmt.Errorf("%w: settled Tool result source", ErrInvalid)
	}
	if err := s.Projection.ValidateCurrent(result, now); err != nil {
		return fmt.Errorf("%w: Tool projection current validation: %v", ErrConflict, err)
	}
	if s.ExpiresUnixNano > s.Projection.ExpiresUnixNano || s.ExpiresUnixNano > s.Projection.Inspection.ExpiresUnixNano {
		return fmt.Errorf("%w: settled Tool result source lifetime", ErrExpired)
	}
	if len(s.Projection.Inline) > MaxSettledToolResultInlineBytesV2 {
		return fmt.Errorf("%w: settled Tool inline payload", ErrLimitExceeded)
	}
	expected, err := s.digestValue()
	if err != nil || expected != s.Digest {
		return fmt.Errorf("%w: settled Tool result source digest", ErrConflict)
	}
	return nil
}

func SealSettledToolResultSourceV2(projection toolcontract.SettledToolResultProjectionV1, result toolcontract.ToolResultV2, now time.Time, notAfterUnixNano int64) (SettledToolResultSourceV2, error) {
	if now.IsZero() || notAfterUnixNano <= now.UnixNano() {
		return SettledToolResultSourceV2{}, fmt.Errorf("%w: settled Tool source deadline", ErrExpired)
	}
	if err := projection.ValidateCurrent(result, now); err != nil {
		return SettledToolResultSourceV2{}, fmt.Errorf("%w: Tool projection current validation: %v", ErrConflict, err)
	}
	expires := notAfterUnixNano
	if projection.ExpiresUnixNano < expires {
		expires = projection.ExpiresUnixNano
	}
	if projection.Inspection.ExpiresUnixNano < expires {
		expires = projection.Inspection.ExpiresUnixNano
	}
	source := SettledToolResultSourceV2{ContractVersion: Version, Projection: projection.Clone(), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	digest, err := source.digestValue()
	if err != nil {
		return SettledToolResultSourceV2{}, err
	}
	source.Digest = digest
	return source, source.ValidateCurrent(result, now)
}
