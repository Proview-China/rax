package contract

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const OwnerJobAttemptObjectKindV1 = "memory_knowledge_owner_job_attempt"

type OwnerJobKind string

const (
	JobMemoryConsolidation OwnerJobKind = "memory_consolidation"
	JobKnowledgeSync       OwnerJobKind = "knowledge_sync"
	JobReindex             OwnerJobKind = "reindex"
	JobPurge               OwnerJobKind = "purge"
	JobExport              OwnerJobKind = "export"
)

type OwnerJobState string

const (
	JobReserved               OwnerJobState = "reserved"
	JobBegun                  OwnerJobState = "begun"
	JobUnknownOutcome         OwnerJobState = "unknown_outcome"
	JobObserved               OwnerJobState = "observed"
	JobReady                  OwnerJobState = "ready"
	JobSettled                OwnerJobState = "settled"
	JobReconciliationRequired OwnerJobState = "reconciliation_required"
	JobResidual               OwnerJobState = "residual"
)

type OwnerJobAttemptV1 struct {
	ContractVersion string        `json:"contract_version"`
	ObjectKind      string        `json:"object_kind"`
	Ref             Ref           `json:"ref"`
	Owner           OwnerDomain   `json:"owner"`
	Kind            OwnerJobKind  `json:"kind"`
	TenantID        string        `json:"tenant_id"`
	AuthorityRef    Ref           `json:"authority_ref"`
	PolicyRef       Ref           `json:"policy_ref"`
	ScopeRef        Ref           `json:"scope_ref"`
	OperationRef    Ref           `json:"operation_ref"`
	AttemptRef      Ref           `json:"attempt_ref"`
	SubjectRef      Ref           `json:"subject_ref"`
	InputDigest     string        `json:"input_digest"`
	ObservationRef  Ref           `json:"observation_ref"`
	ResultRef       Ref           `json:"result_ref"`
	DomainResultRef Ref           `json:"domain_result_ref"`
	State           OwnerJobState `json:"state"`
	Residuals       []string      `json:"residuals"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	ExpiresAt       time.Time     `json:"expires_at"`
	Digest          string        `json:"digest"`
}

func SealOwnerJobAttemptV1(in OwnerJobAttemptV1) (OwnerJobAttemptV1, error) {
	in.ContractVersion = FrameworkContractVersionV1
	in.ObjectKind = OwnerJobAttemptObjectKindV1
	residuals, err := normalizeJobResiduals(in.Residuals)
	if err != nil {
		return OwnerJobAttemptV1{}, err
	}
	in.Residuals = residuals
	in.CreatedAt, in.UpdatedAt, in.ExpiresAt = in.CreatedAt.UTC(), in.UpdatedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	digest, err := Digest(in)
	if err != nil {
		return OwnerJobAttemptV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(); err != nil {
		return OwnerJobAttemptV1{}, err
	}
	return in, nil
}

func (in OwnerJobAttemptV1) Validate() error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != OwnerJobAttemptObjectKindV1 || !validJobOwner(in.Owner, in.Kind) || !validJobState(in.State) || strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.InputDigest) == "" || in.CreatedAt.IsZero() || in.UpdatedAt.Before(in.CreatedAt) || !in.ExpiresAt.After(in.UpdatedAt) {
		return fmt.Errorf("%w: incomplete owner job", ErrInvalidArgument)
	}
	for _, ref := range []Ref{in.Ref, in.AuthorityRef, in.PolicyRef, in.ScopeRef, in.OperationRef, in.AttemptRef, in.SubjectRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if stateAtLeastObserved(in.State) {
		if err := in.ObservationRef.Validate(); err != nil {
			return fmt.Errorf("%w: observation required", ErrInvalidArgument)
		}
	} else if in.State == JobResidual && in.ObservationRef != (Ref{}) {
		if err := in.ObservationRef.Validate(); err != nil {
			return fmt.Errorf("%w: residual observation", ErrInvalidArgument)
		}
	} else if in.ObservationRef != (Ref{}) {
		return fmt.Errorf("%w: premature observation", ErrInvalidArgument)
	}
	if in.State == JobReady || in.State == JobSettled {
		if in.ResultRef.Validate() != nil || in.DomainResultRef.Validate() != nil {
			return fmt.Errorf("%w: result refs required", ErrInvalidArgument)
		}
	} else if in.ResultRef != (Ref{}) || in.DomainResultRef != (Ref{}) {
		return fmt.Errorf("%w: premature result", ErrInvalidArgument)
	}
	residuals, err := normalizeJobResiduals(in.Residuals)
	if err != nil || !slices.Equal(residuals, in.Residuals) {
		return fmt.Errorf("%w: residuals", ErrInvalidArgument)
	}
	if (in.State == JobResidual || in.State == JobReconciliationRequired) && len(in.Residuals) == 0 {
		return fmt.Errorf("%w: residual reason required", ErrInvalidArgument)
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: owner job digest", ErrEvidenceConflict)
	}
	return nil
}

func ValidateOwnerJobTransitionV1(current, next OwnerJobAttemptV1) error {
	if err := current.Validate(); err != nil {
		return fmt.Errorf("current owner job: %w", err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("next owner job: %w", err)
	}
	if current.Ref.ID != next.Ref.ID || next.Ref.Revision != current.Ref.Revision+1 || current.Owner != next.Owner || current.Kind != next.Kind || current.TenantID != next.TenantID || !SameRef(current.AuthorityRef, next.AuthorityRef) || !SameRef(current.PolicyRef, next.PolicyRef) || !SameRef(current.ScopeRef, next.ScopeRef) || !SameRef(current.OperationRef, next.OperationRef) || !SameRef(current.AttemptRef, next.AttemptRef) || !SameRef(current.SubjectRef, next.SubjectRef) || current.InputDigest != next.InputDigest || !current.CreatedAt.Equal(next.CreatedAt) || next.UpdatedAt.Before(current.UpdatedAt) || !next.ExpiresAt.Equal(current.ExpiresAt) {
		return fmt.Errorf("%w: owner job identity drift", ErrEvidenceConflict)
	}
	if current.State == JobUnknownOutcome && next.State != JobObserved && next.State != JobReconciliationRequired && next.State != JobResidual {
		return ErrUnknownOutcome
	}
	if !allowedJobTransition(current.State, next.State) {
		return fmt.Errorf("%w: invalid owner job transition", ErrRevisionConflict)
	}
	return nil
}

func allowedJobTransition(from, to OwnerJobState) bool {
	switch from {
	case JobReserved:
		return to == JobBegun || to == JobResidual
	case JobBegun:
		return to == JobUnknownOutcome || to == JobObserved || to == JobResidual
	case JobUnknownOutcome:
		return to == JobObserved || to == JobReconciliationRequired || to == JobResidual
	case JobObserved:
		return to == JobReady || to == JobReconciliationRequired || to == JobResidual
	case JobReady:
		return to == JobSettled || to == JobReconciliationRequired || to == JobResidual
	case JobReconciliationRequired:
		return to == JobObserved || to == JobResidual
	default:
		return false
	}
}

func validJobOwner(owner OwnerDomain, kind OwnerJobKind) bool {
	if owner != OwnerMemory && owner != OwnerKnowledge {
		return false
	}
	switch kind {
	case JobMemoryConsolidation:
		return owner == OwnerMemory
	case JobKnowledgeSync:
		return owner == OwnerKnowledge
	case JobReindex, JobPurge, JobExport:
		return true
	default:
		return false
	}
}

func validJobState(state OwnerJobState) bool {
	return state == JobReserved || state == JobBegun || state == JobUnknownOutcome || state == JobObserved || state == JobReady || state == JobSettled || state == JobReconciliationRequired || state == JobResidual
}

func stateAtLeastObserved(state OwnerJobState) bool {
	return state == JobObserved || state == JobReady || state == JobSettled || state == JobReconciliationRequired
}

func normalizeJobResiduals(in []string) ([]string, error) {
	out := slices.Clone(in)
	for _, value := range out {
		if value == "" || value != strings.TrimSpace(value) {
			return nil, fmt.Errorf("%w: residual", ErrInvalidArgument)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		out = []string{}
	}
	return out, nil
}
