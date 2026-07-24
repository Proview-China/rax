package domain

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func inspectContentObjectExactV1(ctx context.Context, metadata ports.MetadataStore, content ports.ContentStore, objectID, expectedManifestDigest, scopeDigest string) (contract.ContentObjectRefV1, []contract.ChunkRef, error) {
	manifest, visible, err := metadata.InspectObject(ctx, objectID)
	if err != nil {
		return contract.ContentObjectRefV1{}, nil, normalizeContentReadBoundaryErrorV1(err, "content_object")
	}
	if err := manifest.Validate(); err != nil {
		return contract.ContentObjectRefV1{}, nil, contract.NewError(contract.ErrContentDigestMismatch, "content_manifest", "object manifest failed validation")
	}
	if !visible {
		return contract.ContentObjectRefV1{}, nil, contract.NewError(contract.ErrPreconditionFailed, "content_object", "object must be visible")
	}
	if manifest.ScopeDigest != scopeDigest {
		return contract.ContentObjectRefV1{}, nil, contract.NewError(contract.ErrRevisionConflict, "content_scope", "object belongs to another execution scope")
	}
	if manifest.Digest != expectedManifestDigest {
		return contract.ContentObjectRefV1{}, nil, contract.NewError(contract.ErrRevisionConflict, "content_manifest", "object manifest changed expected exact digest")
	}
	assembled := make([]byte, 0, manifest.TotalLength)
	for _, ref := range manifest.Chunks {
		present, err := content.HasChunk(ctx, ref)
		if err != nil {
			return contract.ContentObjectRefV1{}, nil, normalizeContentReadBoundaryErrorV1(err, "content_chunk_presence")
		}
		if !present {
			return contract.ContentObjectRefV1{}, nil, contract.NewError(contract.ErrCrossStoreIndeterminate, "content_chunk", "referenced chunk is missing")
		}
		data, err := content.GetChunk(ctx, ref)
		if err != nil {
			return contract.ContentObjectRefV1{}, nil, normalizeContentReadBoundaryErrorV1(err, "content_chunk")
		}
		if int64(len(data)) != ref.Length || contract.DigestBytes(data) != ref.Digest {
			return contract.ContentObjectRefV1{}, nil, contract.NewError(contract.ErrContentDigestMismatch, "content_chunk", "chunk bytes failed exact validation")
		}
		assembled = append(assembled, data...)
	}
	if int64(len(assembled)) != manifest.TotalLength || contract.DigestBytes(assembled) != manifest.ContentDigest {
		return contract.ContentObjectRefV1{}, nil, contract.NewError(contract.ErrContentDigestMismatch, "content_object", "reassembled object failed integrity validation")
	}
	return contract.ContentObjectRefFromManifestV1(manifest), append([]contract.ChunkRef{}, manifest.Chunks...), nil
}
