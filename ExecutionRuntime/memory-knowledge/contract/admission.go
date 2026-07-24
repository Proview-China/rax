package contract

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	AdmissionPolicyRequestObjectKindV1 = "memory_knowledge_admission_policy_request"
	AdmissionAdviceObjectKindV1        = "memory_knowledge_admission_advice"
)

type AdmissionAdviceDecision string

const (
	AdviceReject AdmissionAdviceDecision = "reject"
	AdviceReview AdmissionAdviceDecision = "review"
	AdviceCommit AdmissionAdviceDecision = "commit"
	AdviceMerge  AdmissionAdviceDecision = "merge"
)

type AdmissionPolicyRequestV1 struct {
	ContractVersion string      `json:"contract_version"`
	ObjectKind      string      `json:"object_kind"`
	Owner           OwnerDomain `json:"owner"`
	TenantID        string      `json:"tenant_id"`
	CandidateRef    Ref         `json:"candidate_ref"`
	AuthorityRef    Ref         `json:"authority_ref"`
	PolicyRef       Ref         `json:"policy_ref"`
	ScopeRef        Ref         `json:"scope_ref"`
	Purpose         string      `json:"purpose"`
	Sensitivity     string      `json:"sensitivity"`
	RiskFlags       []string    `json:"risk_flags"`
	RequestedAt     time.Time   `json:"requested_at"`
	ExpiresAt       time.Time   `json:"expires_at"`
	Digest          string      `json:"digest"`
}

type AdmissionAdviceV1 struct {
	ContractVersion  string                  `json:"contract_version"`
	ObjectKind       string                  `json:"object_kind"`
	Ref              Ref                     `json:"ref"`
	RequestDigest    string                  `json:"request_digest"`
	PolicyAdapterRef Ref                     `json:"policy_adapter_ref"`
	Decision         AdmissionAdviceDecision `json:"decision"`
	MergeTargetRef   Ref                     `json:"merge_target_ref,omitempty"`
	ReasonCodes      []string                `json:"reason_codes"`
	ObservedAt       time.Time               `json:"observed_at"`
	ExpiresAt        time.Time               `json:"expires_at"`
	Digest           string                  `json:"digest"`
}

func SealAdmissionPolicyRequestV1(in AdmissionPolicyRequestV1) (AdmissionPolicyRequestV1, error) {
	in.ContractVersion, in.ObjectKind = FrameworkContractVersionV1, AdmissionPolicyRequestObjectKindV1
	in.RiskFlags = normalizeAdmissionStrings(in.RiskFlags)
	in.RequestedAt, in.ExpiresAt = in.RequestedAt.UTC(), in.ExpiresAt.UTC()
	in.Digest = ""
	digest, err := Digest(in)
	if err != nil {
		return AdmissionPolicyRequestV1{}, err
	}
	in.Digest = digest
	if err := in.Validate(in.RequestedAt); err != nil {
		return AdmissionPolicyRequestV1{}, err
	}
	return in, nil
}

func (in AdmissionPolicyRequestV1) Validate(now time.Time) error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != AdmissionPolicyRequestObjectKindV1 || (in.Owner != OwnerMemory && in.Owner != OwnerKnowledge) || strings.TrimSpace(in.TenantID) == "" || in.CandidateRef.Validate() != nil || in.AuthorityRef.Validate() != nil || in.PolicyRef.Validate() != nil || in.ScopeRef.Validate() != nil || strings.TrimSpace(in.Purpose) == "" || strings.TrimSpace(in.Sensitivity) == "" || in.RequestedAt.IsZero() || !in.ExpiresAt.After(in.RequestedAt) || !in.ExpiresAt.After(now) || !slices.Equal(in.RiskFlags, normalizeAdmissionStrings(in.RiskFlags)) {
		return fmt.Errorf("%w: admission policy request", ErrInvalidArgument)
	}
	copy := in
	copy.Digest = ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest {
		return fmt.Errorf("%w: admission policy request digest", ErrEvidenceConflict)
	}
	return nil
}

func SealAdmissionAdviceV1(in AdmissionAdviceV1) (AdmissionAdviceV1, error) {
	in.ContractVersion, in.ObjectKind = FrameworkContractVersionV1, AdmissionAdviceObjectKindV1
	in.ReasonCodes = normalizeAdmissionStrings(in.ReasonCodes)
	in.ObservedAt, in.ExpiresAt = in.ObservedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	digest, err := Digest(in)
	if err != nil {
		return AdmissionAdviceV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(in.ObservedAt); err != nil {
		return AdmissionAdviceV1{}, err
	}
	return in, nil
}

func (in AdmissionAdviceV1) Validate(now time.Time) error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != AdmissionAdviceObjectKindV1 || in.Ref.Validate() != nil || strings.TrimSpace(in.RequestDigest) == "" || in.PolicyAdapterRef.Validate() != nil || !validAdviceDecision(in.Decision) || len(in.ReasonCodes) == 0 || !slices.Equal(in.ReasonCodes, normalizeAdmissionStrings(in.ReasonCodes)) || in.ObservedAt.IsZero() || !in.ExpiresAt.After(in.ObservedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: admission advice", ErrInvalidArgument)
	}
	if in.Decision == AdviceMerge {
		if in.MergeTargetRef.Validate() != nil {
			return fmt.Errorf("%w: merge advice target", ErrInvalidArgument)
		}
	} else if in.MergeTargetRef != (Ref{}) {
		return fmt.Errorf("%w: unexpected merge advice target", ErrInvalidArgument)
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: admission advice digest", ErrEvidenceConflict)
	}
	return nil
}

func validAdviceDecision(in AdmissionAdviceDecision) bool {
	return in == AdviceReject || in == AdviceReview || in == AdviceCommit || in == AdviceMerge
}

func normalizeAdmissionStrings(in []string) []string {
	out := slices.Clone(in)
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		out = []string{}
	}
	return out
}
