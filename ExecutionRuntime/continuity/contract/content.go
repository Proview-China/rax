package contract

type ChunkRef struct {
	SchemaVersion string `json:"schema_version"`
	Digest        string `json:"digest"`
	Length        int64  `json:"length"`
}

func (r ChunkRef) Validate() error {
	if err := ValidateToken("chunk_schema_version", r.SchemaVersion); err != nil {
		return err
	}
	if err := ValidateDigest("chunk_digest", r.Digest); err != nil {
		return err
	}
	if r.Length <= 0 {
		return NewError(ErrInvalidArgument, "chunk_length", "must be positive")
	}
	return nil
}

type ObjectManifest struct {
	ContractVersion    string     `json:"contract_version"`
	ObjectID           string     `json:"object_id"`
	SchemaVersion      string     `json:"schema_version"`
	ContentDigest      string     `json:"content_digest"`
	TotalLength        int64      `json:"total_length"`
	Chunks             []ChunkRef `json:"chunks"`
	Compression        string     `json:"compression"`
	EncryptionRef      string     `json:"encryption_ref,omitempty"`
	Classification     string     `json:"classification"`
	OwnerID            string     `json:"owner_id"`
	ScopeDigest        string     `json:"scope_digest"`
	RetentionPolicyRef string     `json:"retention_policy_ref"`
	CreatedUnixNano    int64      `json:"created_unix_nano"`
	Digest             string     `json:"digest"`
}

func (m ObjectManifest) CanonicalDigest() (string, error) {
	copy := m
	copy.Digest = ""
	copy.Chunks = append([]ChunkRef{}, m.Chunks...)
	return CanonicalDigest(copy)
}

func (m ObjectManifest) Validate() error {
	if m.ContractVersion != ContractVersion {
		return NewError(ErrInvalidArgument, "contract_version", "unsupported version")
	}
	for field, value := range map[string]string{
		"object_id": m.ObjectID, "schema_version": m.SchemaVersion,
		"content_digest": m.ContentDigest, "compression": m.Compression,
		"classification": m.Classification, "owner_id": m.OwnerID,
		"scope_digest": m.ScopeDigest, "retention_policy_ref": m.RetentionPolicyRef,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if m.TotalLength <= 0 || m.CreatedUnixNano <= 0 || len(m.Chunks) == 0 {
		return NewError(ErrInvalidArgument, "object_manifest", "length, creation time, and chunks are required")
	}
	if m.Classification == "sensitive" && m.EncryptionRef == "" {
		return NewError(ErrInvalidArgument, "encryption_ref", "sensitive content requires an encryption envelope reference")
	}
	var total int64
	for _, chunk := range m.Chunks {
		if err := chunk.Validate(); err != nil {
			return err
		}
		total += chunk.Length
	}
	if total != m.TotalLength {
		return NewError(ErrContentDigestMismatch, "total_length", "chunk lengths do not match object length")
	}
	expected, err := m.CanonicalDigest()
	if err != nil {
		return err
	}
	if m.Digest == "" || m.Digest != expected {
		return NewError(ErrContentDigestMismatch, "manifest_digest", "canonical digest mismatch")
	}
	return nil
}

type JournalState string

const (
	JournalProposed           JournalState = "proposed"
	JournalMetadataPending    JournalState = "metadata_pending"
	JournalContentStaged      JournalState = "content_staged"
	JournalReferenceCommitted JournalState = "reference_committed"
	JournalVisible            JournalState = "visible"
	JournalClosed             JournalState = "closed"
	JournalUnknownWrite       JournalState = "unknown_write"
	JournalOrphanContent      JournalState = "orphan_content"
	JournalDanglingReference  JournalState = "dangling_reference"
	JournalCorruptContent     JournalState = "corrupt_content"
	JournalCleanupPending     JournalState = "cleanup_pending"
)

type WriteJournal struct {
	JournalID         string        `json:"journal_id"`
	ObjectID          string        `json:"object_id"`
	ObjectDigest      string        `json:"object_digest"`
	ManifestDigest    string        `json:"manifest_digest"`
	State             JournalState  `json:"state"`
	Revision          uint64        `json:"revision"`
	LastInspectionRef string        `json:"last_inspection_ref,omitempty"`
	ResidualRefs      []ResidualRef `json:"residual_refs"`
	UpdatedUnixNano   int64         `json:"updated_unix_nano"`
}

func (j WriteJournal) Validate() error {
	for field, value := range map[string]string{
		"journal_id": j.JournalID, "object_id": j.ObjectID,
		"object_digest": j.ObjectDigest, "manifest_digest": j.ManifestDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if j.Revision == 0 || j.UpdatedUnixNano <= 0 {
		return NewError(ErrInvalidArgument, "journal", "revision and update time are required")
	}
	if !validJournalState(j.State) {
		return NewError(ErrInvalidArgument, "journal_state", "unknown state")
	}
	for _, residual := range j.ResidualRefs {
		if err := residual.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validJournalState(state JournalState) bool {
	switch state {
	case JournalProposed, JournalMetadataPending, JournalContentStaged,
		JournalReferenceCommitted, JournalVisible, JournalClosed,
		JournalUnknownWrite, JournalOrphanContent, JournalDanglingReference,
		JournalCorruptContent, JournalCleanupPending:
		return true
	default:
		return false
	}
}

func AdvanceJournal(current, next JournalState) error {
	allowed := map[JournalState]JournalState{
		JournalProposed:           JournalMetadataPending,
		JournalMetadataPending:    JournalContentStaged,
		JournalContentStaged:      JournalReferenceCommitted,
		JournalReferenceCommitted: JournalVisible,
		JournalVisible:            JournalClosed,
	}
	if allowed[current] == next {
		return nil
	}
	return NewError(ErrInvalidArgument, "journal_transition", "illegal state transition")
}
