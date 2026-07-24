package contract

import (
	"cmp"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const RestoreContextMaterializationContractVersionV1 = "praxis.application/restore-context-materialization/v1"

type RestoreContextRequirementKindV1 string

const (
	RestoreContextRequirementProfileV1   RestoreContextRequirementKindV1 = "profile"
	RestoreContextRequirementToolV1      RestoreContextRequirementKindV1 = "tool_surface"
	RestoreContextRequirementMCPV1       RestoreContextRequirementKindV1 = "mcp_connection"
	RestoreContextRequirementReviewV1    RestoreContextRequirementKindV1 = "review_policy"
	RestoreContextRequirementAuthorityV1 RestoreContextRequirementKindV1 = "authority"
	RestoreContextRequirementBudgetV1    RestoreContextRequirementKindV1 = "budget"
	RestoreContextRequirementBindingV1   RestoreContextRequirementKindV1 = "binding"
)

type RestoreContextRequirementCoordinateV1 struct {
	Kind RestoreContextRequirementKindV1               `json:"kind"`
	Ref  runtimeports.CheckpointExternalExactFactRefV2 `json:"ref"`
}

func (r RestoreContextRequirementCoordinateV1) Validate() error {
	switch r.Kind {
	case RestoreContextRequirementProfileV1, RestoreContextRequirementToolV1, RestoreContextRequirementMCPV1, RestoreContextRequirementReviewV1, RestoreContextRequirementAuthorityV1, RestoreContextRequirementBudgetV1, RestoreContextRequirementBindingV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore Context requirement kind is unsupported")
	}
	return r.Ref.Validate()
}

// RestoreContextMaterializationRequestV1 is a coordination envelope only.
// Every authoritative value is reread by its named Owner; this request grants
// no Context write, Restore Stage, Provider call, or Runtime Activation.
type RestoreContextMaterializationRequestV1 struct {
	ContractVersion   string                                                      `json:"contract_version"`
	ID                string                                                      `json:"id"`
	IdempotencyKey    string                                                      `json:"idempotency_key"`
	Materialization   runtimeports.RestoreMaterializationCurrentProjectionV1      `json:"restore_materialization_current"`
	Stage             runtimeports.RestoreStageDomainResultCurrentProjectionV1    `json:"stage_domain_result_current"`
	SandboxSettlement runtimeports.RestoreStageApplySettlementCurrentProjectionV1 `json:"sandbox_apply_settlement_current"`
	Requirements      []RestoreContextRequirementCoordinateV1                     `json:"requirement_refs"`
	RequestedUnixNano int64                                                       `json:"requested_unix_nano"`
	NotAfterUnixNano  int64                                                       `json:"not_after_unix_nano"`
	Digest            core.Digest                                                 `json:"digest"`
}

func (r RestoreContextMaterializationRequestV1) Clone() RestoreContextMaterializationRequestV1 {
	r.Materialization = r.Materialization.Clone()
	r.Requirements = append([]RestoreContextRequirementCoordinateV1{}, r.Requirements...)
	return r
}

func (r RestoreContextMaterializationRequestV1) DigestV1() (core.Digest, error) {
	copy := r.Clone()
	copy.Digest = ""
	sortRestoreContextRequirementRefsV1(copy.Requirements)
	return core.CanonicalJSONDigest("praxis.application.restore-context-materialization", RestoreContextMaterializationContractVersionV1, "RestoreContextMaterializationRequestV1", copy)
}

func SealRestoreContextMaterializationRequestV1(r RestoreContextMaterializationRequestV1) (RestoreContextMaterializationRequestV1, error) {
	r = r.Clone()
	r.ContractVersion = RestoreContextMaterializationContractVersionV1
	sortRestoreContextRequirementRefsV1(r.Requirements)
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return RestoreContextMaterializationRequestV1{}, err
	}
	r.Digest = digest
	return r, nil
}

func (r RestoreContextMaterializationRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != RestoreContextMaterializationContractVersionV1 || !validSingleCallIDV1(r.ID) || !validSingleCallIDV1(r.IdempotencyKey) || r.Materialization.ValidateCurrent(now) != nil || r.Stage.Validate(now) != nil || r.SandboxSettlement.ValidateCurrent(now) != nil || len(r.Requirements) == 0 || r.RequestedUnixNano <= 0 || r.NotAfterUnixNano <= r.RequestedUnixNano || now.IsZero() || now.UnixNano() < r.RequestedUnixNano || now.UnixNano() >= r.NotAfterUnixNano || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore Context materialization coordination request is incomplete or stale")
	}
	if r.Materialization.Attempt != r.Stage.Fact.RestoreAttempt || r.Materialization.Eligibility != r.Stage.Fact.Eligibility || !runtimeports.SameRestoreStageDomainResultFactRefV1(r.Stage.Fact, r.SandboxSettlement.Fact.DomainResult) || r.Materialization.Attempt.TenantID != r.SandboxSettlement.Fact.TenantID || r.Materialization.Identity.TargetInstance != r.Stage.Fact.Operation.ExecutionScope.Instance || r.Stage.Fact.Operation.ExecutionScope.SandboxLease == nil || r.Materialization.Identity.TargetLease != *r.Stage.Fact.Operation.ExecutionScope.SandboxLease || r.Materialization.Identity.TargetFenceEpoch != r.Stage.Fact.Operation.ExecutionScope.AuthorityEpoch {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Context materialization coordination closure drifted")
	}
	kinds := make(map[RestoreContextRequirementKindV1]struct{}, 7)
	for index, requirement := range r.Requirements {
		if requirement.Validate() != nil || requirement.Ref.TenantID != string(r.Materialization.Attempt.TenantID) || index > 0 && compareRestoreContextRequirementRefV1(r.Requirements[index-1], requirement) >= 0 {
			return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Context requirement coordinates are invalid")
		}
		kinds[requirement.Kind] = struct{}{}
	}
	if len(kinds) != 7 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore Context requires Profile, Tool, MCP, Review, Authority, Budget, and Binding current routes")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Context materialization request digest drifted")
	}
	return nil
}

func sortRestoreContextRequirementRefsV1(values []RestoreContextRequirementCoordinateV1) {
	sort.Slice(values, func(i, j int) bool { return compareRestoreContextRequirementRefV1(values[i], values[j]) < 0 })
}

func compareRestoreContextRequirementRefV1(left, right RestoreContextRequirementCoordinateV1) int {
	if result := cmp.Compare(left.Kind, right.Kind); result != 0 {
		return result
	}
	leftRef, rightRef := left.Ref, right.Ref
	strings := [][2]string{
		{leftRef.TenantID, rightRef.TenantID}, {leftRef.ScopeDigest, rightRef.ScopeDigest},
		{leftRef.ContractVersion, rightRef.ContractVersion}, {leftRef.SchemaRef, rightRef.SchemaRef},
		{leftRef.Owner.BindingSetID, rightRef.Owner.BindingSetID}, {leftRef.Owner.ComponentID, rightRef.Owner.ComponentID},
		{leftRef.Owner.ManifestDigest, rightRef.Owner.ManifestDigest}, {leftRef.Owner.ArtifactDigest, rightRef.Owner.ArtifactDigest},
		{leftRef.Owner.Capability, rightRef.Owner.Capability}, {leftRef.Owner.FactKind, rightRef.Owner.FactKind},
		{leftRef.ID, rightRef.ID}, {leftRef.Digest, rightRef.Digest},
	}
	for _, pair := range strings {
		if result := cmp.Compare(pair[0], pair[1]); result != 0 {
			return result
		}
	}
	if result := cmp.Compare(leftRef.Owner.BindingRevision, rightRef.Owner.BindingRevision); result != 0 {
		return result
	}
	return cmp.Compare(leftRef.Revision, rightRef.Revision)
}
