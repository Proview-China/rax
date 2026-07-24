package ports

import (
	"cmp"
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const RestoreMaterializationCurrentContractVersionV1 = "1.0.0"

type InspectRestoreMaterializationCurrentRequestV1 struct {
	Attempt     RestoreAttemptRefV2     `json:"restore_attempt"`
	Eligibility RestoreEligibilityRefV2 `json:"restore_eligibility"`
}

func (r InspectRestoreMaterializationCurrentRequestV1) Validate() error {
	if r.Attempt.Validate() != nil || r.Eligibility.Validate() != nil || r.Attempt.TenantID != r.Eligibility.TenantID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore materialization current request is incomplete")
	}
	return nil
}

// RestoreMaterializationCurrentProjectionV1 is a read-only projection over
// the exact Continuity Plan and immutable Checkpoint Manifest closure. It
// grants no Eligibility, Stage, Provider, Context write, or Activation.
type RestoreMaterializationCurrentProjectionV1 struct {
	ContractVersion   string                             `json:"contract_version"`
	Attempt           RestoreAttemptRefV2                `json:"restore_attempt"`
	Eligibility       RestoreEligibilityRefV2            `json:"restore_eligibility"`
	RestorePlan       CheckpointExternalExactFactRefV2   `json:"restore_plan"`
	Consistency       CheckpointConsistencyRefV2         `json:"checkpoint_consistency"`
	ManifestSeal      CheckpointManifestSealRefV2        `json:"manifest_seal"`
	SourceScopeDigest core.Digest                        `json:"source_scope_digest"`
	Identity          RestoreIdentityReservationV2       `json:"identity_reservation"`
	ContextGeneration CheckpointExternalExactFactRefV2   `json:"context_generation"`
	ContextFrames     []CheckpointExternalExactFactRefV2 `json:"context_frames"`
	Memory            []CheckpointExternalExactFactRefV2 `json:"memory_refs"`
	Knowledge         []CheckpointExternalExactFactRefV2 `json:"knowledge_refs"`
	Snapshots         []CheckpointExternalExactFactRefV2 `json:"snapshot_refs"`
	CheckedUnixNano   int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                              `json:"expires_unix_nano"`
	ProjectionDigest  core.Digest                        `json:"projection_digest"`
}

func (p RestoreMaterializationCurrentProjectionV1) Clone() RestoreMaterializationCurrentProjectionV1 {
	p.ContextFrames = append([]CheckpointExternalExactFactRefV2{}, p.ContextFrames...)
	p.Memory = append([]CheckpointExternalExactFactRefV2{}, p.Memory...)
	p.Knowledge = append([]CheckpointExternalExactFactRefV2{}, p.Knowledge...)
	p.Snapshots = append([]CheckpointExternalExactFactRefV2{}, p.Snapshots...)
	return p
}

func (p RestoreMaterializationCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p.Clone()
	copy.ProjectionDigest = ""
	normalizeRestoreMaterializationRefsV1(&copy)
	return core.CanonicalJSONDigest("praxis.runtime.restore-materialization-current", RestoreMaterializationCurrentContractVersionV1, "RestoreMaterializationCurrentProjectionV1", copy)
}

func SealRestoreMaterializationCurrentProjectionV1(p RestoreMaterializationCurrentProjectionV1, now time.Time) (RestoreMaterializationCurrentProjectionV1, error) {
	p = p.Clone()
	normalizeRestoreMaterializationRefsV1(&p)
	p.ContractVersion = RestoreMaterializationCurrentContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return RestoreMaterializationCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.ValidateCurrent(now)
}

func (p RestoreMaterializationCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != RestoreMaterializationCurrentContractVersionV1 || p.Attempt.Validate() != nil || p.Eligibility.Validate() != nil || p.RestorePlan.Validate() != nil || p.Consistency.Validate() != nil || p.ManifestSeal.Validate() != nil || p.SourceScopeDigest.Validate() != nil || p.Identity.Validate() != nil || p.ContextGeneration.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore materialization projection is incomplete or stale")
	}
	if p.Attempt.TenantID != p.Eligibility.TenantID ||
		p.RestorePlan.TenantID != string(p.Attempt.TenantID) ||
		p.RestorePlan.ScopeDigest != string(p.SourceScopeDigest) ||
		p.Consistency.Attempt != p.ManifestSeal.Attempt ||
		p.ManifestSeal.ExactLookup.TenantID != string(p.Attempt.TenantID) ||
		p.ManifestSeal.ExactLookup.ScopeDigest != string(p.SourceScopeDigest) ||
		p.ContextGeneration.TenantID != string(p.Attempt.TenantID) ||
		p.ContextGeneration.ScopeDigest != string(p.SourceScopeDigest) {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore materialization root closure drifted")
	}
	sets := [][]CheckpointExternalExactFactRefV2{p.ContextFrames, p.Memory, p.Knowledge, p.Snapshots}
	for _, values := range sets {
		if len(values) == 0 || len(values) > MaxRestoreGovernanceExternalRefsV2 || !restoreMaterializationRefsCanonicalV1(values) {
			return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore materialization exact ref set is empty or non-canonical")
		}
		for _, ref := range values {
			if ref.Validate() != nil || ref.TenantID != string(p.Attempt.TenantID) || ref.ScopeDigest != string(p.SourceScopeDigest) {
				return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore materialization exact ref crosses tenant or source scope")
			}
		}
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore materialization projection digest drifted")
	}
	return nil
}

// ContainsSnapshotV1 proves only membership in the sealed materialization
// closure. The Snapshot Owner must still Inspect its exact current artifact.
func (p RestoreMaterializationCurrentProjectionV1) ContainsSnapshotV1(expected CheckpointExternalExactFactRefV2) bool {
	index := sort.Search(len(p.Snapshots), func(i int) bool { return compareRestoreMaterializationRefV1(p.Snapshots[i], expected) >= 0 })
	return index < len(p.Snapshots) && p.Snapshots[index] == expected
}

func normalizeRestoreMaterializationRefsV1(p *RestoreMaterializationCurrentProjectionV1) {
	for _, values := range []*[]CheckpointExternalExactFactRefV2{&p.ContextFrames, &p.Memory, &p.Knowledge, &p.Snapshots} {
		sort.Slice(*values, func(i, j int) bool { return compareRestoreMaterializationRefV1((*values)[i], (*values)[j]) < 0 })
	}
}

func restoreMaterializationRefsCanonicalV1(values []CheckpointExternalExactFactRefV2) bool {
	for index := range values {
		if index > 0 && compareRestoreMaterializationRefV1(values[index-1], values[index]) >= 0 {
			return false
		}
	}
	return true
}

func compareRestoreMaterializationRefV1(left, right CheckpointExternalExactFactRefV2) int {
	strings := [][2]string{
		{left.TenantID, right.TenantID},
		{left.ScopeDigest, right.ScopeDigest},
		{left.ContractVersion, right.ContractVersion},
		{left.SchemaRef, right.SchemaRef},
		{left.Owner.BindingSetID, right.Owner.BindingSetID},
		{left.Owner.ComponentID, right.Owner.ComponentID},
		{left.Owner.ManifestDigest, right.Owner.ManifestDigest},
		{left.Owner.ArtifactDigest, right.Owner.ArtifactDigest},
		{left.Owner.Capability, right.Owner.Capability},
		{left.Owner.FactKind, right.Owner.FactKind},
		{left.ID, right.ID},
		{left.Digest, right.Digest},
	}
	for _, values := range strings {
		if result := cmp.Compare(values[0], values[1]); result != 0 {
			return result
		}
	}
	if result := cmp.Compare(left.Owner.BindingRevision, right.Owner.BindingRevision); result != 0 {
		return result
	}
	return cmp.Compare(left.Revision, right.Revision)
}

type RestoreMaterializationCurrentReaderV1 interface {
	InspectRestoreMaterializationCurrentV1(context.Context, InspectRestoreMaterializationCurrentRequestV1) (RestoreMaterializationCurrentProjectionV1, error)
}
