package contract

import (
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const SystemReadyGatewayContractVersionV2 = "praxis.agent-host/system-ready-gateway/v2"

// SystemReadySupervisionPolicyCurrentV2 is a narrow read-only projection of
// Runtime-owned supervision policy currentness. Agent Host may compare it, but
// does not own or sign the underlying policy fact.
type SystemReadySupervisionPolicyCurrentV2 struct {
	ContractVersion         string                         `json:"contract_version"`
	Ref                     runtimeports.OwnerCurrentRefV1 `json:"ref"`
	MinimumReadyWindowNanos int64                          `json:"minimum_ready_window_nanos"`
	CheckedUnixNano         int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                          `json:"expires_unix_nano"`
	ProjectionDigest        core.Digest                    `json:"projection_digest"`
}

func (p SystemReadySupervisionPolicyCurrentV2) DigestV2() (core.Digest, error) {
	c := p
	c.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.system-ready-supervision-policy-current-v2", SystemReadyGatewayContractVersionV2, "SystemReadySupervisionPolicyCurrentV2", c)
}

func SealSystemReadySupervisionPolicyCurrentV2(p SystemReadySupervisionPolicyCurrentV2) (SystemReadySupervisionPolicyCurrentV2, error) {
	if p.ContractVersion != "" && p.ContractVersion != SystemReadyGatewayContractVersionV2 {
		return SystemReadySupervisionPolicyCurrentV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "supervision policy projection version drifted")
	}
	p.ContractVersion = SystemReadyGatewayContractVersionV2
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	d, err := p.DigestV2()
	if err != nil {
		return SystemReadySupervisionPolicyCurrentV2{}, err
	}
	if provided != "" && provided != d {
		return SystemReadySupervisionPolicyCurrentV2{}, NewError(ErrorConflict, "supervision_policy_projection_drift", "supervision policy projection supplied a wrong digest")
	}
	p.ProjectionDigest = d
	return p, p.Validate()
}

func (p SystemReadySupervisionPolicyCurrentV2) Validate() error {
	if p.ContractVersion != SystemReadyGatewayContractVersionV2 || p.MinimumReadyWindowNanos <= 0 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "supervision_policy_projection_incomplete", "supervision policy projection is incomplete")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if p.Ref.ExpiresUnixNano != p.ExpiresUnixNano {
		return NewError(ErrorConflict, "supervision_policy_projection_expiry_drift", "supervision policy projection expiry drifted from its exact Ref")
	}
	d, err := p.DigestV2()
	if err != nil {
		return err
	}
	if d != p.ProjectionDigest {
		return NewError(ErrorConflict, "supervision_policy_projection_drift", "supervision policy projection digest drifted")
	}
	return nil
}

type SystemReadyEnsureRequestV2 struct {
	ContractVersion         string                         `json:"contract_version"`
	AttemptID               string                         `json:"attempt_id"`
	HostID                  string                         `json:"host_id"`
	StartID                 string                         `json:"start_id"`
	Claim                   HostStartClaimRefV1            `json:"claim"`
	Definition              runtimeports.OwnerCurrentRefV1 `json:"definition"`
	Plan                    runtimeports.OwnerCurrentRefV1 `json:"plan"`
	Assembly                runtimeports.OwnerCurrentRefV1 `json:"assembly"`
	BindingSet              runtimeports.OwnerCurrentRefV1 `json:"binding_set"`
	Activation              runtimeports.OwnerCurrentRefV1 `json:"activation"`
	GenerationBinding       runtimeports.OwnerCurrentRefV1 `json:"generation_binding"`
	ApplicationStart        runtimeports.OwnerCurrentRefV1 `json:"application_start"`
	SandboxLease            runtimeports.OwnerCurrentRefV1 `json:"sandbox_lease"`
	SandboxActive           runtimeports.OwnerCurrentRefV1 `json:"sandbox_active"`
	ExecutionReady          runtimeports.OwnerCurrentRefV1 `json:"execution_ready"`
	SupervisionPolicy       runtimeports.OwnerCurrentRefV1 `json:"supervision_policy"`
	Components              []ComponentProductionCurrentV2 `json:"components"`
	MinimumReadyWindowNanos int64                          `json:"minimum_ready_window_nanos"`
	AvailabilityEpoch       core.Epoch                     `json:"availability_epoch"`
	ExpectedCurrent         *SystemReadyCurrentRefV2       `json:"expected_current,omitempty"`
	RequestDigest           core.Digest                    `json:"request_digest"`
}

func (r SystemReadyEnsureRequestV2) canonical() SystemReadyEnsureRequestV2 {
	c := r
	c.Components = append([]ComponentProductionCurrentV2{}, r.Components...)
	sort.Slice(c.Components, func(i, j int) bool { return c.Components[i].Domain < c.Components[j].Domain })
	if r.ExpectedCurrent != nil {
		v := *r.ExpectedCurrent
		c.ExpectedCurrent = &v
	}
	return c
}
func (r SystemReadyEnsureRequestV2) DigestV2() (core.Digest, error) {
	c := r.canonical()
	c.RequestDigest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.system-ready-gateway-v2", SystemReadyGatewayContractVersionV2, "SystemReadyEnsureRequestV2", c)
}
func SealSystemReadyEnsureRequestV2(r SystemReadyEnsureRequestV2) (SystemReadyEnsureRequestV2, error) {
	if r.ContractVersion != "" && r.ContractVersion != SystemReadyGatewayContractVersionV2 {
		return SystemReadyEnsureRequestV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "SystemReady gateway request version drifted")
	}
	r.ContractVersion = SystemReadyGatewayContractVersionV2
	r = r.canonical()
	provided := r.RequestDigest
	r.RequestDigest = ""
	d, e := r.DigestV2()
	if e != nil {
		return SystemReadyEnsureRequestV2{}, e
	}
	if provided != "" && provided != d {
		return SystemReadyEnsureRequestV2{}, NewError(ErrorConflict, "system_ready_request_drift", "SystemReady request supplied a wrong digest")
	}
	r.RequestDigest = d
	return r, r.Validate()
}
func (r SystemReadyEnsureRequestV2) Validate() error {
	if r.ContractVersion != SystemReadyGatewayContractVersionV2 || r.MinimumReadyWindowNanos <= 0 || r.AvailabilityEpoch == 0 || len(r.Components) == 0 {
		return NewError(ErrorInvalidArgument, "system_ready_request_incomplete", "SystemReady request is incomplete")
	}
	for n, v := range map[string]string{"attempt": r.AttemptID, "host": r.HostID, "start": r.StartID} {
		if e := ValidateIdentifierV1(n, v); e != nil {
			return e
		}
	}
	if e := r.Claim.Validate(); e != nil {
		return e
	}
	if r.Claim.HostID != r.HostID || r.Claim.StartID != r.StartID {
		return NewError(ErrorConflict, "system_ready_claim_subject_drift", "SystemReady request names another claim")
	}
	refs := []runtimeports.OwnerCurrentRefV1{r.Definition, r.Plan, r.Assembly, r.BindingSet, r.Activation, r.GenerationBinding, r.ApplicationStart, r.SandboxLease, r.SandboxActive, r.ExecutionReady, r.SupervisionPolicy}
	for _, v := range refs {
		if e := v.Validate(); e != nil {
			return e
		}
	}
	for i, v := range r.Components {
		if e := v.Validate(); e != nil {
			return e
		}
		if i > 0 && r.Components[i-1].Domain >= v.Domain {
			return NewError(ErrorConflict, "system_ready_component_duplicate", "components must be canonical")
		}
	}
	if r.ExpectedCurrent != nil {
		if e := r.ExpectedCurrent.Validate(); e != nil {
			return e
		}
		if r.ExpectedCurrent.Epoch != r.AvailabilityEpoch {
			return NewError(ErrorConflict, "system_ready_epoch_drift", "expected Current epoch drifted")
		}
	}
	d, e := r.DigestV2()
	if e != nil {
		return e
	}
	if d != r.RequestDigest {
		return NewError(ErrorConflict, "system_ready_request_drift", "SystemReady request digest drifted")
	}
	return nil
}

type SystemReadyGatewayResultV2 struct {
	ContractVersion string                  `json:"contract_version"`
	AttemptID       string                  `json:"attempt_id"`
	RequestDigest   core.Digest             `json:"request_digest"`
	Fact            SystemReadyFactRefV2    `json:"fact"`
	Current         SystemReadyCurrentRefV2 `json:"current"`
	ResultDigest    core.Digest             `json:"result_digest"`
}

func (r SystemReadyGatewayResultV2) DigestV2() (core.Digest, error) {
	c := r
	c.ResultDigest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.system-ready-gateway-result-v2", SystemReadyGatewayContractVersionV2, "SystemReadyGatewayResultV2", c)
}
func SealSystemReadyGatewayResultV2(r SystemReadyGatewayResultV2) (SystemReadyGatewayResultV2, error) {
	r.ContractVersion = SystemReadyGatewayContractVersionV2
	p := r.ResultDigest
	r.ResultDigest = ""
	d, e := r.DigestV2()
	if e != nil {
		return SystemReadyGatewayResultV2{}, e
	}
	if p != "" && p != d {
		return SystemReadyGatewayResultV2{}, NewError(ErrorConflict, "system_ready_result_drift", "result supplied wrong digest")
	}
	r.ResultDigest = d
	return r, r.Validate()
}
func (r SystemReadyGatewayResultV2) Validate() error {
	if r.ContractVersion != SystemReadyGatewayContractVersionV2 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "result version invalid")
	}
	if e := ValidateIdentifierV1("attempt", r.AttemptID); e != nil {
		return e
	}
	if e := r.RequestDigest.Validate(); e != nil {
		return e
	}
	if e := r.Fact.Validate(); e != nil {
		return e
	}
	if e := r.Current.Validate(); e != nil {
		return e
	}
	d, e := r.DigestV2()
	if e != nil {
		return e
	}
	if d != r.ResultDigest {
		return NewError(ErrorConflict, "system_ready_result_drift", "result digest drifted")
	}
	return nil
}

type SystemReadyInspectRequestV2 struct {
	HostID        string      `json:"host_id"`
	StartID       string      `json:"start_id"`
	AttemptID     string      `json:"attempt_id"`
	RequestDigest core.Digest `json:"request_digest"`
}

func (r SystemReadyInspectRequestV2) Validate() error {
	for name, value := range map[string]string{"host": r.HostID, "start": r.StartID, "attempt": r.AttemptID} {
		if err := ValidateIdentifierV1(name, value); err != nil {
			return err
		}
	}
	return r.RequestDigest.Validate()
}
