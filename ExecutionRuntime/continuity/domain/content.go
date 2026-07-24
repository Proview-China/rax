package domain

import (
	"context"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type FaultInjector func(contract.JournalState, contract.WriteJournal) error

type PutObjectRequest struct {
	JournalID          string
	ObjectID           string
	SchemaVersion      string
	Classification     string
	OwnerID            string
	ScopeDigest        string
	RetentionPolicyRef string
	Compression        string
	EncryptionRef      string
	Data               []byte
}

type ContentManager struct {
	metadata  ports.MetadataStore
	content   ports.ContentStore
	clock     Clock
	chunkSize int
	fault     FaultInjector
}

func NewContentManager(metadata ports.MetadataStore, content ports.ContentStore, clock Clock, chunkSize int, fault FaultInjector) (*ContentManager, error) {
	if metadata == nil || content == nil || clock == nil {
		return nil, contract.NewError(contract.ErrInvalidArgument, "content_manager", "metadata, content, and clock are required")
	}
	if chunkSize <= 0 || chunkSize > 16<<20 {
		return nil, contract.NewError(contract.ErrInvalidArgument, "chunk_size", "must be between 1 and 16 MiB")
	}
	return &ContentManager{metadata: metadata, content: content, clock: clock, chunkSize: chunkSize, fault: fault}, nil
}

func (m *ContentManager) Put(ctx context.Context, request PutObjectRequest) (contract.ObjectManifest, contract.WriteJournal, error) {
	manifest, chunks, err := m.buildManifest(request)
	if err != nil {
		return contract.ObjectManifest{}, contract.WriteJournal{}, err
	}
	journal := contract.WriteJournal{
		JournalID: request.JournalID, ObjectID: request.ObjectID,
		ObjectDigest: manifest.ContentDigest, ManifestDigest: manifest.Digest,
		State: contract.JournalProposed, Revision: 1, UpdatedUnixNano: m.clock.Now().UnixNano(),
	}
	if err := m.metadata.CreateJournal(ctx, journal); err != nil {
		return contract.ObjectManifest{}, contract.WriteJournal{}, err
	}
	journal, err = m.continueWrite(ctx, journal, manifest, chunks)
	return manifest, journal, err
}

// Recover inspects the exact existing journal and resumes only that object. It
// never creates a replacement object or journal identity after an unknown write.
func (m *ContentManager) Recover(ctx context.Context, journalID string, data []byte) (contract.WriteJournal, error) {
	journal, err := m.metadata.InspectJournal(ctx, journalID)
	if err != nil {
		return contract.WriteJournal{}, err
	}
	journal.LastInspectionRef = fmt.Sprintf("inspect/%s/revision/%d", journal.JournalID, journal.Revision)
	manifest, _, err := m.metadata.InspectObject(ctx, journal.ObjectID)
	if err != nil {
		return journal, contract.NewError(contract.ErrCrossStoreIndeterminate, "manifest", "exact staged manifest is not inspectable")
	}
	if contract.DigestBytes(data) != journal.ObjectDigest || manifest.Digest != journal.ManifestDigest {
		return journal, contract.NewError(contract.ErrContentDigestMismatch, "recovery_data", "data or manifest does not match the original journal")
	}
	chunks, err := splitByManifest(data, manifest)
	if err != nil {
		return journal, err
	}
	return m.continueWrite(ctx, journal, manifest, chunks)
}

func (m *ContentManager) Read(ctx context.Context, objectID string) ([]byte, contract.ObjectManifest, error) {
	manifest, visible, err := m.metadata.InspectObject(ctx, objectID)
	if err != nil {
		return nil, contract.ObjectManifest{}, err
	}
	if !visible {
		return nil, contract.ObjectManifest{}, contract.NewError(contract.ErrCrossStoreIndeterminate, "object", "object is not visible")
	}
	result := make([]byte, 0, manifest.TotalLength)
	for _, ref := range manifest.Chunks {
		chunk, err := m.content.GetChunk(ctx, ref)
		if err != nil {
			return nil, contract.ObjectManifest{}, err
		}
		result = append(result, chunk...)
	}
	if int64(len(result)) != manifest.TotalLength || contract.DigestBytes(result) != manifest.ContentDigest {
		return nil, contract.ObjectManifest{}, contract.NewError(contract.ErrContentDigestMismatch, "object", "reassembled object failed integrity validation")
	}
	return result, manifest, nil
}

func (m *ContentManager) continueWrite(ctx context.Context, journal contract.WriteJournal, manifest contract.ObjectManifest, chunks [][]byte) (contract.WriteJournal, error) {
	for journal.State != contract.JournalClosed {
		var err error
		switch journal.State {
		case contract.JournalProposed:
			err = m.metadata.StageManifest(ctx, manifest)
			if err == nil {
				journal, err = m.advance(ctx, journal, contract.JournalMetadataPending)
			}
		case contract.JournalMetadataPending:
			for i, chunk := range chunks {
				if err = m.content.PutChunk(ctx, manifest.Chunks[i], chunk); err != nil {
					break
				}
			}
			if err == nil {
				journal, err = m.advance(ctx, journal, contract.JournalContentStaged)
			}
		case contract.JournalContentStaged:
			for _, ref := range manifest.Chunks {
				var present bool
				present, err = m.content.HasChunk(ctx, ref)
				if err != nil || !present {
					if err == nil {
						err = contract.NewError(contract.ErrCrossStoreIndeterminate, "chunk", "staged chunk is missing")
					}
					break
				}
			}
			if err == nil {
				err = m.metadata.CommitObjectReference(ctx, manifest.ObjectID, manifest.ContentDigest)
			}
			if err == nil {
				journal, err = m.advance(ctx, journal, contract.JournalReferenceCommitted)
			}
		case contract.JournalReferenceCommitted:
			err = m.metadata.SetObjectVisible(ctx, manifest.ObjectID, true)
			if err == nil {
				journal, err = m.advance(ctx, journal, contract.JournalVisible)
			}
		case contract.JournalVisible:
			journal, err = m.advance(ctx, journal, contract.JournalClosed)
		default:
			return journal, contract.NewError(contract.ErrCrossStoreIndeterminate, "journal_state", "journal requires explicit inspection")
		}
		if err != nil {
			return journal, err
		}
		if m.fault != nil {
			if err := m.fault(journal.State, journal); err != nil {
				return journal, contract.NewError(contract.ErrCrossStoreIndeterminate, "fault_injection", err.Error())
			}
		}
	}
	return journal, nil
}

func (m *ContentManager) advance(ctx context.Context, current contract.WriteJournal, next contract.JournalState) (contract.WriteJournal, error) {
	if err := contract.AdvanceJournal(current.State, next); err != nil {
		return current, err
	}
	updated := current
	updated.State = next
	updated.Revision++
	updated.UpdatedUnixNano = m.clock.Now().UnixNano()
	if err := m.metadata.CASJournal(ctx, current.Revision, updated); err != nil {
		return current, err
	}
	return updated, nil
}

func (m *ContentManager) buildManifest(request PutObjectRequest) (contract.ObjectManifest, [][]byte, error) {
	for field, value := range map[string]string{
		"journal_id": request.JournalID, "object_id": request.ObjectID,
		"schema_version": request.SchemaVersion, "classification": request.Classification,
		"owner_id": request.OwnerID, "scope_digest": request.ScopeDigest,
		"retention_policy_ref": request.RetentionPolicyRef, "compression": request.Compression,
	} {
		if err := contract.ValidateToken(field, value); err != nil {
			return contract.ObjectManifest{}, nil, err
		}
	}
	if len(request.Data) == 0 {
		return contract.ObjectManifest{}, nil, contract.NewError(contract.ErrInvalidArgument, "data", "must not be empty")
	}
	chunks := make([][]byte, 0, (len(request.Data)+m.chunkSize-1)/m.chunkSize)
	refs := make([]contract.ChunkRef, 0, cap(chunks))
	for offset := 0; offset < len(request.Data); offset += m.chunkSize {
		end := min(offset+m.chunkSize, len(request.Data))
		chunk := append([]byte{}, request.Data[offset:end]...)
		chunks = append(chunks, chunk)
		refs = append(refs, contract.ChunkRef{SchemaVersion: request.SchemaVersion, Digest: contract.DigestBytes(chunk), Length: int64(len(chunk))})
	}
	manifest := contract.ObjectManifest{
		ContractVersion: contract.ContractVersion, ObjectID: request.ObjectID,
		SchemaVersion: request.SchemaVersion, ContentDigest: contract.DigestBytes(request.Data),
		TotalLength: int64(len(request.Data)), Chunks: refs, Compression: request.Compression,
		EncryptionRef: request.EncryptionRef, Classification: request.Classification,
		OwnerID: request.OwnerID, ScopeDigest: request.ScopeDigest,
		RetentionPolicyRef: request.RetentionPolicyRef, CreatedUnixNano: m.clock.Now().UnixNano(),
	}
	digest, err := manifest.CanonicalDigest()
	if err != nil {
		return contract.ObjectManifest{}, nil, err
	}
	manifest.Digest = digest
	if err := manifest.Validate(); err != nil {
		return contract.ObjectManifest{}, nil, err
	}
	return manifest, chunks, nil
}

func splitByManifest(data []byte, manifest contract.ObjectManifest) ([][]byte, error) {
	chunks := make([][]byte, 0, len(manifest.Chunks))
	offset := 0
	for _, ref := range manifest.Chunks {
		end := offset + int(ref.Length)
		if end > len(data) {
			return nil, contract.NewError(contract.ErrContentDigestMismatch, "recovery_data", "chunk boundaries exceed content")
		}
		chunk := append([]byte{}, data[offset:end]...)
		if contract.DigestBytes(chunk) != ref.Digest {
			return nil, contract.NewError(contract.ErrContentDigestMismatch, "recovery_data", fmt.Sprintf("chunk at offset %d changed", offset))
		}
		chunks = append(chunks, chunk)
		offset = end
	}
	if offset != len(data) {
		return nil, contract.NewError(contract.ErrContentDigestMismatch, "recovery_data", "content has trailing bytes")
	}
	return chunks, nil
}
