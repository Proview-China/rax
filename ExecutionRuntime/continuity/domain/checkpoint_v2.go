package domain

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

// CheckpointManifestControllerV2 owns Continuity Manifest/Seal facts only. It
// does not coordinate a barrier, inspect Provider state, commit Runtime
// consistency, or execute Restore.
type CheckpointManifestControllerV2 struct {
	repository ports.CheckpointManifestRepositoryV2
}

func NewCheckpointManifestControllerV2(repository ports.CheckpointManifestRepositoryV2) (*CheckpointManifestControllerV2, error) {
	if nilInterfaceV2(repository) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "checkpoint_manifest_repository", "is required")
	}
	return &CheckpointManifestControllerV2{repository: repository}, nil
}

func nilInterfaceV2(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (c *CheckpointManifestControllerV2) CreateCheckpointManifestV2(
	ctx context.Context,
	request ports.CreateCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, bool, error) {
	if !request.ExpectAbsent {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "expect_absent", "create-once requires expectAbsent=true")
	}
	if err := request.Candidate.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if request.Candidate.Revision != 1 || request.Candidate.State != contract.ManifestCollecting {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "manifest_create", "revision 1 collecting fact is required")
	}
	return c.repository.CreateCheckpointManifestFactV2(ctx, request.Candidate.Clone())
}

func (c *CheckpointManifestControllerV2) InspectCheckpointManifestV2(
	ctx context.Context,
	request ports.InspectCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	return c.repository.InspectCheckpointManifestV2(ctx, request)
}

func (c *CheckpointManifestControllerV2) InspectCurrentCheckpointManifestV2(
	ctx context.Context,
	request ports.InspectCurrentCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, error) {
	if err := request.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	return c.repository.InspectCurrentCheckpointManifestV2(ctx, request)
}

func (c *CheckpointManifestControllerV2) CompareAndSwapCheckpointManifestV2(
	ctx context.Context,
	request ports.CompareAndSwapCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, bool, error) {
	if err := request.Expected.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if err := request.Next.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if request.Next.ManifestID != request.Expected.Exact().ID || request.Next.Revision != request.Expected.Exact().Revision+1 {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_revision", "next fact must advance the exact expected ref")
	}
	current, err := c.repository.InspectCheckpointManifestV2(ctx, ports.InspectCheckpointManifestRequestV2{Ref: request.Expected})
	if err != nil {
		// A replay after a lost reply may already have advanced current. The
		// repository recognizes only the exact expected->next replay.
		return c.repository.CompareAndSwapCheckpointManifestFactV2(ctx, request.Expected, request.Next.Clone())
	}
	if err := validateManifestIdentityV2(current, request.Next); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if err := contract.AdvanceCheckpointManifestStateV2(current.State, request.Next.State); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	return c.repository.CompareAndSwapCheckpointManifestFactV2(ctx, request.Expected, request.Next.Clone())
}

func (c *CheckpointManifestControllerV2) CreateCheckpointManifestSealV2(
	ctx context.Context,
	request ports.CreateCheckpointManifestSealRequestV2,
) (contract.CheckpointManifestSealFactV2, bool, error) {
	seal := request.Seal.Clone()
	if err := seal.Validate(); err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	manifest, err := c.repository.InspectCurrentCheckpointManifestV2(ctx, ports.InspectCurrentCheckpointManifestRequestV2{
		TenantID: seal.ManifestRef.Exact().TenantID, ScopeDigest: seal.ManifestRef.Exact().ScopeDigest,
		ManifestID: seal.ManifestRef.Exact().ID, Owner: seal.ManifestRef.Exact().Owner,
	})
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	if err := contract.ValidateCheckpointManifestSealBindingV2(manifest, seal); err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	return c.repository.CreateCheckpointManifestSealFactV2(ctx, seal)
}

func (c *CheckpointManifestControllerV2) InspectCheckpointManifestSealV2(
	ctx context.Context,
	request ports.InspectCheckpointManifestSealRequestV2,
) (contract.CheckpointManifestSealFactV2, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	return c.repository.InspectCheckpointManifestSealV2(ctx, request)
}

func validateManifestIdentityV2(current, next contract.CheckpointManifestFactV2) error {
	currentFrames, err := contract.ExactRefSetDigestV2(current.ContextFrameRefs)
	if err != nil {
		return err
	}
	nextFrames, err := contract.ExactRefSetDigestV2(next.ContextFrameRefs)
	if err != nil {
		return err
	}
	if current.ManifestID != next.ManifestID || current.Owner != next.Owner || current.Scope != next.Scope ||
		current.IdempotencyKey != next.IdempotencyKey ||
		!current.CheckpointAttemptRef.Equal(next.CheckpointAttemptRef) ||
		!current.BarrierRef.Equal(next.BarrierRef) ||
		!current.EffectCutRef.Equal(next.EffectCutRef) ||
		current.TimelineCut != next.TimelineCut ||
		!current.ContextGenerationRef.Equal(next.ContextGenerationRef) ||
		currentFrames != nextFrames ||
		current.RuntimeParticipantSetDigest != next.RuntimeParticipantSetDigest ||
		current.RequiredParticipantSetDigest != next.RequiredParticipantSetDigest ||
		current.CreatedUnixNano != next.CreatedUnixNano {
		return contract.NewError(contract.ErrRevisionConflict, "manifest_identity", "immutable manifest identity or frozen owner inputs changed")
	}
	return nil
}

var _ ports.CheckpointManifestGovernancePortV2 = (*CheckpointManifestControllerV2)(nil)
