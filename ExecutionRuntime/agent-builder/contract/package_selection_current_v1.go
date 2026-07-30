package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	PackageSelectionContractVersionV1 = "praxis.agent.package-selection/v1"
	PackageSelectionRequestKindV1     = "AgentPackageSelectionRequestV1"
	PackageSelectionCurrentKindV1     = "AgentPackageSelectionCurrentV1"
	MaxPackageSelectionIDBytesV1      = 512
)

// AgentPackageSelectionCurrentRefV1 is the exact nominal current coordinate.
// It intentionally contains only identity, revision, digest and expiry.
type AgentPackageSelectionCurrentRefV1 struct {
	SelectionID     string        `json:"selection_id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (value AgentPackageSelectionCurrentRefV1) IsZero() bool {
	return value == (AgentPackageSelectionCurrentRefV1{})
}

func (value AgentPackageSelectionCurrentRefV1) Validate() error {
	if err := ValidateAgentPackageSelectionIDV1(value.SelectionID); err != nil {
		return err
	}
	if value.Revision == 0 || value.ExpiresUnixNano <= 0 {
		return invalid(core.ReasonInvalidReference, "package selection current ref is incomplete")
	}
	return value.Digest.Validate()
}

type AgentPackageSelectionRequestV1 struct {
	ContractVersion           string                            `json:"contract_version"`
	SchemaVersion             string                            `json:"schema_version"`
	ObjectKind                string                            `json:"object_kind"`
	SelectionID               string                            `json:"selection_id"`
	PackageRef                AgentPackageRefV1                 `json:"package_ref"`
	ExpectedCurrent           AgentPackageSelectionCurrentRefV1 `json:"expected_current,omitempty"`
	RequestedNotAfterUnixNano int64                             `json:"requested_not_after_unix_nano"`
	// RequestDigest protects canonical integrity only. It is not a command
	// idempotency key; only same expected + same next CAS is idempotent.
	RequestDigest core.Digest `json:"request_digest"`
}

func AgentPackageSelectionRequestDigestV1(value AgentPackageSelectionRequestV1) (core.Digest, error) {
	value = clone(value)
	value.RequestDigest = ""
	return core.CanonicalJSONDigest(
		"praxis.agent.package-selection",
		PackageSelectionContractVersionV1,
		PackageSelectionRequestKindV1,
		value,
	)
}

func SealAgentPackageSelectionRequestV1(value AgentPackageSelectionRequestV1) (AgentPackageSelectionRequestV1, error) {
	value = clone(value)
	value.ContractVersion = PackageSelectionContractVersionV1
	value.SchemaVersion = SchemaVersionV1
	value.ObjectKind = PackageSelectionRequestKindV1
	value.RequestDigest = ""
	digest, err := AgentPackageSelectionRequestDigestV1(value)
	if err != nil {
		return AgentPackageSelectionRequestV1{}, err
	}
	value.RequestDigest = digest
	if err = value.Validate(); err != nil {
		return AgentPackageSelectionRequestV1{}, err
	}
	return value, nil
}

func (value AgentPackageSelectionRequestV1) Validate() error {
	if value.ContractVersion != PackageSelectionContractVersionV1 ||
		value.SchemaVersion != SchemaVersionV1 ||
		value.ObjectKind != PackageSelectionRequestKindV1 {
		return invalid(core.ReasonInvalidState, "package selection request discriminator is invalid")
	}
	if err := ValidateAgentPackageSelectionIDV1(value.SelectionID); err != nil {
		return err
	}
	if err := value.PackageRef.Validate(); err != nil {
		return err
	}
	if !value.ExpectedCurrent.IsZero() {
		if err := value.ExpectedCurrent.Validate(); err != nil {
			return err
		}
		if value.ExpectedCurrent.SelectionID != value.SelectionID {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "package selection expected current names another selection")
		}
	}
	if value.RequestedNotAfterUnixNano <= 0 {
		return invalid(core.ReasonBindingExpired, "package selection requested expiry is required")
	}
	want, err := AgentPackageSelectionRequestDigestV1(value)
	if err != nil {
		return err
	}
	if want != value.RequestDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "package selection request digest drifted")
	}
	return nil
}

func (value AgentPackageSelectionRequestV1) ValidateCurrent(now time.Time) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "package selection clock is invalid")
	}
	if value.RequestedNotAfterUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "package selection request expired")
	}
	return nil
}

type AgentPackageSelectionCurrentV1 struct {
	ContractVersion  string                                    `json:"contract_version"`
	SchemaVersion    string                                    `json:"schema_version"`
	ObjectKind       string                                    `json:"object_kind"`
	Ref              AgentPackageSelectionCurrentRefV1         `json:"ref"`
	PackageRef       AgentPackageRefV1                         `json:"package_ref"`
	PublicationRef   assemblycontract.AssemblyPublicationRefV2 `json:"publication_ref"`
	ClosureDigest    core.Digest                               `json:"closure_digest"`
	CheckedUnixNano  int64                                     `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                     `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                               `json:"projection_digest"`
}

func AgentPackageSelectionProjectionDigestV1(value AgentPackageSelectionCurrentV1) (core.Digest, error) {
	value = clone(value)
	value.Ref.Digest = ""
	value.ProjectionDigest = ""
	return core.CanonicalJSONDigest(
		"praxis.agent.package-selection",
		PackageSelectionContractVersionV1,
		PackageSelectionCurrentKindV1,
		value,
	)
}

func SealAgentPackageSelectionCurrentV1(value AgentPackageSelectionCurrentV1) (AgentPackageSelectionCurrentV1, error) {
	value = clone(value)
	value.ContractVersion = PackageSelectionContractVersionV1
	value.SchemaVersion = SchemaVersionV1
	value.ObjectKind = PackageSelectionCurrentKindV1
	value.Ref.Digest = ""
	value.ProjectionDigest = ""
	digest, err := AgentPackageSelectionProjectionDigestV1(value)
	if err != nil {
		return AgentPackageSelectionCurrentV1{}, err
	}
	value.Ref.Digest = digest
	value.ProjectionDigest = digest
	if err = value.Validate(); err != nil {
		return AgentPackageSelectionCurrentV1{}, err
	}
	return value, nil
}

func (value AgentPackageSelectionCurrentV1) Validate() error {
	if value.ContractVersion != PackageSelectionContractVersionV1 ||
		value.SchemaVersion != SchemaVersionV1 ||
		value.ObjectKind != PackageSelectionCurrentKindV1 {
		return invalid(core.ReasonInvalidState, "package selection current discriminator is invalid")
	}
	if err := value.Ref.Validate(); err != nil {
		return err
	}
	if value.CheckedUnixNano <= 0 || value.ExpiresUnixNano <= value.CheckedUnixNano {
		return invalid(core.ReasonInvalidState, "package selection validity window is invalid")
	}
	if value.Ref.ExpiresUnixNano != value.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "package selection ref expiry differs from current")
	}
	if err := value.PackageRef.Validate(); err != nil {
		return err
	}
	if err := value.PublicationRef.Validate(); err != nil {
		return err
	}
	if err := value.ClosureDigest.Validate(); err != nil {
		return err
	}
	want, err := AgentPackageSelectionProjectionDigestV1(value)
	if err != nil {
		return err
	}
	if want != value.ProjectionDigest || value.Ref.Digest != value.ProjectionDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "package selection projection digest drifted")
	}
	return nil
}

func (value AgentPackageSelectionCurrentV1) ValidateCurrent(expected AgentPackageSelectionCurrentRefV1, now time.Time) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Ref != expected {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "package selection current exact ref drifted")
	}
	if now.IsZero() || now.UnixNano() < value.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "package selection current clock regressed")
	}
	if now.UnixNano() >= value.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "package selection current expired")
	}
	return nil
}

func (value AgentPackageSelectionCurrentV1) RefV1() AgentPackageSelectionCurrentRefV1 {
	return value.Ref
}

func CloneAgentPackageSelectionCurrentV1(value AgentPackageSelectionCurrentV1) AgentPackageSelectionCurrentV1 {
	return clone(value)
}

func ValidateAgentPackageSelectionIDV1(value string) error {
	if value != strings.TrimSpace(value) || value == "" || len(value) > MaxPackageSelectionIDBytesV1 {
		return invalid(core.ReasonInvalidReference, "package selection ID is invalid")
	}
	return nil
}
