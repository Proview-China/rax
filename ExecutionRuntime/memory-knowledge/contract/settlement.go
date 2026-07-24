package contract

import (
	"fmt"
	"strings"
	"time"
)

type DomainResultState string

const (
	DomainResultReady          DomainResultState = "result_ready"
	DomainResultSettled        DomainResultState = "settled"
	DomainResultReconciliation DomainResultState = "reconciliation_required"
)

type DomainResultFact struct {
	Ref           Ref               `json:"ref"`
	Owner         OwnerDomain       `json:"owner"`
	OperationRef  Ref               `json:"operation_ref"`
	AttemptID     string            `json:"attempt_id"`
	SubjectRef    Ref               `json:"subject_ref"`
	CASBefore     uint64            `json:"cas_before"`
	CASAfter      uint64            `json:"cas_after"`
	InspectionRef Ref               `json:"inspection_ref"`
	EvidenceRefs  []Ref             `json:"evidence_refs"`
	Coverage      Coverage          `json:"coverage"`
	CleanupState  string            `json:"cleanup_state"`
	Residuals     []string          `json:"residuals"`
	State         DomainResultState `json:"state"`
	CreatedAt     time.Time         `json:"created_at"`
}

func NewDomainResultFact(owner OwnerDomain, id, attemptID string, operation, subject, inspection Ref, before, after uint64, evidence []Ref, coverage Coverage, cleanup string, residuals []string, now time.Time) (DomainResultFact, error) {
	f := DomainResultFact{
		Ref: Ref{ID: id, Revision: 1}, Owner: owner, OperationRef: operation,
		AttemptID: attemptID, SubjectRef: subject, CASBefore: before, CASAfter: after,
		InspectionRef: inspection, EvidenceRefs: NormalizeRefs(evidence), Coverage: coverage,
		CleanupState: cleanup, Residuals: append([]string{}, residuals...),
		State: DomainResultReady, CreatedAt: now.UTC(),
	}
	if err := f.validateFields(); err != nil {
		return DomainResultFact{}, err
	}
	digest, err := domainResultDigest(f)
	if err != nil {
		return DomainResultFact{}, err
	}
	f.Ref.Digest = digest
	if err := f.Validate(); err != nil {
		return DomainResultFact{}, err
	}
	return f, nil
}

func (f DomainResultFact) Validate() error {
	if err := f.validateFields(); err != nil {
		return err
	}
	if strings.TrimSpace(f.Ref.Digest) == "" {
		return fmt.Errorf("%w: missing domain result digest", ErrInvalidArgument)
	}
	digest, err := domainResultDigest(f)
	if err != nil {
		return err
	}
	if digest != f.Ref.Digest {
		return fmt.Errorf("%w: domain result digest mismatch", ErrEvidenceConflict)
	}
	return nil
}

func (f DomainResultFact) validateFields() error {
	if f.Owner != OwnerMemory && f.Owner != OwnerKnowledge {
		return fmt.Errorf("%w: invalid owner", ErrInvalidArgument)
	}
	if strings.TrimSpace(f.Ref.ID) == "" || f.Ref.Revision == 0 || strings.TrimSpace(f.AttemptID) == "" || f.CreatedAt.IsZero() || f.CASAfter < f.CASBefore {
		return fmt.Errorf("%w: incomplete domain result", ErrInvalidArgument)
	}
	if err := f.OperationRef.Validate(); err != nil {
		return err
	}
	if err := f.SubjectRef.Validate(); err != nil {
		return err
	}
	if err := f.InspectionRef.Validate(); err != nil {
		return err
	}
	for _, ref := range f.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func domainResultDigest(f DomainResultFact) (string, error) {
	f.Ref.Digest = ""
	digest, err := Digest(f)
	if err != nil {
		return "", fmt.Errorf("canonical domain result: %w", err)
	}
	return digest, nil
}

// RuntimeSettlementRef is deliberately opaque and reference-only. It cannot
// carry Runtime Outcome, disposition, Binding, Policy, Trust, or another
// owner's facts. Runtime owns every semantic field behind this reference.
type RuntimeSettlementRef struct {
	Ref Ref `json:"ref"`
}

func (s RuntimeSettlementRef) Validate() error { return s.Ref.Validate() }

// DomainResultAssociation is the component-readable binding carried beside an
// opaque Runtime settlement. It exposes no Runtime semantics; it only proves
// which exact domain result revision and digest Runtime settled.
type DomainResultAssociation struct {
	DomainResultRef Ref `json:"domain_result_ref"`
}

func AssociateDomainResult(result DomainResultFact) (DomainResultAssociation, error) {
	if err := result.Validate(); err != nil {
		return DomainResultAssociation{}, err
	}
	return DomainResultAssociation{DomainResultRef: result.Ref}, nil
}

func (a DomainResultAssociation) Validate() error {
	return a.DomainResultRef.Validate()
}

func (a DomainResultAssociation) Verify(result DomainResultFact) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := result.Validate(); err != nil {
		return err
	}
	if !SameRef(a.DomainResultRef, result.Ref) {
		return ErrSettlementMismatch
	}
	return nil
}

type SettlementApplication struct {
	Ref             Ref               `json:"ref"`
	Owner           OwnerDomain       `json:"owner"`
	DomainResultRef Ref               `json:"domain_result_ref"`
	SettlementRef   Ref               `json:"settlement_ref"`
	State           DomainResultState `json:"state"`
	AppliedAt       time.Time         `json:"applied_at"`
}

func NewSettlementApplication(owner OwnerDomain, id string, revision uint64, result DomainResultFact, association DomainResultAssociation, settlement RuntimeSettlementRef, now time.Time) (SettlementApplication, error) {
	if owner != result.Owner {
		return SettlementApplication{}, ErrSettlementMismatch
	}
	if err := association.Verify(result); err != nil {
		return SettlementApplication{}, err
	}
	if err := settlement.Validate(); err != nil {
		return SettlementApplication{}, err
	}
	a := SettlementApplication{
		Ref: Ref{ID: id, Revision: revision}, Owner: owner, DomainResultRef: result.Ref,
		SettlementRef: settlement.Ref, State: DomainResultSettled, AppliedAt: now.UTC(),
	}
	digest, err := settlementApplicationDigest(a)
	if err != nil {
		return SettlementApplication{}, err
	}
	a.Ref.Digest = digest
	if err := a.Validate(); err != nil {
		return SettlementApplication{}, err
	}
	return a, nil
}

func (a SettlementApplication) Validate() error {
	if a.Owner != OwnerMemory && a.Owner != OwnerKnowledge {
		return fmt.Errorf("%w: invalid owner", ErrInvalidArgument)
	}
	if strings.TrimSpace(a.Ref.ID) == "" || a.Ref.Revision == 0 || strings.TrimSpace(a.Ref.Digest) == "" || a.State != DomainResultSettled || a.AppliedAt.IsZero() {
		return fmt.Errorf("%w: incomplete settlement application", ErrInvalidArgument)
	}
	if err := a.DomainResultRef.Validate(); err != nil {
		return err
	}
	if err := a.SettlementRef.Validate(); err != nil {
		return err
	}
	digest, err := settlementApplicationDigest(a)
	if err != nil {
		return err
	}
	if digest != a.Ref.Digest {
		return fmt.Errorf("%w: settlement application digest mismatch", ErrEvidenceConflict)
	}
	return nil
}

func settlementApplicationDigest(a SettlementApplication) (string, error) {
	a.Ref.Digest = ""
	digest, err := Digest(a)
	if err != nil {
		return "", fmt.Errorf("canonical settlement application: %w", err)
	}
	return digest, nil
}
