package ports

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const RunSettlementPlanAdmissionContractVersionV3 = "3.0.0"

type RunSettlementDeclarationRefV3 struct {
	ID                 string        `json:"id"`
	Revision           core.Revision `json:"revision"`
	Digest             core.Digest   `json:"digest"`
	BindingSetID       string        `json:"binding_set_id"`
	BindingSetRevision core.Revision `json:"binding_set_revision"`
	BindingRevision    core.Revision `json:"binding_revision"`
	ComponentID        ComponentIDV2 `json:"component_id"`
}

func (r RunSettlementDeclarationRefV3) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || validateEvidenceIDV2(r.BindingSetID) != nil || r.Revision != 1 || r.BindingSetRevision == 0 || r.BindingRevision == 0 || ValidateNamespacedNameV2(NamespacedNameV2(r.ComponentID)) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run settlement declaration ref is incomplete")
	}
	return r.Digest.Validate()
}

type RunSettlementDeclarationFactV3 struct {
	ContractVersion    string                       `json:"contract_version"`
	ID                 string                       `json:"id"`
	Revision           core.Revision                `json:"revision"`
	Digest             core.Digest                  `json:"digest"`
	BindingSetID       string                       `json:"binding_set_id"`
	BindingSetRevision core.Revision                `json:"binding_set_revision"`
	BindingRevision    core.Revision                `json:"binding_revision"`
	ComponentID        ComponentIDV2                `json:"component_id"`
	BindingID          string                       `json:"binding_id"`
	ManifestDigest     core.Digest                  `json:"manifest_digest"`
	ArtifactDigest     core.Digest                  `json:"artifact_digest"`
	Requirements       []RunSettlementRequirementV2 `json:"requirements"`
	ExpiresUnixNano    int64                        `json:"expires_unix_nano"`
}

func (f RunSettlementDeclarationFactV3) Validate() error {
	if f.ContractVersion != RunSettlementPlanAdmissionContractVersionV3 || validateEvidenceIDV2(f.ID) != nil || validateEvidenceIDV2(f.BindingSetID) != nil || validateEvidenceIDV2(f.BindingID) != nil || f.Revision != 1 || f.BindingSetRevision == 0 || f.BindingRevision == 0 || f.Requirements == nil || len(f.Requirements) > MaxManifestSetEntries || f.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run settlement declaration identity, explicit requirement set and TTL are required")
	}
	if ValidateNamespacedNameV2(NamespacedNameV2(f.ComponentID)) != nil || f.ManifestDigest.Validate() != nil || f.ArtifactDigest.Validate() != nil || f.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run settlement declaration binding or digest is invalid")
	}
	if err := validateDeclarationRequirementsV3(f.Requirements); err != nil {
		return err
	}
	digest, err := runSettlementDeclarationDigestV3(f)
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run settlement declaration digest drifted")
	}
	return nil
}

func SealRunSettlementDeclarationFactV3(f RunSettlementDeclarationFactV3) (RunSettlementDeclarationFactV3, error) {
	f.Digest = EvidenceGenesisDigestV2
	if err := validateDeclarationRequirementsV3(f.Requirements); err != nil {
		return RunSettlementDeclarationFactV3{}, err
	}
	digest, err := runSettlementDeclarationDigestV3(f)
	if err != nil {
		return RunSettlementDeclarationFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f RunSettlementDeclarationFactV3) RefV3() (RunSettlementDeclarationRefV3, error) {
	if err := f.Validate(); err != nil {
		return RunSettlementDeclarationRefV3{}, err
	}
	return RunSettlementDeclarationRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest, BindingSetID: f.BindingSetID, BindingSetRevision: f.BindingSetRevision, BindingRevision: f.BindingRevision, ComponentID: f.ComponentID}, nil
}

func runSettlementDeclarationDigestV3(f RunSettlementDeclarationFactV3) (core.Digest, error) {
	f.Digest = ""
	f.Requirements = normalizedRunSettlementRequirementsV3(f.Requirements)
	return core.CanonicalJSONDigest("praxis.runtime.run-plan-admission", RunSettlementPlanAdmissionContractVersionV3, "RunSettlementDeclarationFactV3", f)
}

type RunSettlementBaselinePolicyRefV3 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r RunSettlementBaselinePolicyRefV3) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run settlement baseline policy ref is incomplete")
	}
	return r.Digest.Validate()
}

type RunSettlementBaselinePolicyFactV3 struct {
	ContractVersion      string                       `json:"contract_version"`
	ID                   string                       `json:"id"`
	Revision             core.Revision                `json:"revision"`
	Digest               core.Digest                  `json:"digest"`
	RunID                core.AgentRunID              `json:"run_id"`
	RunIdentityDigest    core.Digest                  `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest                  `json:"execution_scope_digest"`
	Requirements         []RunSettlementRequirementV2 `json:"requirements"`
	PolicyOwner          EvidenceProducerBindingRefV2 `json:"policy_owner"`
	ExpiresUnixNano      int64                        `json:"expires_unix_nano"`
}

func (f RunSettlementBaselinePolicyFactV3) Validate() error {
	if f.ContractVersion != RunSettlementPlanAdmissionContractVersionV3 || validateEvidenceIDV2(f.ID) != nil || validateEvidenceIDV2(string(f.RunID)) != nil || f.Revision == 0 || len(f.Requirements) == 0 || len(f.Requirements) > MaxManifestSetEntries || f.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run settlement baseline policy identity, requirements and TTL are required")
	}
	if f.RunIdentityDigest.Validate() != nil || f.ExecutionScopeDigest.Validate() != nil || f.PolicyOwner.Validate() != nil || f.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run settlement baseline policy binding is invalid")
	}
	if err := validateDeclarationRequirementsV3(f.Requirements); err != nil {
		return err
	}
	digest, err := runSettlementBaselinePolicyDigestV3(f)
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run settlement baseline policy digest drifted")
	}
	return nil
}

func SealRunSettlementBaselinePolicyFactV3(f RunSettlementBaselinePolicyFactV3) (RunSettlementBaselinePolicyFactV3, error) {
	f.Digest = EvidenceGenesisDigestV2
	digest, err := runSettlementBaselinePolicyDigestV3(f)
	if err != nil {
		return RunSettlementBaselinePolicyFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f RunSettlementBaselinePolicyFactV3) RefV3() (RunSettlementBaselinePolicyRefV3, error) {
	if err := f.Validate(); err != nil {
		return RunSettlementBaselinePolicyRefV3{}, err
	}
	return RunSettlementBaselinePolicyRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest}, nil
}

func runSettlementBaselinePolicyDigestV3(f RunSettlementBaselinePolicyFactV3) (core.Digest, error) {
	f.Digest = ""
	f.Requirements = normalizedRunSettlementRequirementsV3(f.Requirements)
	return core.CanonicalJSONDigest("praxis.runtime.run-plan-admission", RunSettlementPlanAdmissionContractVersionV3, "RunSettlementBaselinePolicyFactV3", f)
}

type RunSettlementPlanCertificationRefV3 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

type RunSettlementPlanCertificationAssociationV3 struct {
	Certification        RunSettlementPlanCertificationRefV3 `json:"certification"`
	RunID                core.AgentRunID                     `json:"run_id"`
	RunIdentityDigest    core.Digest                         `json:"run_identity_digest"`
	ExecutionScopeDigest core.Digest                         `json:"execution_scope_digest"`
	Plan                 RunSettlementPlanRefV2              `json:"plan"`
}

func (a RunSettlementPlanCertificationAssociationV3) Validate() error {
	if err := a.Certification.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(a.RunID)) != nil || a.RunIdentityDigest.Validate() != nil || a.ExecutionScopeDigest.Validate() != nil || a.Plan.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run Plan certification association is incomplete")
	}
	return nil
}

func NewRunSettlementPlanCertificationAssociationV3(run core.AgentRunRecord, plan RunSettlementPlanFactV2, certification RunSettlementPlanCertificationRefV3) (RunSettlementPlanCertificationAssociationV3, error) {
	if err := certification.Validate(); err != nil {
		return RunSettlementPlanCertificationAssociationV3{}, err
	}
	planRef, err := plan.RefV2()
	if err != nil {
		return RunSettlementPlanCertificationAssociationV3{}, err
	}
	identity, err := RunIdentityDigestV2(run)
	if err != nil || plan.RunID != run.ID || plan.RunIdentityDigest != identity || !SameExecutionScopeV2(plan.ExecutionScope, run.Scope) {
		return RunSettlementPlanCertificationAssociationV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "certification association Run and Plan differ")
	}
	association := RunSettlementPlanCertificationAssociationV3{Certification: certification, RunID: run.ID, RunIdentityDigest: identity, ExecutionScopeDigest: plan.ExecutionScopeDigest, Plan: planRef}
	return association, association.Validate()
}

func (r RunSettlementPlanCertificationRefV3) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run settlement Plan certification ref is incomplete")
	}
	return r.Digest.Validate()
}

type RunSettlementPlanCertificationFactV3 struct {
	ContractVersion      string                           `json:"contract_version"`
	ID                   string                           `json:"id"`
	Revision             core.Revision                    `json:"revision"`
	Digest               core.Digest                      `json:"digest"`
	RunID                core.AgentRunID                  `json:"run_id"`
	RunIdentityDigest    core.Digest                      `json:"run_identity_digest"`
	ExecutionScope       core.ExecutionScope              `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                      `json:"execution_scope_digest"`
	Plan                 RunSettlementPlanRefV2           `json:"plan"`
	BindingSet           RunBindingSetRefV2               `json:"binding_set"`
	BaselinePolicy       RunSettlementBaselinePolicyRefV3 `json:"baseline_policy"`
	Declarations         []RunSettlementDeclarationRefV3  `json:"declarations"`
	CertificationOwner   EvidenceProducerBindingRefV2     `json:"certification_owner"`
	CreatedUnixNano      int64                            `json:"created_unix_nano"`
	ExpiresUnixNano      int64                            `json:"expires_unix_nano"`
}

func (f RunSettlementPlanCertificationFactV3) Validate() error {
	if f.ContractVersion != RunSettlementPlanAdmissionContractVersionV3 || validateEvidenceIDV2(f.ID) != nil || validateEvidenceIDV2(string(f.RunID)) != nil || f.Revision != 1 || len(f.Declarations) == 0 || len(f.Declarations) > MaxManifestSetEntries || f.CreatedUnixNano <= 0 || f.ExpiresUnixNano <= f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run settlement Plan certification identity, declarations and TTL are required")
	}
	if f.RunIdentityDigest.Validate() != nil || f.ExecutionScopeDigest.Validate() != nil || f.Plan.Validate() != nil || f.BindingSet.Validate() != nil || f.BaselinePolicy.Validate() != nil || f.CertificationOwner.Validate() != nil || f.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Run settlement Plan certification references are invalid")
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, _ := ExecutionScopeDigestV2(f.ExecutionScope)
	if scopeDigest != f.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run settlement Plan certification scope drifted")
	}
	seen := map[ComponentIDV2]struct{}{}
	for _, declaration := range f.Declarations {
		if err := declaration.Validate(); err != nil {
			return err
		}
		if _, exists := seen[declaration.ComponentID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run settlement Plan certification contains duplicate component declarations")
		}
		seen[declaration.ComponentID] = struct{}{}
	}
	digest, err := runSettlementPlanCertificationDigestV3(f)
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run settlement Plan certification digest drifted")
	}
	return nil
}

func SealRunSettlementPlanCertificationFactV3(f RunSettlementPlanCertificationFactV3) (RunSettlementPlanCertificationFactV3, error) {
	f.Digest = EvidenceGenesisDigestV2
	digest, err := runSettlementPlanCertificationDigestV3(f)
	if err != nil {
		return RunSettlementPlanCertificationFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f RunSettlementPlanCertificationFactV3) RefV3() (RunSettlementPlanCertificationRefV3, error) {
	if err := f.Validate(); err != nil {
		return RunSettlementPlanCertificationRefV3{}, err
	}
	return RunSettlementPlanCertificationRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest}, nil
}

func runSettlementPlanCertificationDigestV3(f RunSettlementPlanCertificationFactV3) (core.Digest, error) {
	f.Digest = ""
	f.Declarations = append([]RunSettlementDeclarationRefV3{}, f.Declarations...)
	sort.Slice(f.Declarations, func(i, j int) bool { return f.Declarations[i].ComponentID < f.Declarations[j].ComponentID })
	return core.CanonicalJSONDigest("praxis.runtime.run-plan-admission", RunSettlementPlanAdmissionContractVersionV3, "RunSettlementPlanCertificationFactV3", f)
}

type CertifyRunSettlementPlanRequestV3 struct {
	CertificationID string                           `json:"certification_id"`
	Run             core.AgentRunRecord              `json:"run"`
	Plan            RunSettlementPlanFactV2          `json:"plan"`
	BaselinePolicy  RunSettlementBaselinePolicyRefV3 `json:"baseline_policy"`
	Owner           EvidenceProducerBindingRefV2     `json:"certification_owner"`
	TTL             time.Duration                    `json:"ttl"`
}

type RunSettlementDeclarationReaderV3 interface {
	InspectRunSettlementDeclarationV3(context.Context, string, ComponentIDV2) (RunSettlementDeclarationFactV3, error)
}

// RunSettlementDeclarationFactPortV3 lets a bound component publish only its
// immutable declaration. It does not grant certification or Run creation.
type RunSettlementDeclarationFactPortV3 interface {
	RunSettlementDeclarationReaderV3
	CreateRunSettlementDeclarationV3(context.Context, RunSettlementDeclarationFactV3) (RunSettlementDeclarationFactV3, error)
}

type RunSettlementBaselinePolicyReaderV3 interface {
	InspectRunSettlementBaselinePolicyV3(context.Context, string) (RunSettlementBaselinePolicyFactV3, error)
}

type RunSettlementBaselinePolicyFactPortV3 interface {
	RunSettlementBaselinePolicyReaderV3
	CreateRunSettlementBaselinePolicyV3(context.Context, RunSettlementBaselinePolicyFactV3) (RunSettlementBaselinePolicyFactV3, error)
}

type RunSettlementPlanCertificationFactPortV3 interface {
	CreateRunSettlementPlanCertificationV3(context.Context, RunSettlementPlanCertificationFactV3) (RunSettlementPlanCertificationFactV3, error)
	InspectRunSettlementPlanCertificationV3(context.Context, core.ExecutionScope, core.AgentRunID) (RunSettlementPlanCertificationFactV3, error)
}

type RunSettlementPlanAdmissionPortV3 interface {
	CertifyRunSettlementPlanV3(context.Context, CertifyRunSettlementPlanRequestV3) (RunSettlementPlanCertificationFactV3, error)
	InspectCertifiedRunSettlementPlanV3(context.Context, core.ExecutionScope, core.AgentRunID) (RunSettlementPlanCertificationFactV3, error)
	ValidateRunSettlementPlanCertificationV3(context.Context, RunSettlementPlanCertificationRefV3, core.AgentRunRecord, RunSettlementPlanFactV2) error
}

func validateDeclarationRequirementsV3(requirements []RunSettlementRequirementV2) error {
	seen := map[NamespacedNameV2]struct{}{}
	for _, requirement := range requirements {
		if err := requirement.Validate(); err != nil {
			return err
		}
		if _, exists := seen[requirement.ID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "declaration contains duplicate Run settlement requirement")
		}
		seen[requirement.ID] = struct{}{}
	}
	return nil
}

func normalizedRunSettlementRequirementsV3(requirements []RunSettlementRequirementV2) []RunSettlementRequirementV2 {
	normalized := append([]RunSettlementRequirementV2{}, requirements...)
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].ID < normalized[j].ID })
	return normalized
}
