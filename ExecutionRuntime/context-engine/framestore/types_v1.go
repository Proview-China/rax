// Package framestore provides the Context Owner's single-node durable store
// for exact Frame closures. It does not install a production composition root,
// claim multi-node HA, or authorize Model/Harness dispatch.
package framestore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

const maxFrameStoreContentsV1 = 1028

// ContentValueV1 is an immutable exact content item captured with a Frame
// closure. Value is always deep-copied on ingress and egress.
type ContentValueV1 struct {
	Ref   contract.ContentRef `json:"ref"`
	Value []byte              `json:"value"`
}

// CurrentStateV1 is the complete Context-owned closure committed atomically
// with a Generation current pointer. It is a store input, not a cross-Owner
// public fact.
type CurrentStateV1 struct {
	Owner      contract.OwnerRef                          `json:"owner"`
	Binding    contract.ContextParentFrameSourceBindingV1 `json:"binding"`
	Frame      contract.ContextFrame                      `json:"frame"`
	Manifest   contract.ContextManifest                   `json:"manifest"`
	Generation contract.ContextGeneration                 `json:"generation"`
	Pointer    contract.ContextGenerationCurrentPointerV1 `json:"pointer"`
	Contents   []ContentValueV1                           `json:"contents"`
}

// CommitReceiptV1 is returned only when the mutation outcome is known. A lost
// reply returns a zero receipt and ErrUnknown; recovery uses InspectCommitV1.
type CommitReceiptV1 struct {
	OperationID string                                     `json:"operation_id"`
	FrameRef    contract.FactRef                           `json:"frame_ref"`
	Pointer     contract.ContextGenerationCurrentPointerV1 `json:"pointer"`
	Created     bool                                       `json:"created"`
	Digest      contract.Digest                            `json:"digest"`
}

func (r CommitReceiptV1) digestValue() (contract.Digest, error) {
	copy := r
	copy.Digest = ""
	return contract.DigestJSON(struct {
		Domain  string
		Receipt CommitReceiptV1
	}{"praxis.context/frame-store-commit-receipt-v1", copy})
}

func (r CommitReceiptV1) Validate() error {
	if !validIDV1(r.OperationID) || r.FrameRef.Validate() != nil || r.Pointer.Validate() != nil || !r.Created || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: frame store commit receipt", contract.ErrInvalid)
	}
	want, err := r.digestValue()
	if err != nil || want != r.Digest {
		return fmt.Errorf("%w: frame store commit receipt digest", contract.ErrConflict)
	}
	return nil
}

func sealCommitReceiptV1(receipt CommitReceiptV1) (CommitReceiptV1, error) {
	receipt.Digest = ""
	digest, err := receipt.digestValue()
	if err != nil {
		return CommitReceiptV1{}, err
	}
	receipt.Digest = digest
	return receipt, receipt.Validate()
}

func normalizeStateV1(state CurrentStateV1) (CurrentStateV1, error) {
	cloned, err := cloneV1(state)
	if err != nil {
		return CurrentStateV1{}, err
	}
	sort.Slice(cloned.Contents, func(i, j int) bool {
		left, right := cloned.Contents[i].Ref, cloned.Contents[j].Ref
		if left.Ref != right.Ref {
			return left.Ref < right.Ref
		}
		if left.Digest != right.Digest {
			return left.Digest < right.Digest
		}
		return left.Length < right.Length
	})
	return cloned, nil
}

func validateStateAtV1(ctx context.Context, state CurrentStateV1, owner contract.OwnerRef, nowUnixNano int64) error {
	if err := checkContextSQLiteV1(ctx); err != nil {
		return err
	}
	if owner.Validate() != nil || state.Owner != owner || state.Binding.Validate() != nil ||
		state.Frame.Validate() != nil || state.Manifest.Validate() != nil ||
		state.Generation.Validate() != nil || state.Pointer.Validate() != nil ||
		nowUnixNano <= 0 {
		return fmt.Errorf("%w: frame store current state", contract.ErrInvalid)
	}
	if nowUnixNano < state.Frame.CreatedUnixNano ||
		nowUnixNano < state.Manifest.CreatedUnixNano ||
		nowUnixNano < state.Generation.CreatedUnixNano {
		return fmt.Errorf("%w: frame store current state clock rollback", contract.ErrConflict)
	}
	frameRef, manifestRef, generationRef, err := stateRefsV1(state)
	if err != nil {
		return err
	}
	subject := state.Binding.Subject
	if state.Binding.Source.ID != state.Frame.ID ||
		subject.FrameRef != frameRef || subject.ManifestRef != manifestRef ||
		subject.GenerationRef != generationRef ||
		state.Frame.ManifestRef != manifestRef ||
		state.Frame.GenerationID != generationRef.ID ||
		state.Frame.Generation != state.Generation.Ordinal ||
		state.Generation.RootFrame != frameRef ||
		state.Frame.Execution.ScopeDigest != subject.ExecutionScopeDigest ||
		state.Manifest.Execution.ScopeDigest != subject.ExecutionScopeDigest ||
		state.Frame.Execution.RunID != subject.RunID ||
		state.Manifest.Execution.RunID != subject.RunID ||
		state.Frame.Execution.Turn != subject.Turn ||
		state.Manifest.Execution.Turn != subject.Turn ||
		state.Frame.Execution.AuthorityDigest != subject.AuthorityDigest ||
		state.Manifest.Execution.AuthorityDigest != subject.AuthorityDigest ||
		state.Pointer.ExecutionScopeDigest != subject.ExecutionScopeDigest ||
		state.Pointer.RunID != subject.RunID ||
		state.Pointer.SessionRef != subject.SessionRef ||
		state.Pointer.Turn != subject.Turn ||
		state.Pointer.GenerationRef != generationRef ||
		state.Pointer.GenerationOrdinal != subject.GenerationOrdinal ||
		state.Pointer.ParentFrameGenerationBindingDigest != subject.ParentFrameGenerationBindingDigest {
		return fmt.Errorf("%w: frame store exact closure", contract.ErrConflict)
	}
	if nowUnixNano >= stateExpiryV1(state) {
		return fmt.Errorf("%w: frame store current state", contract.ErrExpired)
	}
	if len(state.Contents) == 0 || len(state.Contents) > maxFrameStoreContentsV1 {
		return fmt.Errorf("%w: frame store content cardinality", contract.ErrLimitExceeded)
	}
	required := requiredContentRefsV1(state.Manifest, state.Frame)
	if len(required) != len(state.Contents) {
		return fmt.Errorf("%w: frame store content closure cardinality", contract.ErrConflict)
	}
	values := make(map[contract.ContentRef][]byte, len(state.Contents))
	var previous *contract.ContentRef
	for index := range state.Contents {
		item := state.Contents[index]
		if item.Ref.Validate() != nil || len(item.Value) == 0 ||
			uint64(len(item.Value)) != item.Ref.Length ||
			contract.DigestBytes(item.Value) != item.Ref.Digest {
			return fmt.Errorf("%w: frame store content item", contract.ErrConflict)
		}
		if previous != nil && !contentRefLessV1(*previous, item.Ref) {
			return fmt.Errorf("%w: frame store content order", contract.ErrConflict)
		}
		if _, exists := values[item.Ref]; exists {
			return fmt.Errorf("%w: duplicate frame store content", contract.ErrConflict)
		}
		if _, exists := required[item.Ref]; !exists {
			return fmt.Errorf("%w: unbound frame store content", contract.ErrConflict)
		}
		values[item.Ref] = append([]byte(nil), item.Value...)
		copyRef := item.Ref
		previous = &copyRef
	}
	if err := kernel.InspectFrameStagedV1(
		ctx,
		contentViewV1{values: values},
		state.Manifest,
		state.Frame,
		frameStoreInspectLimitsV1(),
	); err != nil {
		return fmt.Errorf("frame store content inspection: %w", err)
	}
	return checkContextSQLiteV1(ctx)
}

func stateRefsV1(state CurrentStateV1) (contract.FactRef, contract.FactRef, contract.FactRef, error) {
	frameDigest, err := state.Frame.DigestValue()
	if err != nil {
		return contract.FactRef{}, contract.FactRef{}, contract.FactRef{}, err
	}
	manifestDigest, err := state.Manifest.DigestValue()
	if err != nil {
		return contract.FactRef{}, contract.FactRef{}, contract.FactRef{}, err
	}
	generationDigest, err := state.Generation.DigestValue()
	if err != nil {
		return contract.FactRef{}, contract.FactRef{}, contract.FactRef{}, err
	}
	return contract.FactRef{ID: state.Frame.ID, Revision: state.Frame.Revision, Digest: frameDigest},
		contract.FactRef{ID: state.Manifest.ID, Revision: state.Manifest.Revision, Digest: manifestDigest},
		contract.FactRef{ID: state.Generation.ID, Revision: state.Generation.Revision, Digest: generationDigest}, nil
}

func stateExpiryV1(state CurrentStateV1) int64 {
	expires := state.Binding.BindingExpiresUnixNano
	for _, candidate := range []int64{
		state.Binding.RecipeExpiresUnixNano,
		state.Binding.AuthorityExpiresUnixNano,
		state.Frame.ExpiresUnixNano,
		state.Manifest.ExpiresUnixNano,
		state.Pointer.ExpiresUnixNano,
	} {
		if candidate < expires {
			expires = candidate
		}
	}
	return expires
}

// stateCreatedFloorV1 is the earliest deterministic instant at which the
// complete persisted closure can be valid. It is deliberately independent of
// the observation clock used for freshness/currentness checks.
func stateCreatedFloorV1(state CurrentStateV1) int64 {
	created := state.Frame.CreatedUnixNano
	for _, candidate := range []int64{
		state.Manifest.CreatedUnixNano,
		state.Generation.CreatedUnixNano,
	} {
		if candidate > created {
			created = candidate
		}
	}
	return created
}

func requiredContentRefsV1(manifest contract.ContextManifest, frame contract.ContextFrame) map[contract.ContentRef]struct{} {
	result := make(map[contract.ContentRef]struct{}, len(manifest.Fragments)+4)
	for _, fragment := range manifest.Fragments {
		result[fragment.Content] = struct{}{}
	}
	result[frame.StablePrefix] = struct{}{}
	if frame.SemiStable != nil {
		result[*frame.SemiStable] = struct{}{}
	}
	result[frame.DynamicTail] = struct{}{}
	result[frame.Rendered] = struct{}{}
	return result
}

func contentRefLessV1(left, right contract.ContentRef) bool {
	if left.Ref != right.Ref {
		return left.Ref < right.Ref
	}
	if left.Digest != right.Digest {
		return left.Digest < right.Digest
	}
	return left.Length < right.Length
}

type contentViewV1 struct {
	values map[contract.ContentRef][]byte
}

func (v contentViewV1) GetContextV1(ctx context.Context, ref contract.ContentRef) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil frame store content context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, ok := v.values[ref]
	if !ok {
		return nil, fmt.Errorf("%w: frame store content", contract.ErrNotFound)
	}
	return append([]byte(nil), value...), nil
}

func (contentViewV1) PutContextV1(context.Context, []byte) (contract.ContentRef, error) {
	return contract.ContentRef{}, fmt.Errorf("%w: frame store content is read-only", contract.ErrUnsupported)
}

func frameStoreInspectLimitsV1() kernel.InspectWorkLimitsV1 {
	return kernel.InspectWorkLimitsV1{
		MaxFragments:        512,
		MaxContentItems:     1028,
		MaxContentItemBytes: 76 * 1024 * 1024,
		MaxRawBytes:         76 * 1024 * 1024,
		StreamChunkBytes:    kernel.StagedStreamChunkBytesV1,
		CloneChunkBytes:     kernel.StagedCloneChunkBytesV1,
	}
}

func cloneV1[T any](value T) (T, error) {
	var result T
	payload, err := json.Marshal(value)
	if err != nil {
		return result, fmt.Errorf("%w: frame store clone", contract.ErrInvalid)
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return result, fmt.Errorf("%w: frame store clone", contract.ErrInvalid)
	}
	return result, nil
}

func validIDV1(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func equalContentValueV1(left, right ContentValueV1) bool {
	return left.Ref == right.Ref && bytes.Equal(left.Value, right.Value)
}
