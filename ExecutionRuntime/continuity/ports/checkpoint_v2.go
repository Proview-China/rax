package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type CreateCheckpointManifestRequestV2 struct {
	Candidate    contract.CheckpointManifestFactV2 `json:"candidate"`
	ExpectAbsent bool                              `json:"expect_absent"`
}

type CompareAndSwapCheckpointManifestRequestV2 struct {
	Expected contract.CheckpointManifestRefV2  `json:"expected"`
	Next     contract.CheckpointManifestFactV2 `json:"next"`
}

type CreateCheckpointManifestSealRequestV2 struct {
	Seal contract.CheckpointManifestSealFactV2 `json:"seal"`
}

type InspectCheckpointManifestRequestV2 struct {
	Ref contract.CheckpointManifestRefV2 `json:"ref"`
}

type InspectCurrentCheckpointManifestRequestV2 struct {
	TenantID    string                `json:"tenant_id"`
	ScopeDigest string                `json:"scope_digest"`
	ManifestID  string                `json:"manifest_id"`
	Owner       contract.OwnerBinding `json:"owner"`
}

func (r InspectCurrentCheckpointManifestRequestV2) Validate() error {
	for field, value := range map[string]string{
		"tenant_id": r.TenantID, "scope_digest": r.ScopeDigest, "manifest_id": r.ManifestID,
	} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if r.Owner.ComponentID != contract.ContinuityComponentID ||
		r.Owner.Capability != contract.CheckpointManifestCapabilityV2 ||
		r.Owner.FactKind != "checkpoint_manifest_fact_v2" {
		return contract.NewError(contract.ErrInvalidArgument, "owner_binding", "current reader requires the exact Continuity Manifest owner")
	}
	return nil
}

type InspectCheckpointManifestSealRequestV2 struct {
	Ref contract.CheckpointManifestSealRefV2 `json:"ref"`
}

// CheckpointManifestReaderV2 separates exact historical reads from the
// manifest Owner's current pointer. Neither read grants Runtime consistency or
// Restore eligibility.
type CheckpointManifestReaderV2 interface {
	InspectCheckpointManifestV2(context.Context, InspectCheckpointManifestRequestV2) (contract.CheckpointManifestFactV2, error)
	InspectCurrentCheckpointManifestV2(context.Context, InspectCurrentCheckpointManifestRequestV2) (contract.CheckpointManifestFactV2, error)
	InspectCheckpointManifestSealV2(context.Context, InspectCheckpointManifestSealRequestV2) (contract.CheckpointManifestSealFactV2, error)
}

// CheckpointManifestRepositoryV2 persists immutable manifest revisions and
// revision-1 seals. It exposes no delete or seal-CAS operation by design.
type CheckpointManifestRepositoryV2 interface {
	CheckpointManifestReaderV2
	CreateCheckpointManifestFactV2(context.Context, contract.CheckpointManifestFactV2) (contract.CheckpointManifestFactV2, bool, error)
	CompareAndSwapCheckpointManifestFactV2(context.Context, contract.CheckpointManifestRefV2, contract.CheckpointManifestFactV2) (contract.CheckpointManifestFactV2, bool, error)
	CreateCheckpointManifestSealFactV2(context.Context, contract.CheckpointManifestSealFactV2) (contract.CheckpointManifestSealFactV2, bool, error)
}

// CheckpointManifestGovernancePortV2 is the Continuity Owner public port. The
// bool result is true only for an exact idempotent replay of an already durable
// write. Lost replies are recovered by Inspect of the original identity.
type CheckpointManifestGovernancePortV2 interface {
	CheckpointManifestReaderV2
	CreateCheckpointManifestV2(context.Context, CreateCheckpointManifestRequestV2) (contract.CheckpointManifestFactV2, bool, error)
	CompareAndSwapCheckpointManifestV2(context.Context, CompareAndSwapCheckpointManifestRequestV2) (contract.CheckpointManifestFactV2, bool, error)
	CreateCheckpointManifestSealV2(context.Context, CreateCheckpointManifestSealRequestV2) (contract.CheckpointManifestSealFactV2, bool, error)
}
