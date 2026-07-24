package sdk

import (
	"context"
	"fmt"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type offlineBundleClosureV1 struct {
	SortedContentRefs []contract.ContentRef `json:"sorted_content_refs"`
	ContentSetDigest  contract.Digest       `json:"content_set_digest"`
}

func NewOfflineContentBundleV1(items []OfflineContentItemV1, limits OfflineSDKLimitsV1) (OfflineContentBundleV1, error) {
	return newOfflineContentBundleContextV1(context.Background(), items, limits)
}

func newOfflineContentBundleContextV1(ctx context.Context, items []OfflineContentItemV1, limits OfflineSDKLimitsV1) (OfflineContentBundleV1, error) {
	if ctx == nil {
		return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, "", "context", "nil context", contract.ErrInvalid)
	}
	if items == nil {
		return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, "", "items", "items must be present", contract.ErrInvalid)
	}
	if limits.MaxInputContentItems == 0 || limits.MaxOutputContentItems == 0 || limits.MaxInputContentItemBytes == 0 || limits.MaxInputRawBytes == 0 || limits.MaxOutputRawBytes == 0 {
		return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, "", "limits", "bundle limits must be positive", contract.ErrInvalid)
	}
	if len(items) > int(limits.MaxOutputContentItems) {
		return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, "", "items", "content item limit exceeded", contract.ErrLimitExceeded)
	}
	cloned := make([]OfflineContentItemV1, len(items))
	seen := make(map[string]struct{}, len(items))
	var total uint64
	for i := range items {
		if err := ctx.Err(); err != nil {
			return OfflineContentBundleV1{}, err
		}
		item := items[i]
		if item.Ref.Validate() != nil || item.Ref.Length == 0 || len(item.Bytes) == 0 {
			return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, "", fmt.Sprintf("items[%d]", i), "positive exact content required", contract.ErrInvalid)
		}
		digest, digestErr := digestBytesContextV1(ctx, item.Bytes)
		if digestErr != nil {
			return OfflineContentBundleV1{}, digestErr
		}
		if uint64(len(item.Bytes)) != item.Ref.Length || digest != item.Ref.Digest {
			return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorConflictV1, "", fmt.Sprintf("items[%d]", i), "content reference mismatch", contract.ErrConflict)
		}
		itemMax := limits.MaxOutputRawBytes
		if itemMax < limits.MaxInputContentItemBytes {
			itemMax = limits.MaxInputContentItemBytes
		}
		if item.Ref.Length > itemMax {
			return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, "", fmt.Sprintf("items[%d]", i), "content item bytes exceeded", contract.ErrLimitExceeded)
		}
		if _, ok := seen[item.Ref.Ref]; ok {
			return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorConflictV1, "", fmt.Sprintf("items[%d].ref", i), "duplicate content ref", contract.ErrConflict)
		}
		seen[item.Ref.Ref] = struct{}{}
		if ^uint64(0)-total < item.Ref.Length {
			return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, "", "items", "content byte count overflow", contract.ErrLimitExceeded)
		}
		total += item.Ref.Length
		value, err := cloneContextBytesV1(ctx, item.Bytes)
		if err != nil {
			return OfflineContentBundleV1{}, err
		}
		cloned[i] = OfflineContentItemV1{Ref: item.Ref, Bytes: value}
	}
	if total > limits.MaxInputRawBytes && total > limits.MaxOutputRawBytes {
		return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, "", "items", "aggregate content bytes exceeded", contract.ErrLimitExceeded)
	}
	sort.Slice(cloned, func(i, j int) bool { return lessContentRefV1(cloned[i].Ref, cloned[j].Ref) })
	refs := make([]contract.ContentRef, len(cloned))
	for i := range cloned {
		refs[i] = cloned[i].Ref
	}
	digest, err := canonicalDigestV1("content-set", struct {
		SortedContentRefs []contract.ContentRef `json:"sorted_content_refs"`
	}{SortedContentRefs: refs}, ctx)
	if err != nil {
		return OfflineContentBundleV1{}, mapErrorV1("", "items", err)
	}
	return OfflineContentBundleV1{items: cloned, contentSetDigest: digest}, nil
}

func (b OfflineContentBundleV1) validateContextV1(ctx context.Context, limits OfflineSDKLimitsV1) error {
	if b.items == nil || b.contentSetDigest.Validate() != nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, "", "content_bundle", "unconstructed content bundle", contract.ErrInvalid)
	}
	rebuilt, err := newOfflineContentBundleContextV1(ctx, b.items, limits)
	if err != nil {
		return err
	}
	if rebuilt.contentSetDigest != b.contentSetDigest {
		return sdkErrorV1(OfflineErrorConflictV1, "", "content_bundle.content_set_digest", "content set digest mismatch", contract.ErrConflict)
	}
	return nil
}

func (b OfflineContentBundleV1) Items() []OfflineContentItemV1 {
	items := make([]OfflineContentItemV1, len(b.items))
	for i := range b.items {
		items[i] = OfflineContentItemV1{Ref: b.items[i].Ref, Bytes: cloneByteChunksV1(b.items[i].Bytes)}
	}
	return items
}

func (b OfflineContentBundleV1) Lookup(ref contract.ContentRef) ([]byte, bool) {
	for i := range b.items {
		if b.items[i].Ref == ref {
			return cloneByteChunksV1(b.items[i].Bytes), true
		}
	}
	return nil, false
}

func (b OfflineContentBundleV1) containsV1(ref contract.ContentRef) bool {
	for i := range b.items {
		if b.items[i].Ref == ref {
			return true
		}
	}
	return false
}

func cloneByteChunksV1(value []byte) []byte {
	if value == nil {
		return nil
	}
	result := make([]byte, len(value))
	for offset := 0; offset < len(value); offset += wireChunkBytesV1 {
		end := offset + wireChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		copy(result[offset:end], value[offset:end])
	}
	return result
}

func (b OfflineContentBundleV1) ContentSetDigest() contract.Digest { return b.contentSetDigest }

func (b OfflineContentBundleV1) closureV1() offlineBundleClosureV1 {
	refs := make([]contract.ContentRef, len(b.items))
	for i := range b.items {
		refs[i] = b.items[i].Ref
	}
	return offlineBundleClosureV1{SortedContentRefs: refs, ContentSetDigest: b.contentSetDigest}
}

func lessContentRefV1(left, right contract.ContentRef) bool {
	if left.Ref != right.Ref {
		return left.Ref < right.Ref
	}
	if left.Digest != right.Digest {
		return left.Digest < right.Digest
	}
	return left.Length < right.Length
}

type bundleStoreV1 struct{ bundle OfflineContentBundleV1 }

func (s bundleStoreV1) GetContextV1(ctx context.Context, ref contract.ContentRef) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i := range s.bundle.items {
		if s.bundle.items[i].Ref == ref {
			return cloneContextBytesV1(ctx, s.bundle.items[i].Bytes)
		}
	}
	return nil, fmt.Errorf("%w: content reference", contract.ErrUnknown)
}

func (bundleStoreV1) PutContextV1(context.Context, []byte) (contract.ContentRef, error) {
	return contract.ContentRef{}, fmt.Errorf("%w: read-only bundle", contract.ErrUnsupported)
}
