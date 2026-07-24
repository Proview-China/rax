package contract

import (
	"strings"
	"time"
)

const HostStartClaimContractVersionV1 = "praxis.agent-host/host-start-claim/v1"

type HostStartOperationV1 string

const HostStartOperationStartV1 HostStartOperationV1 = "start"

// HostStartClaimV1 is the version-neutral, permanent conflict-domain fact shared
// by all governed Host facades. ExpiresUnixNano limits continuation of a start;
// it never releases HostID+StartID for another request or Host version.
type HostStartClaimV1 struct {
	ContractVersion     string               `json:"contract_version"`
	HostContractVersion string               `json:"host_contract_version"`
	HostID              string               `json:"host_id"`
	StartID             string               `json:"start_id"`
	ConfigDigest        DigestV1             `json:"config_digest"`
	DefinitionSourceRef ExactRefV1           `json:"definition_source_ref"`
	RequestedOperation  HostStartOperationV1 `json:"requested_operation"`
	CreatedUnixNano     int64                `json:"created_unix_nano"`
	ExpiresUnixNano     int64                `json:"expires_unix_nano"`
	Digest              DigestV1             `json:"digest"`
}

type HostStartClaimRefV1 struct {
	HostID          string   `json:"host_id"`
	StartID         string   `json:"start_id"`
	Revision        uint64   `json:"revision"`
	Digest          DigestV1 `json:"digest"`
	ExpiresUnixNano int64    `json:"expires_unix_nano"`
}

func (r HostStartClaimRefV1) Validate() error {
	if err := ValidateIdentifierV1("host id", r.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", r.StartID); err != nil {
		return err
	}
	if r.Revision != 1 || r.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "host_start_claim_ref_incomplete", "host start claim exact Ref is incomplete")
	}
	return r.Digest.Validate()
}

func (c HostStartClaimV1) CurrentRefV1() (HostStartClaimRefV1, error) {
	if err := c.ValidateHistoricalV1(); err != nil {
		return HostStartClaimRefV1{}, err
	}
	return HostStartClaimRefV1{HostID: c.HostID, StartID: c.StartID, Revision: 1, Digest: c.Digest, ExpiresUnixNano: c.ExpiresUnixNano}, nil
}

func (c HostStartClaimV1) ValidateHistoricalV1() error {
	if c.ContractVersion != HostStartClaimContractVersionV1 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "host start claim contract version is unsupported")
	}
	if c.HostContractVersion != ContractVersionV1 && c.HostContractVersion != ContractVersionV2 && c.HostContractVersion != HostLifecycleContractVersionV3 {
		return NewError(ErrorInvalidArgument, "host_contract_version_unsupported", "host start claim names an unsupported Host contract version")
	}
	if err := ValidateIdentifierV1("host id", c.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", c.StartID); err != nil {
		return err
	}
	if err := c.ConfigDigest.Validate(); err != nil {
		return err
	}
	if err := c.DefinitionSourceRef.Validate(); err != nil {
		return err
	}
	if c.RequestedOperation != HostStartOperationStartV1 {
		return NewError(ErrorInvalidArgument, "host_start_operation_unsupported", "host start claim operation is unsupported")
	}
	if c.CreatedUnixNano <= 0 || c.ExpiresUnixNano <= c.CreatedUnixNano {
		return NewError(ErrorInvalidArgument, "host_start_claim_window_invalid", "host start claim time window is invalid")
	}
	expected, err := c.digestV1()
	if err != nil {
		return err
	}
	if expected != c.Digest {
		return NewError(ErrorPrecondition, "host_start_claim_digest_drift", "host start claim digest drifted")
	}
	return nil
}

func (c HostStartClaimV1) ValidateCurrentV1(now time.Time) error {
	if err := c.ValidateHistoricalV1(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < c.CreatedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "host start claim was checked before its creation watermark")
	}
	if now.UnixNano() >= c.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "host_start_claim_expired", "host start claim no longer permits start continuation")
	}
	return nil
}

func (c HostStartClaimV1) digestV1() (DigestV1, error) {
	clone := c
	clone.Digest = ""
	return DigestJSONV1(struct {
		Domain string           `json:"domain"`
		Type   string           `json:"type"`
		Body   HostStartClaimV1 `json:"body"`
	}{Domain: "praxis.agent-host.host-start-claim-v1", Type: "HostStartClaimV1", Body: clone})
}

func SealHostStartClaimV1(c HostStartClaimV1) (HostStartClaimV1, error) {
	provided := c.Digest
	digest, err := c.digestV1()
	if err != nil {
		return HostStartClaimV1{}, err
	}
	if provided != "" && provided != digest {
		return HostStartClaimV1{}, NewError(ErrorConflict, "host_start_claim_digest_drift", "host start claim supplied a wrong non-zero digest")
	}
	c.Digest = digest
	if err := c.ValidateHistoricalV1(); err != nil {
		return HostStartClaimV1{}, err
	}
	return c, nil
}

func (c HostStartClaimV1) RefV1() (ExactRefV1, error) {
	if err := c.ValidateHistoricalV1(); err != nil {
		return ExactRefV1{}, err
	}
	idDigest, err := DigestJSONV1(struct {
		HostID  string `json:"host_id"`
		StartID string `json:"start_id"`
	}{HostID: c.HostID, StartID: c.StartID})
	if err != nil {
		return ExactRefV1{}, err
	}
	return ExactRefV1{Kind: "praxis.agent-host/host-start-claim", ID: "claim/" + strings.TrimPrefix(string(idDigest), "sha256:"), Revision: 1, Digest: c.Digest}, nil
}

func SameHostStartClaimV1(a, b HostStartClaimV1) bool {
	return a.HostID == b.HostID && a.StartID == b.StartID && a.Digest == b.Digest
}
