package contract

const (
	HostStartClaimInputContractVersionV3   = "praxis.agent-host/host-start-claim-input/v3"
	HostStartClaimInputObjectKindV3        = "praxis.agent-host/HostStartClaimInputV3"
	HostStartClaimBindingContractVersionV3 = "praxis.agent-host/host-start-claim-input-binding/v3"
	HostStartClaimBindingObjectKindV3      = "praxis.agent-host/HostStartClaimInputBindingV3"
)

// HostStartClaimInputV3 is the additive, flat V3 input sidecar. The permanent
// HostStartClaimV1 remains the only lifecycle conflict-domain fact.
type HostStartClaimInputV3 struct {
	ContractVersion      string                     `json:"contract_version"`
	ObjectKind           string                     `json:"object_kind"`
	HostID               string                     `json:"host_id"`
	StartID              string                     `json:"start_id"`
	DeploymentCurrentRef HostDeploymentCurrentRefV1 `json:"deployment_current_ref"`
	HostConfigDigest     DigestV1                   `json:"host_config_digest"`
	DefinitionSourceRef  ExactRefV1                 `json:"definition_source_ref"`
	RequestedOperation   HostStartOperationV1       `json:"requested_operation"`
	CreatedUnixNano      int64                      `json:"created_unix_nano"`
	ExpiresUnixNano      int64                      `json:"expires_unix_nano"`
	ContentDigest        DigestV1                   `json:"content_digest"`
}

func (i HostStartClaimInputV3) digestV3() (DigestV1, error) {
	i.ContentDigest = ""
	return DigestJSONV1(struct {
		Domain string                `json:"domain"`
		Type   string                `json:"type"`
		Body   HostStartClaimInputV3 `json:"body"`
	}{Domain: "praxis.agent-host.host-start-claim-input-v3", Type: "HostStartClaimInputV3", Body: i})
}

func SealHostStartClaimInputV3(i HostStartClaimInputV3) (HostStartClaimInputV3, error) {
	if i.ContractVersion != "" && i.ContractVersion != HostStartClaimInputContractVersionV3 {
		return HostStartClaimInputV3{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostStart InputV3 contract version drifted")
	}
	if i.ObjectKind != "" && i.ObjectKind != HostStartClaimInputObjectKindV3 {
		return HostStartClaimInputV3{}, NewError(ErrorInvalidArgument, "object_kind_mismatch", "HostStart InputV3 object kind drifted")
	}
	i.ContractVersion = HostStartClaimInputContractVersionV3
	i.ObjectKind = HostStartClaimInputObjectKindV3
	provided := i.ContentDigest
	i.ContentDigest = ""
	digest, err := i.digestV3()
	if err != nil {
		return HostStartClaimInputV3{}, err
	}
	if provided != "" && provided != digest {
		return HostStartClaimInputV3{}, NewError(ErrorConflict, "host_start_input_v3_digest_drift", "HostStart InputV3 supplied a wrong non-zero digest")
	}
	i.ContentDigest = digest
	return i, i.ValidateV3()
}

func (i HostStartClaimInputV3) ValidateV3() error {
	if i.ContractVersion != HostStartClaimInputContractVersionV3 || i.ObjectKind != HostStartClaimInputObjectKindV3 {
		return NewError(ErrorInvalidArgument, "host_start_input_v3_contract_invalid", "HostStart InputV3 discriminator is unsupported")
	}
	if err := ValidateIdentifierV1("host id", i.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", i.StartID); err != nil {
		return err
	}
	if err := i.DeploymentCurrentRef.Validate(); err != nil {
		return err
	}
	if i.DeploymentCurrentRef.HostID != i.HostID {
		return NewError(ErrorConflict, "host_start_input_v3_deployment_drift", "HostStart InputV3 deployment belongs to another Host")
	}
	if err := i.HostConfigDigest.Validate(); err != nil {
		return err
	}
	if err := i.DefinitionSourceRef.Validate(); err != nil {
		return err
	}
	if i.RequestedOperation != HostStartOperationStartV1 || i.CreatedUnixNano <= 0 || i.ExpiresUnixNano <= i.CreatedUnixNano || i.ExpiresUnixNano > i.DeploymentCurrentRef.ExpiresUnixNano {
		return NewError(ErrorInvalidArgument, "host_start_input_v3_window_invalid", "HostStart InputV3 operation or time window is invalid")
	}
	expected, err := i.digestV3()
	if err != nil || expected != i.ContentDigest {
		return NewError(ErrorConflict, "host_start_input_v3_digest_drift", "HostStart InputV3 digest drifted")
	}
	return nil
}

func (i HostStartClaimInputV3) ClaimV1() (HostStartClaimV1, error) {
	if err := i.ValidateV3(); err != nil {
		return HostStartClaimV1{}, err
	}
	return SealHostStartClaimV1(HostStartClaimV1{
		ContractVersion:     HostStartClaimContractVersionV1,
		HostContractVersion: HostLifecycleContractVersionV3,
		HostID:              i.HostID,
		StartID:             i.StartID,
		ConfigDigest:        i.ContentDigest,
		DefinitionSourceRef: i.DefinitionSourceRef,
		RequestedOperation:  i.RequestedOperation,
		CreatedUnixNano:     i.CreatedUnixNano,
		ExpiresUnixNano:     i.ExpiresUnixNano,
	})
}

type HostStartClaimInputBindingV3 struct {
	ContractVersion string                `json:"contract_version"`
	ObjectKind      string                `json:"object_kind"`
	ClaimRef        HostStartClaimRefV1   `json:"claim_ref"`
	Input           HostStartClaimInputV3 `json:"input"`
	BindingDigest   DigestV1              `json:"binding_digest"`
}

func (b HostStartClaimInputBindingV3) digestV3() (DigestV1, error) {
	b.BindingDigest = ""
	return DigestJSONV1(struct {
		Domain string                       `json:"domain"`
		Type   string                       `json:"type"`
		Body   HostStartClaimInputBindingV3 `json:"body"`
	}{Domain: "praxis.agent-host.host-start-claim-input-binding-v3", Type: "HostStartClaimInputBindingV3", Body: b})
}

func SealHostStartClaimInputBindingV3(b HostStartClaimInputBindingV3) (HostStartClaimInputBindingV3, error) {
	if b.ContractVersion != "" && b.ContractVersion != HostStartClaimBindingContractVersionV3 {
		return HostStartClaimInputBindingV3{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostStart Input binding version drifted")
	}
	if b.ObjectKind != "" && b.ObjectKind != HostStartClaimBindingObjectKindV3 {
		return HostStartClaimInputBindingV3{}, NewError(ErrorInvalidArgument, "object_kind_mismatch", "HostStart Input binding object kind drifted")
	}
	b.ContractVersion = HostStartClaimBindingContractVersionV3
	b.ObjectKind = HostStartClaimBindingObjectKindV3
	provided := b.BindingDigest
	b.BindingDigest = ""
	digest, err := b.digestV3()
	if err != nil {
		return HostStartClaimInputBindingV3{}, err
	}
	if provided != "" && provided != digest {
		return HostStartClaimInputBindingV3{}, NewError(ErrorConflict, "host_start_input_binding_v3_digest_drift", "HostStart Input binding supplied a wrong non-zero digest")
	}
	b.BindingDigest = digest
	return b, b.ValidateV3()
}

func (b HostStartClaimInputBindingV3) ValidateV3() error {
	if b.ContractVersion != HostStartClaimBindingContractVersionV3 || b.ObjectKind != HostStartClaimBindingObjectKindV3 {
		return NewError(ErrorInvalidArgument, "host_start_input_binding_v3_contract_invalid", "HostStart Input binding discriminator is unsupported")
	}
	if err := b.ClaimRef.Validate(); err != nil {
		return err
	}
	if err := b.Input.ValidateV3(); err != nil {
		return err
	}
	if b.ClaimRef.HostID != b.Input.HostID || b.ClaimRef.StartID != b.Input.StartID || b.ClaimRef.ExpiresUnixNano != b.Input.ExpiresUnixNano {
		return NewError(ErrorConflict, "host_start_input_binding_v3_claim_drift", "HostStart Input binding claim coordinates drifted")
	}
	expectedClaim, err := b.Input.ClaimV1()
	if err != nil {
		return err
	}
	expectedRef, err := expectedClaim.CurrentRefV1()
	if err != nil {
		return err
	}
	if b.ClaimRef != expectedRef {
		return NewError(ErrorConflict, "host_start_input_binding_v3_claim_drift", "HostStart Input binding does not bind the derived exact Claim")
	}
	expected, err := b.digestV3()
	if err != nil || expected != b.BindingDigest {
		return NewError(ErrorConflict, "host_start_input_binding_v3_digest_drift", "HostStart Input binding digest drifted")
	}
	return nil
}

func NewHostStartClaimInputBindingV3(claim HostStartClaimV1, input HostStartClaimInputV3) (HostStartClaimInputBindingV3, error) {
	if err := claim.ValidateHistoricalV1(); err != nil {
		return HostStartClaimInputBindingV3{}, err
	}
	if err := input.ValidateV3(); err != nil {
		return HostStartClaimInputBindingV3{}, err
	}
	expected, err := input.ClaimV1()
	if err != nil {
		return HostStartClaimInputBindingV3{}, err
	}
	if !SameHostStartClaimV1(claim, expected) {
		return HostStartClaimInputBindingV3{}, NewError(ErrorConflict, "host_start_input_v3_claim_drift", "HostStart Claim does not bind the exact InputV3")
	}
	ref, err := claim.CurrentRefV1()
	if err != nil {
		return HostStartClaimInputBindingV3{}, err
	}
	return SealHostStartClaimInputBindingV3(HostStartClaimInputBindingV3{ClaimRef: ref, Input: input})
}
