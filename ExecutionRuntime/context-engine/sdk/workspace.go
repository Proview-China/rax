package sdk

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

type workspaceStateV1 uint8

const (
	workspaceNewV1 workspaceStateV1 = iota
	workspaceOpenV1
	workspaceSealedV1
	workspaceExportedV1
	workspaceAbortedV1
	workspaceDestroyedV1
)

type offlineWorkspaceSealV1 struct {
	id    contract.Digest
	count uint32
	bytes uint64
}

type offlineWorkspaceV1 struct {
	mu     sync.RWMutex
	state  workspaceStateV1
	input  OfflineContentBundleV1
	staged map[string]OfflineContentItemV1
	limits kernel.CompileWorkLimitsV1
	seal   offlineWorkspaceSealV1
}

func newOfflineWorkspaceV1(ctx context.Context, input OfflineContentBundleV1) (*offlineWorkspaceV1, error) {
	cloned, err := cloneBundleContextV1(ctx, input)
	if err != nil {
		return nil, err
	}
	return &offlineWorkspaceV1{state: workspaceNewV1, input: cloned}, nil
}

func (w *offlineWorkspaceV1) Begin(ctx context.Context, limits kernel.CompileWorkLimitsV1) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state != workspaceNewV1 {
		return fmt.Errorf("%w: workspace begin state", contract.ErrConflict)
	}
	w.limits = limits
	w.staged = make(map[string]OfflineContentItemV1)
	w.state = workspaceOpenV1
	return ctx.Err()
}

func (w *offlineWorkspaceV1) GetContextV1(ctx context.Context, ref contract.ContentRef) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w.mu.RLock()
	if w.state != workspaceOpenV1 && w.state != workspaceSealedV1 {
		w.mu.RUnlock()
		return nil, fmt.Errorf("%w: workspace read state", contract.ErrConflict)
	}
	item, ok := w.staged[ref.Ref]
	w.mu.RUnlock()
	if ok {
		if item.Ref != ref {
			return nil, fmt.Errorf("%w: staged content ref", contract.ErrConflict)
		}
		return cloneContextBytesV1(ctx, item.Bytes)
	}
	for i := range w.input.items {
		if w.input.items[i].Ref == ref {
			return cloneContextBytesV1(ctx, w.input.items[i].Bytes)
		}
	}
	return nil, fmt.Errorf("%w: content reference", contract.ErrUnknown)
}

func (w *offlineWorkspaceV1) PutContextV1(ctx context.Context, value []byte) (contract.ContentRef, error) {
	if ctx == nil {
		return contract.ContentRef{}, fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContentRef{}, err
	}
	if len(value) == 0 {
		return contract.ContentRef{}, fmt.Errorf("%w: empty staged content", contract.ErrInvalid)
	}
	digest, err := digestBytesContextV1(ctx, value)
	if err != nil {
		return contract.ContentRef{}, err
	}
	ref := contract.ContentRef{Ref: string(digest), Digest: digest, Length: uint64(len(value))}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state != workspaceOpenV1 {
		return contract.ContentRef{}, fmt.Errorf("%w: workspace write state", contract.ErrConflict)
	}
	if ref.Length > w.limits.MaxOutputRawBytes {
		return contract.ContentRef{}, fmt.Errorf("%w: staged item limit", contract.ErrLimitExceeded)
	}
	if current, ok := w.staged[ref.Ref]; ok {
		if current.Ref != ref || !bytes.Equal(current.Bytes, value) {
			return contract.ContentRef{}, fmt.Errorf("%w: staged content collision", contract.ErrConflict)
		}
		return ref, nil
	}
	cloned, err := cloneContextBytesV1(ctx, value)
	if err != nil {
		return contract.ContentRef{}, err
	}
	w.staged[ref.Ref] = OfflineContentItemV1{Ref: ref, Bytes: cloned}
	return ref, ctx.Err()
}

func cloneContextBytesV1(ctx context.Context, value []byte) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	result := make([]byte, len(value))
	for offset := 0; offset < len(value); offset += kernel.StagedCloneChunkBytesV1 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := offset + kernel.StagedCloneChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		copy(result[offset:end], value[offset:end])
	}
	return result, ctx.Err()
}

func (w *offlineWorkspaceV1) Seal(ctx context.Context) (offlineWorkspaceSealV1, error) {
	if ctx == nil {
		return offlineWorkspaceSealV1{}, fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return offlineWorkspaceSealV1{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state != workspaceOpenV1 {
		return offlineWorkspaceSealV1{}, fmt.Errorf("%w: workspace seal state", contract.ErrConflict)
	}
	items := make([]OfflineContentItemV1, 0, len(w.staged))
	var raw uint64
	for _, item := range w.staged {
		items = append(items, item)
		raw += item.Ref.Length
	}
	bundle, err := newOfflineContentBundleContextV1(ctx, items, sdkLimitsFromCompileV1(w.limits))
	if err != nil {
		return offlineWorkspaceSealV1{}, err
	}
	w.seal = offlineWorkspaceSealV1{id: bundle.ContentSetDigest(), count: uint32(len(items)), bytes: raw}
	w.state = workspaceSealedV1
	return w.seal, ctx.Err()
}

func (w *offlineWorkspaceV1) Export(ctx context.Context, seal offlineWorkspaceSealV1, limits OfflineSDKLimitsV1) (OfflineContentBundleV1, error) {
	if ctx == nil {
		return OfflineContentBundleV1{}, fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return OfflineContentBundleV1{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state != workspaceSealedV1 || seal != w.seal {
		return OfflineContentBundleV1{}, fmt.Errorf("%w: workspace export seal", contract.ErrConflict)
	}
	input, cloneErr := cloneBundleContextV1(ctx, w.input)
	if cloneErr != nil {
		return OfflineContentBundleV1{}, cloneErr
	}
	items := input.items
	seen := make(map[string]OfflineContentItemV1, len(items)+len(w.staged))
	for _, item := range items {
		seen[item.Ref.Ref] = item
	}
	for _, item := range w.staged {
		if current, ok := seen[item.Ref.Ref]; ok {
			if current.Ref != item.Ref || !bytes.Equal(current.Bytes, item.Bytes) {
				return OfflineContentBundleV1{}, fmt.Errorf("%w: workspace export content collision", contract.ErrConflict)
			}
			continue
		}
		cloned, cloneErr := cloneContextBytesV1(ctx, item.Bytes)
		if cloneErr != nil {
			return OfflineContentBundleV1{}, cloneErr
		}
		copy := OfflineContentItemV1{Ref: item.Ref, Bytes: cloned}
		items = append(items, copy)
		seen[item.Ref.Ref] = copy
	}
	bundle, err := newOfflineContentBundleContextV1(ctx, items, limits)
	if err != nil {
		return OfflineContentBundleV1{}, err
	}
	w.state = workspaceExportedV1
	return bundle, ctx.Err()
}

func (w *offlineWorkspaceV1) Abort() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch w.state {
	case workspaceOpenV1, workspaceSealedV1:
		clear(w.staged)
		w.staged = nil
		w.state = workspaceAbortedV1
	case workspaceNewV1, workspaceAbortedV1, workspaceExportedV1, workspaceDestroyedV1:
	}
	return nil
}

func (w *offlineWorkspaceV1) Destroy() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state == workspaceDestroyedV1 {
		return nil
	}
	clear(w.staged)
	w.staged = nil
	w.input = OfflineContentBundleV1{}
	w.seal = offlineWorkspaceSealV1{}
	w.state = workspaceDestroyedV1
	return nil
}

func sdkLimitsFromCompileV1(limits kernel.CompileWorkLimitsV1) OfflineSDKLimitsV1 {
	return OfflineSDKLimitsV1{
		MaxRecipes: 1, MaxCandidates: limits.MaxCandidates, MaxInputContentItems: limits.MaxInputContentItems,
		MaxInputContentItemBytes: limits.MaxInputContentItemBytes, MaxInputRawBytes: limits.MaxInputRawBytes,
		MaxGeneratedContentItems: limits.MaxGeneratedContentItems, MaxGeneratedRawBytes: limits.MaxGeneratedRawBytes,
		MaxOutputContentItems: limits.MaxOutputContentItems, MaxOutputRawBytes: limits.MaxOutputRawBytes,
		MaxTotalTokens: limits.MaxTotalTokens, MaxDiagnostics: hardMaxDiagnosticsV1,
		MaxDiagnosticMessageBytes: hardMaxDiagnosticMessageBytesV1, MaxNonContentWireBytes: hardMaxNonContentWireBytesV1,
		MaxWireRequestBytes: hardWire48MiBV1, MaxWireResponseBytes: hardWire144MiBV1,
	}
}
