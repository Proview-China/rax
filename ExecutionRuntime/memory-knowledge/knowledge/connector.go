package knowledge

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const (
	ConnectorContractVersionV1     = "praxis.knowledge/source-connector/v1"
	AcquireRequestObjectKindV1     = "knowledge_acquire_request"
	AcquireObservationObjectKindV1 = "knowledge_acquire_observation"
	AcquireInspectionObjectKindV1  = "knowledge_acquire_inspection"
	AcquireEffectKindV1            = "knowledge_source_acquire"
)

type SourceKind string

const (
	SourceFile     SourceKind = "file"
	SourceDatabase SourceKind = "database"
	SourceCode     SourceKind = "code_repository"
	SourceAPI      SourceKind = "api"
	SourceRule     SourceKind = "rule_system"
	SourceManual   SourceKind = "manual"
	SourceExternal SourceKind = "external_public"
)

type AcquireRequestV1 struct {
	ContractVersion       string       `json:"contract_version"`
	ObjectKind            string       `json:"object_kind"`
	Ref                   contract.Ref `json:"ref"`
	TenantID              string       `json:"tenant_id"`
	SourceKind            SourceKind   `json:"source_kind"`
	SourceSubjectRef      contract.Ref `json:"source_subject_ref"`
	ConnectorRef          contract.Ref `json:"connector_ref"`
	AuthorityRef          contract.Ref `json:"authority_ref"`
	PolicyRef             contract.Ref `json:"policy_ref"`
	ScopeRef              contract.Ref `json:"scope_ref"`
	BudgetRef             contract.Ref `json:"budget_ref"`
	OperationRef          contract.Ref `json:"operation_ref"`
	AttemptRef            contract.Ref `json:"attempt_ref"`
	PermitRef             contract.Ref `json:"permit_ref"`
	PrepareEnforcementRef contract.Ref `json:"prepare_enforcement_ref"`
	ExecuteEnforcementRef contract.Ref `json:"execute_enforcement_ref"`
	EffectKind            string       `json:"effect_kind"`
	RequestedAt           time.Time    `json:"requested_at"`
	ExpiresAt             time.Time    `json:"expires_at"`
	Digest                string       `json:"digest"`
}

type AcquireObservationV1 struct {
	ContractVersion    string         `json:"contract_version"`
	ObjectKind         string         `json:"object_kind"`
	Ref                contract.Ref   `json:"ref"`
	RequestRef         contract.Ref   `json:"request_ref"`
	AttemptRef         contract.Ref   `json:"attempt_ref"`
	ProviderReceiptRef contract.Ref   `json:"provider_receipt_ref"`
	AssetRef           contract.Ref   `json:"asset_ref"`
	ContentDigest      string         `json:"content_digest"`
	SourceVersion      string         `json:"source_version"`
	ProvenanceRefs     []contract.Ref `json:"provenance_refs"`
	License            string         `json:"license"`
	Scope              string         `json:"scope"`
	Sensitivity        string         `json:"sensitivity"`
	ObservedAt         time.Time      `json:"observed_at"`
	ExpiresAt          time.Time      `json:"expires_at"`
	Digest             string         `json:"digest"`
}

type AcquireInspectionOutcome string

const (
	AcquireObserved              AcquireInspectionOutcome = "observed"
	AcquireConfirmedNotPersisted AcquireInspectionOutcome = "confirmed_not_persisted"
	AcquireIndeterminate         AcquireInspectionOutcome = "indeterminate"
)

type AcquireInspectionV1 struct {
	ContractVersion string                   `json:"contract_version"`
	ObjectKind      string                   `json:"object_kind"`
	Ref             contract.Ref             `json:"ref"`
	RequestRef      contract.Ref             `json:"request_ref"`
	AttemptRef      contract.Ref             `json:"attempt_ref"`
	Outcome         AcquireInspectionOutcome `json:"outcome"`
	ObservationRef  contract.Ref             `json:"observation_ref,omitempty"`
	InspectedAt     time.Time                `json:"inspected_at"`
	ExpiresAt       time.Time                `json:"expires_at"`
	Digest          string                   `json:"digest"`
}

func SealAcquireRequestV1(in AcquireRequestV1) (AcquireRequestV1, error) {
	in.ContractVersion, in.ObjectKind, in.EffectKind = ConnectorContractVersionV1, AcquireRequestObjectKindV1, AcquireEffectKindV1
	in.RequestedAt, in.ExpiresAt = in.RequestedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	d, err := contract.Digest(in)
	if err != nil {
		return AcquireRequestV1{}, err
	}
	in.Ref.Digest, in.Digest = d, d
	if err := in.Validate(in.RequestedAt); err != nil {
		return AcquireRequestV1{}, err
	}
	return in, nil
}

func (in AcquireRequestV1) Validate(now time.Time) error {
	if in.ContractVersion != ConnectorContractVersionV1 || in.ObjectKind != AcquireRequestObjectKindV1 || in.EffectKind != AcquireEffectKindV1 || strings.TrimSpace(in.TenantID) == "" || !validSourceKind(in.SourceKind) || in.RequestedAt.IsZero() || !in.ExpiresAt.After(in.RequestedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: acquire request", contract.ErrInvalidArgument)
	}
	for _, ref := range []contract.Ref{in.Ref, in.SourceSubjectRef, in.ConnectorRef, in.AuthorityRef, in.PolicyRef, in.ScopeRef, in.BudgetRef, in.OperationRef, in.AttemptRef, in.PermitRef, in.PrepareEnforcementRef, in.ExecuteEnforcementRef} {
		if ref.Validate() != nil {
			return contract.ErrInvalidArgument
		}
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, err := contract.Digest(c)
	if err != nil {
		return err
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: acquire request digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func SealAcquireObservationV1(in AcquireObservationV1) (AcquireObservationV1, error) {
	in.ContractVersion, in.ObjectKind = ConnectorContractVersionV1, AcquireObservationObjectKindV1
	in.ProvenanceRefs = contract.NormalizeRefs(in.ProvenanceRefs)
	in.ObservedAt, in.ExpiresAt = in.ObservedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	d, err := contract.Digest(in)
	if err != nil {
		return AcquireObservationV1{}, err
	}
	in.Ref.Digest, in.Digest = d, d
	if err := in.Validate(in.ObservedAt); err != nil {
		return AcquireObservationV1{}, err
	}
	return in, nil
}

func (in AcquireObservationV1) Validate(now time.Time) error {
	if in.ContractVersion != ConnectorContractVersionV1 || in.ObjectKind != AcquireObservationObjectKindV1 || strings.TrimSpace(in.ContentDigest) == "" || strings.TrimSpace(in.SourceVersion) == "" || strings.TrimSpace(in.License) == "" || strings.TrimSpace(in.Scope) == "" || strings.TrimSpace(in.Sensitivity) == "" || len(in.ProvenanceRefs) == 0 || in.ObservedAt.IsZero() || !in.ExpiresAt.After(in.ObservedAt) || !in.ExpiresAt.After(now) || !slices.Equal(in.ProvenanceRefs, contract.NormalizeRefs(in.ProvenanceRefs)) {
		return fmt.Errorf("%w: acquire observation", contract.ErrInvalidArgument)
	}
	for _, ref := range append([]contract.Ref{in.Ref, in.RequestRef, in.AttemptRef, in.ProviderReceiptRef, in.AssetRef}, in.ProvenanceRefs...) {
		if ref.Validate() != nil {
			return contract.ErrInvalidArgument
		}
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, err := contract.Digest(c)
	if err != nil {
		return err
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: acquire observation digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func SealAcquireInspectionV1(in AcquireInspectionV1) (AcquireInspectionV1, error) {
	in.ContractVersion, in.ObjectKind = ConnectorContractVersionV1, AcquireInspectionObjectKindV1
	in.InspectedAt, in.ExpiresAt = in.InspectedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	d, err := contract.Digest(in)
	if err != nil {
		return AcquireInspectionV1{}, err
	}
	in.Ref.Digest, in.Digest = d, d
	if err := in.Validate(in.InspectedAt); err != nil {
		return AcquireInspectionV1{}, err
	}
	return in, nil
}

func (in AcquireInspectionV1) Validate(now time.Time) error {
	if in.ContractVersion != ConnectorContractVersionV1 || in.ObjectKind != AcquireInspectionObjectKindV1 || (in.Outcome != AcquireObserved && in.Outcome != AcquireConfirmedNotPersisted && in.Outcome != AcquireIndeterminate) || in.Ref.Validate() != nil || in.RequestRef.Validate() != nil || in.AttemptRef.Validate() != nil || in.InspectedAt.IsZero() || !in.ExpiresAt.After(in.InspectedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: acquire inspection", contract.ErrInvalidArgument)
	}
	if in.Outcome == AcquireObserved {
		if in.ObservationRef.Validate() != nil {
			return contract.ErrInvalidArgument
		}
	} else if in.ObservationRef != (contract.Ref{}) {
		return contract.ErrInvalidArgument
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, err := contract.Digest(c)
	if err != nil {
		return err
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: acquire inspection digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func validSourceKind(in SourceKind) bool {
	return in == SourceFile || in == SourceDatabase || in == SourceCode || in == SourceAPI || in == SourceRule || in == SourceManual || in == SourceExternal
}
