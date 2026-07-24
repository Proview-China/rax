package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	SingleCallToolActionBindingCurrentContractVersionV2 = "praxis.tool.single-call-action-binding-current/v2"
	MaxSingleCallToolActionBindingCurrentTTLV2          = 15 * time.Second
)

const bindingCurrentCanonicalDomainV2 = "praxis.tool"

type SingleCallToolActionBindingCurrentRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r SingleCallToolActionBindingCurrentRefV2) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision != 1 {
		return invalid("Single-call Tool Action Binding V2 Ref is invalid")
	}
	return r.Digest.Validate()
}

type SingleCallToolActionBindingSubjectV2 struct {
	ContractVersion            string                        `json:"contract_version"`
	ApplicationRequestID       string                        `json:"application_request_id"`
	ApplicationRequestRevision core.Revision                 `json:"application_request_revision"`
	ApplicationRequestDigest   core.Digest                   `json:"application_request_digest"`
	PendingAction              PendingActionExactRefV2       `json:"pending_action"`
	TenantID                   core.TenantID                 `json:"tenant_id"`
	RunID                      string                        `json:"run_id"`
	SessionID                  string                        `json:"session_id"`
	TurnID                     string                        `json:"turn_id"`
	ActionCoordinateDigest     core.Digest                   `json:"action_coordinate_digest"`
	ExecutionScope             core.ExecutionScope           `json:"execution_scope"`
	ExecutionScopeDigest       core.Digest                   `json:"execution_scope_digest"`
	SourceSubjectDigest        core.Digest                   `json:"source_subject_digest"`
	EffectKind                 runtimeports.EffectKindV2     `json:"effect_kind"`
	PolicyProfile              runtimeports.NamespacedNameV2 `json:"policy_profile"`
	CandidateContractVersion   string                        `json:"candidate_contract_version"`
	InputContractVersion       string                        `json:"input_contract_version"`
	Digest                     core.Digest                   `json:"digest"`
}

func (s SingleCallToolActionBindingSubjectV2) Validate() error {
	if s.ContractVersion != SingleCallToolActionBindingCurrentContractVersionV2 || strings.TrimSpace(s.ApplicationRequestID) == "" || s.ApplicationRequestRevision == 0 || s.ApplicationRequestDigest.Validate() != nil || s.PendingAction.Validate() != nil || s.PendingAction.Revision != 1 || strings.TrimSpace(string(s.TenantID)) == "" || strings.TrimSpace(s.RunID) == "" || strings.TrimSpace(s.SessionID) == "" || strings.TrimSpace(s.TurnID) == "" || s.ActionCoordinateDigest.Validate() != nil || s.ExecutionScope.Validate() != nil || s.ExecutionScopeDigest.Validate() != nil || s.SourceSubjectDigest.Validate() != nil || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(s.EffectKind)) != nil || runtimeports.ValidateNamespacedNameV2(s.PolicyProfile) != nil || s.CandidateContractVersion != ActionContractVersionV3 || s.InputContractVersion != ToolInputContractCurrentContractVersionV1 {
		return invalid("Single-call Tool Action Binding V2 Subject is invalid")
	}
	if s.EffectKind != runtimeports.OperationScopeEvidenceActionEffectKindV3 || s.PolicyProfile != runtimeports.OperationScopeEvidenceActionPolicyProfileV3 {
		return conflict("Single-call Tool Action Binding V2 effect matrix drifted")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(s.ExecutionScope)
	if err != nil || scopeDigest != s.ExecutionScopeDigest || s.ExecutionScope.Identity.TenantID != s.TenantID {
		return conflict("Single-call Tool Action Binding V2 execution scope drifted")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("Single-call Tool Action Binding V2 Subject digest drifted")
	}
	return nil
}

func (s SingleCallToolActionBindingSubjectV2) ComputeDigest() (core.Digest, error) {
	s.Digest = ""
	return core.CanonicalJSONDigest(bindingCurrentCanonicalDomainV2, SingleCallToolActionBindingCurrentContractVersionV2, "SingleCallToolActionBindingSubjectV2", s)
}

func SealSingleCallToolActionBindingSubjectV2(s SingleCallToolActionBindingSubjectV2) (SingleCallToolActionBindingSubjectV2, error) {
	s.ContractVersion = SingleCallToolActionBindingCurrentContractVersionV2
	s.EffectKind = runtimeports.OperationScopeEvidenceActionEffectKindV3
	s.PolicyProfile = runtimeports.OperationScopeEvidenceActionPolicyProfileV3
	s.CandidateContractVersion = ActionContractVersionV3
	s.InputContractVersion = ToolInputContractCurrentContractVersionV1
	provided := s.Digest
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return SingleCallToolActionBindingSubjectV2{}, err
	}
	if provided != "" && provided != digest {
		return SingleCallToolActionBindingSubjectV2{}, conflict("supplied Single-call Tool Action Binding V2 Subject digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

type SingleCallToolActionBindingIssuanceSubjectV2 struct {
	ContractVersion          string                               `json:"contract_version"`
	BindingSubject           SingleCallToolActionBindingSubjectV2 `json:"binding_subject"`
	RequestedExpiresUnixNano int64                                `json:"requested_expires_unix_nano"`
	Digest                   core.Digest                          `json:"digest"`
}

func (s SingleCallToolActionBindingIssuanceSubjectV2) Validate() error {
	if s.ContractVersion != SingleCallToolActionBindingCurrentContractVersionV2 || s.BindingSubject.Validate() != nil || s.RequestedExpiresUnixNano < 0 {
		return invalid("Single-call Tool Action Binding V2 Issuance is invalid")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("Single-call Tool Action Binding V2 Issuance digest drifted")
	}
	return nil
}

func (s SingleCallToolActionBindingIssuanceSubjectV2) ComputeDigest() (core.Digest, error) {
	s.Digest = ""
	return core.CanonicalJSONDigest(bindingCurrentCanonicalDomainV2, SingleCallToolActionBindingCurrentContractVersionV2, "SingleCallToolActionBindingIssuanceSubjectV2", s)
}

func SealSingleCallToolActionBindingIssuanceSubjectV2(s SingleCallToolActionBindingIssuanceSubjectV2) (SingleCallToolActionBindingIssuanceSubjectV2, error) {
	s.ContractVersion = SingleCallToolActionBindingCurrentContractVersionV2
	provided := s.Digest
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return SingleCallToolActionBindingIssuanceSubjectV2{}, err
	}
	if provided != "" && provided != digest {
		return SingleCallToolActionBindingIssuanceSubjectV2{}, conflict("supplied Single-call Tool Action Binding V2 Issuance digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

func DeriveSingleCallToolActionBindingCurrentIDV2(issuance SingleCallToolActionBindingIssuanceSubjectV2) (string, error) {
	if err := issuance.Validate(); err != nil {
		return "", err
	}
	digest, err := issuance.ComputeDigest()
	if err != nil {
		return "", err
	}
	return "single-call-tool-binding-v2-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}
