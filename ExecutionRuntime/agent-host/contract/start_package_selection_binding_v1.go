package contract

import (
	"time"

	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
)

const (
	HostStartPackageSelectionBindingContractVersionV1 = "praxis.agent-host/host-start-package-selection-binding/v1"
	HostStartPackageSelectionBindingObjectKindV1      = "praxis.agent-host/HostStartPackageSelectionBindingV1"
)

// HostStartPackageSelectionBindingRefV1 is the exact, create-once coordinate
// for the Host-owned association between one Start Claim and one Builder
// Package Selection.
type HostStartPackageSelectionBindingRefV1 struct {
	HostID          string              `json:"host_id"`
	StartID         string              `json:"start_id"`
	ClaimRef        HostStartClaimRefV1 `json:"claim_ref"`
	Revision        uint64              `json:"revision"`
	ExpiresUnixNano int64               `json:"expires_unix_nano"`
	Digest          DigestV1            `json:"digest"`
}

func (ref HostStartPackageSelectionBindingRefV1) Validate() error {
	if err := ValidateIdentifierV1("host id", ref.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", ref.StartID); err != nil {
		return err
	}
	if err := ref.ClaimRef.Validate(); err != nil {
		return err
	}
	if ref.HostID != ref.ClaimRef.HostID || ref.StartID != ref.ClaimRef.StartID {
		return NewError(ErrorConflict, "host_start_package_selection_binding_ref_claim_drift", "HostStart Package Selection binding Ref names another Claim")
	}
	if ref.Revision != 1 || ref.ExpiresUnixNano <= 0 || ref.ExpiresUnixNano > ref.ClaimRef.ExpiresUnixNano {
		return NewError(ErrorInvalidArgument, "host_start_package_selection_binding_ref_incomplete", "HostStart Package Selection binding Ref is incomplete")
	}
	return ref.Digest.Validate()
}

// HostStartPackageSelectionBindingV1 is an immutable Host-owned association.
// It copies no Package or Publication body and grants no Builder authority.
type HostStartPackageSelectionBindingV1 struct {
	ContractVersion              string                                            `json:"contract_version"`
	ObjectKind                   string                                            `json:"object_kind"`
	Ref                          HostStartPackageSelectionBindingRefV1             `json:"ref"`
	ClaimRef                     HostStartClaimRefV1                               `json:"claim_ref"`
	ClaimInputBindingDigest      DigestV1                                          `json:"claim_input_binding_digest"`
	DeploymentCurrentRef         HostDeploymentCurrentRefV2                        `json:"deployment_current_ref"`
	PackageSelectionRef          buildercontract.AgentPackageSelectionCurrentRefV1 `json:"package_selection_ref"`
	VerifiedPackageClosureDigest DigestV1                                          `json:"verified_package_closure_digest"`
	CreatedUnixNano              int64                                             `json:"created_unix_nano"`
	ExpiresUnixNano              int64                                             `json:"expires_unix_nano"`
	BindingDigest                DigestV1                                          `json:"binding_digest"`
}

func (binding HostStartPackageSelectionBindingV1) digestV1() (DigestV1, error) {
	binding.Ref.Digest = ""
	binding.BindingDigest = ""
	return DigestJSONV1(struct {
		Domain string                             `json:"domain"`
		Type   string                             `json:"type"`
		Body   HostStartPackageSelectionBindingV1 `json:"body"`
	}{
		Domain: "praxis.agent-host.host-start-package-selection-binding-v1",
		Type:   HostStartPackageSelectionBindingObjectKindV1,
		Body:   binding,
	})
}

func SealHostStartPackageSelectionBindingV1(
	binding HostStartPackageSelectionBindingV1,
) (HostStartPackageSelectionBindingV1, error) {
	if binding.ContractVersion != "" && binding.ContractVersion != HostStartPackageSelectionBindingContractVersionV1 {
		return HostStartPackageSelectionBindingV1{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostStart Package Selection binding contract version drifted")
	}
	if binding.ObjectKind != "" && binding.ObjectKind != HostStartPackageSelectionBindingObjectKindV1 {
		return HostStartPackageSelectionBindingV1{}, NewError(ErrorInvalidArgument, "object_kind_mismatch", "HostStart Package Selection binding object kind drifted")
	}
	binding.ContractVersion = HostStartPackageSelectionBindingContractVersionV1
	binding.ObjectKind = HostStartPackageSelectionBindingObjectKindV1
	binding.Ref.HostID = binding.ClaimRef.HostID
	binding.Ref.StartID = binding.ClaimRef.StartID
	binding.Ref.ClaimRef = binding.ClaimRef
	binding.Ref.Revision = 1
	binding.ExpiresUnixNano = minHostStartPackageSelectionExpiryV1(
		binding.ClaimRef.ExpiresUnixNano,
		binding.DeploymentCurrentRef.ExpiresUnixNano,
		binding.PackageSelectionRef.ExpiresUnixNano,
	)
	binding.Ref.ExpiresUnixNano = binding.ExpiresUnixNano
	providedRefDigest := binding.Ref.Digest
	providedBindingDigest := binding.BindingDigest
	binding.Ref.Digest = ""
	binding.BindingDigest = ""
	digest, err := binding.digestV1()
	if err != nil {
		return HostStartPackageSelectionBindingV1{}, err
	}
	if (providedRefDigest != "" && providedRefDigest != digest) ||
		(providedBindingDigest != "" && providedBindingDigest != digest) {
		return HostStartPackageSelectionBindingV1{}, NewError(ErrorConflict, "host_start_package_selection_binding_digest_drift", "HostStart Package Selection binding supplied a wrong non-zero digest")
	}
	binding.Ref.Digest = digest
	binding.BindingDigest = digest
	return binding, binding.ValidateHistoricalV1()
}

func NewHostStartPackageSelectionBindingV1(
	claim HostStartClaimV1,
	input HostStartClaimInputBindingV3,
	deployment HostDeploymentCurrentV2,
	selection buildercontract.AgentPackageSelectionCurrentV1,
	createdUnixNano int64,
) (HostStartPackageSelectionBindingV1, error) {
	if err := claim.ValidateHistoricalV1(); err != nil {
		return HostStartPackageSelectionBindingV1{}, err
	}
	if err := input.ValidateV3(); err != nil {
		return HostStartPackageSelectionBindingV1{}, err
	}
	if err := deployment.ValidateHistoricalV2(); err != nil {
		return HostStartPackageSelectionBindingV1{}, err
	}
	if err := selection.Validate(); err != nil {
		return HostStartPackageSelectionBindingV1{}, err
	}
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		return HostStartPackageSelectionBindingV1{}, err
	}
	binding, err := SealHostStartPackageSelectionBindingV1(HostStartPackageSelectionBindingV1{
		ClaimRef:                     claimRef,
		ClaimInputBindingDigest:      input.BindingDigest,
		DeploymentCurrentRef:         deployment.Ref,
		PackageSelectionRef:          selection.Ref,
		VerifiedPackageClosureDigest: DigestV1(selection.ClosureDigest),
		CreatedUnixNano:              createdUnixNano,
	})
	if err != nil {
		return HostStartPackageSelectionBindingV1{}, err
	}
	if err = binding.ValidateAgainstClaimInputV1(claim, input); err != nil {
		return HostStartPackageSelectionBindingV1{}, err
	}
	return binding, nil
}

func (binding HostStartPackageSelectionBindingV1) ValidateHistoricalV1() error {
	if binding.ContractVersion != HostStartPackageSelectionBindingContractVersionV1 ||
		binding.ObjectKind != HostStartPackageSelectionBindingObjectKindV1 {
		return NewError(ErrorInvalidArgument, "host_start_package_selection_binding_contract_invalid", "HostStart Package Selection binding discriminator is unsupported")
	}
	if err := binding.Ref.Validate(); err != nil {
		return err
	}
	if err := binding.ClaimRef.Validate(); err != nil {
		return err
	}
	if err := binding.ClaimInputBindingDigest.Validate(); err != nil {
		return err
	}
	if err := binding.DeploymentCurrentRef.Validate(); err != nil {
		return err
	}
	if err := binding.PackageSelectionRef.Validate(); err != nil {
		return err
	}
	if err := binding.VerifiedPackageClosureDigest.Validate(); err != nil {
		return err
	}
	if binding.Ref.ClaimRef != binding.ClaimRef ||
		binding.Ref.HostID != binding.ClaimRef.HostID ||
		binding.Ref.StartID != binding.ClaimRef.StartID ||
		binding.DeploymentCurrentRef.HostID != binding.ClaimRef.HostID {
		return NewError(ErrorConflict, "host_start_package_selection_binding_coordinate_drift", "HostStart Package Selection binding coordinates drifted")
	}
	if binding.DeploymentCurrentRef.PackageSelectionRef != binding.PackageSelectionRef {
		return NewError(ErrorConflict, "host_start_package_selection_binding_selection_drift", "Host deployment and Package Selection exact Refs differ")
	}
	expectedExpiry := minHostStartPackageSelectionExpiryV1(
		binding.ClaimRef.ExpiresUnixNano,
		binding.DeploymentCurrentRef.ExpiresUnixNano,
		binding.PackageSelectionRef.ExpiresUnixNano,
	)
	if binding.CreatedUnixNano <= 0 ||
		binding.CreatedUnixNano >= binding.ExpiresUnixNano ||
		binding.ExpiresUnixNano != expectedExpiry ||
		binding.Ref.ExpiresUnixNano != binding.ExpiresUnixNano {
		return NewError(ErrorConflict, "host_start_package_selection_binding_window_drift", "HostStart Package Selection binding validity window drifted")
	}
	expectedDigest, err := binding.digestV1()
	if err != nil || expectedDigest != binding.BindingDigest ||
		binding.Ref.Digest != binding.BindingDigest {
		return NewError(ErrorConflict, "host_start_package_selection_binding_digest_drift", "HostStart Package Selection binding digest drifted")
	}
	return nil
}

func (binding HostStartPackageSelectionBindingV1) ValidateCurrentV1(
	expected HostStartPackageSelectionBindingRefV1,
	now time.Time,
) error {
	if err := binding.ValidateHistoricalV1(); err != nil {
		return err
	}
	if binding.Ref != expected {
		return NewError(ErrorConflict, "host_start_package_selection_binding_ref_drift", "HostStart Package Selection binding exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < binding.CreatedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "HostStart Package Selection binding clock regressed")
	}
	if now.UnixNano() >= binding.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "host_start_package_selection_binding_expired", "HostStart Package Selection binding expired")
	}
	return nil
}

func (binding HostStartPackageSelectionBindingV1) ValidateAgainstClaimInputV1(
	claim HostStartClaimV1,
	input HostStartClaimInputBindingV3,
) error {
	if err := binding.ValidateHistoricalV1(); err != nil {
		return err
	}
	if err := claim.ValidateHistoricalV1(); err != nil {
		return err
	}
	if err := input.ValidateV3(); err != nil {
		return err
	}
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		return err
	}
	v1 := input.Input.DeploymentCurrentRef
	v2 := binding.DeploymentCurrentRef
	if binding.ClaimRef != claimRef ||
		input.ClaimRef != claimRef ||
		binding.ClaimInputBindingDigest != input.BindingDigest ||
		v1.HostID != v2.HostID ||
		v1.DeploymentID != v2.DeploymentID ||
		v1.Revision != v2.Revision ||
		v1.BootstrapDigest != v2.BootstrapDigest ||
		v2.ExpiresUnixNano > v1.ExpiresUnixNano {
		return NewError(ErrorConflict, "host_start_package_selection_binding_splice", "HostStart Claim, InputV3 and deployment/selection association do not form one exact closure")
	}
	return nil
}

func minHostStartPackageSelectionExpiryV1(values ...int64) int64 {
	var minimum int64
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}
