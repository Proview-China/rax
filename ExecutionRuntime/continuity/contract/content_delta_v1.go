package contract

import (
	"sort"
	"time"
)

const (
	ContentDeltaContractV1   = "praxis.continuity/content-delta/v1"
	ContentDeltaFactSchemaV1 = "praxis.continuity/content-delta-fact/v1"
	ContentDeltaCapabilityV1 = "continuity/content-delta-v1"
)

type ContentObjectRefV1 struct {
	ObjectID           string `json:"object_id"`
	SchemaVersion      string `json:"schema_version"`
	ManifestDigest     string `json:"manifest_digest"`
	ContentDigest      string `json:"content_digest"`
	TotalLength        int64  `json:"total_length"`
	Compression        string `json:"compression"`
	EncryptionRef      string `json:"encryption_ref,omitempty"`
	Classification     string `json:"classification"`
	OwnerID            string `json:"owner_id"`
	ScopeDigest        string `json:"scope_digest"`
	RetentionPolicyRef string `json:"retention_policy_ref"`
}

func (r ContentObjectRefV1) Validate() error {
	for field, value := range map[string]string{
		"object_id": r.ObjectID, "schema_version": r.SchemaVersion,
		"compression": r.Compression, "classification": r.Classification,
		"owner_id": r.OwnerID, "scope_digest": r.ScopeDigest, "retention_policy_ref": r.RetentionPolicyRef,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := ValidateDigest("manifest_digest", r.ManifestDigest); err != nil {
		return err
	}
	if err := ValidateDigest("content_digest", r.ContentDigest); err != nil {
		return err
	}
	if r.EncryptionRef != "" {
		if err := ValidateToken("encryption_ref", r.EncryptionRef); err != nil {
			return err
		}
	}
	if r.TotalLength <= 0 {
		return NewError(ErrInvalidArgument, "total_length", "must be positive")
	}
	if r.Classification == "sensitive" && r.EncryptionRef == "" {
		return NewError(ErrInvalidArgument, "encryption_ref", "sensitive content requires an encryption envelope reference")
	}
	return nil
}

func ContentObjectRefFromManifestV1(manifest ObjectManifest) ContentObjectRefV1 {
	return ContentObjectRefV1{
		ObjectID: manifest.ObjectID, SchemaVersion: manifest.SchemaVersion,
		ManifestDigest: manifest.Digest, ContentDigest: manifest.ContentDigest,
		TotalLength: manifest.TotalLength, Compression: manifest.Compression,
		EncryptionRef: manifest.EncryptionRef, Classification: manifest.Classification,
		OwnerID: manifest.OwnerID, ScopeDigest: manifest.ScopeDigest, RetentionPolicyRef: manifest.RetentionPolicyRef,
	}
}

type ContentDeltaRecipeKindV1 string

const (
	ContentDeltaReuse ContentDeltaRecipeKindV1 = "reuse"
	ContentDeltaAdd   ContentDeltaRecipeKindV1 = "add"
)

func (k ContentDeltaRecipeKindV1) Validate() error {
	if k != ContentDeltaReuse && k != ContentDeltaAdd {
		return NewError(ErrInvalidArgument, "content_delta_recipe_kind", "unknown recipe kind")
	}
	return nil
}

type ContentDeltaRecipeEntryV1 struct {
	Ordinal uint32                   `json:"ordinal"`
	Chunk   ChunkRef                 `json:"chunk"`
	Kind    ContentDeltaRecipeKindV1 `json:"kind"`
}

func (e ContentDeltaRecipeEntryV1) Validate() error {
	if err := e.Chunk.Validate(); err != nil {
		return err
	}
	return e.Kind.Validate()
}

type ContentDeltaSourceProjectionV1 struct {
	Base         ContentObjectRefV1 `json:"base"`
	BaseChunks   []ChunkRef         `json:"base_chunks"`
	Target       ContentObjectRefV1 `json:"target"`
	TargetChunks []ChunkRef         `json:"target_chunks"`
}

func (p ContentDeltaSourceProjectionV1) Validate() error {
	if err := p.Base.Validate(); err != nil {
		return err
	}
	if err := p.Target.Validate(); err != nil {
		return err
	}
	if p.Base.ObjectID == p.Target.ObjectID {
		return NewError(ErrInvalidArgument, "content_delta_objects", "base and target object ids must differ")
	}
	if p.Base.ScopeDigest != p.Target.ScopeDigest {
		return NewError(ErrRevisionConflict, "content_delta_scope", "base and target belong to different scopes")
	}
	if err := validateContentDeltaChunksV1(p.BaseChunks, p.Base.TotalLength, p.Base.SchemaVersion, "base_chunks"); err != nil {
		return err
	}
	return validateContentDeltaChunksV1(p.TargetChunks, p.Target.TotalLength, p.Target.SchemaVersion, "target_chunks")
}

func (p ContentDeltaSourceProjectionV1) Clone() ContentDeltaSourceProjectionV1 {
	result := p
	result.BaseChunks = append([]ChunkRef{}, p.BaseChunks...)
	result.TargetChunks = append([]ChunkRef{}, p.TargetChunks...)
	return result
}

type ContentDeltaFactV1 struct {
	ContractVersion string                      `json:"contract_version"`
	SchemaRef       string                      `json:"schema_ref"`
	DeltaID         string                      `json:"delta_id"`
	Revision        uint64                      `json:"revision"`
	IdempotencyKey  string                      `json:"idempotency_key"`
	RequestDigest   string                      `json:"request_digest"`
	Scope           Scope                       `json:"scope"`
	Owner           OwnerBinding                `json:"owner"`
	Base            ContentObjectRefV1          `json:"base"`
	Target          ContentObjectRefV1          `json:"target"`
	BaseChunks      []ChunkRef                  `json:"base_chunks"`
	TargetRecipe    []ContentDeltaRecipeEntryV1 `json:"target_recipe"`
	ReusedChunks    []ChunkRef                  `json:"reused_chunks"`
	AddedChunks     []ChunkRef                  `json:"added_chunks"`
	RemovedChunks   []ChunkRef                  `json:"removed_chunks"`
	SharedBytes     int64                       `json:"shared_bytes"`
	AddedBytes      int64                       `json:"added_bytes"`
	RemovedBytes    int64                       `json:"removed_bytes"`
	CreatedUnixNano int64                       `json:"created_unix_nano"`
	Digest          string                      `json:"digest"`
}

func (f ContentDeltaFactV1) CanonicalDigest() (string, error) {
	copy := f.Clone()
	copy.Digest = ""
	return CanonicalDigest(copy)
}

func (f ContentDeltaFactV1) Validate() error {
	if f.ContractVersion != ContentDeltaContractV1 || f.SchemaRef != ContentDeltaFactSchemaV1 {
		return NewError(ErrInvalidArgument, "content_delta_contract", "unsupported contract or schema")
	}
	if err := ValidateToken("delta_id", f.DeltaID); err != nil {
		return err
	}
	if err := ValidateToken("idempotency_key", f.IdempotencyKey); err != nil {
		return err
	}
	if err := ValidateDigest("request_digest", f.RequestDigest); err != nil {
		return err
	}
	if f.Revision != 1 || f.CreatedUnixNano <= 0 {
		return NewError(ErrInvalidArgument, "content_delta_fact", "immutable delta requires revision one and creation time")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := validateContentDeltaOwnerV1(f.Owner); err != nil {
		return err
	}
	projection := ContentDeltaSourceProjectionV1{Base: f.Base, BaseChunks: f.BaseChunks, Target: f.Target, TargetChunks: recipeChunksV1(f.TargetRecipe)}
	if err := projection.Validate(); err != nil {
		return err
	}
	if f.Base.ScopeDigest != f.Scope.ExecutionScopeDigest || f.Target.ScopeDigest != f.Scope.ExecutionScopeDigest {
		return NewError(ErrRevisionConflict, "content_delta_scope", "objects do not match execution scope")
	}
	for i := range f.TargetRecipe {
		if err := f.TargetRecipe[i].Validate(); err != nil {
			return err
		}
		if f.TargetRecipe[i].Ordinal != uint32(i) {
			return NewError(ErrInvalidArgument, "target_recipe", "ordinals must be contiguous and zero based")
		}
	}
	recipe, reused, added, removed, sharedBytes, addedBytes, removedBytes, err := deriveContentDeltaV1(f.BaseChunks, recipeChunksV1(f.TargetRecipe))
	if err != nil {
		return err
	}
	if !sameContentDeltaRecipeV1(recipe, f.TargetRecipe) || !sameChunkRefsV1(reused, f.ReusedChunks) || !sameChunkRefsV1(added, f.AddedChunks) || !sameChunkRefsV1(removed, f.RemovedChunks) ||
		sharedBytes != f.SharedBytes || addedBytes != f.AddedBytes || removedBytes != f.RemovedBytes {
		return NewError(ErrRevisionConflict, "content_delta_derivation", "recipe, normalized sets, or byte totals changed")
	}
	expected, err := f.CanonicalDigest()
	if err != nil {
		return err
	}
	if f.Digest == "" || f.Digest != expected {
		return NewError(ErrRevisionConflict, "content_delta_digest", "canonical digest mismatch")
	}
	return nil
}

func (f ContentDeltaFactV1) Ref() ContentDeltaRefV1 {
	return ContentDeltaRefV1(ExactFactRefV2{
		ContractVersion: f.ContractVersion, SchemaRef: f.SchemaRef, Owner: f.Owner,
		TenantID: f.Scope.TenantID, ID: f.DeltaID, Revision: f.Revision,
		Digest: f.Digest, ScopeDigest: f.Scope.ExecutionScopeDigest,
	})
}

func (f ContentDeltaFactV1) Clone() ContentDeltaFactV1 {
	result := f
	result.BaseChunks = append([]ChunkRef{}, f.BaseChunks...)
	result.TargetRecipe = append([]ContentDeltaRecipeEntryV1{}, f.TargetRecipe...)
	result.ReusedChunks = append([]ChunkRef{}, f.ReusedChunks...)
	result.AddedChunks = append([]ChunkRef{}, f.AddedChunks...)
	result.RemovedChunks = append([]ChunkRef{}, f.RemovedChunks...)
	return result
}

type ContentDeltaRefV1 ExactFactRefV2

func (r ContentDeltaRefV1) Validate() error {
	value := ExactFactRefV2(r)
	if err := value.Validate(); err != nil {
		return err
	}
	if value.ContractVersion != ContentDeltaContractV1 || value.SchemaRef != ContentDeltaFactSchemaV1 || value.Revision != 1 {
		return NewError(ErrInvalidArgument, "content_delta_ref", "wrong contract, schema, or revision")
	}
	return validateContentDeltaOwnerV1(value.Owner)
}

func (r ContentDeltaRefV1) Exact() ExactFactRefV2 { return ExactFactRefV2(r) }

func NewContentDeltaFactV1(deltaID, idempotencyKey, requestDigest string, scope Scope, owner OwnerBinding, source ContentDeltaSourceProjectionV1, now time.Time) (ContentDeltaFactV1, error) {
	if err := source.Validate(); err != nil {
		return ContentDeltaFactV1{}, err
	}
	recipe, reused, added, removed, sharedBytes, addedBytes, removedBytes, err := deriveContentDeltaV1(source.BaseChunks, source.TargetChunks)
	if err != nil {
		return ContentDeltaFactV1{}, err
	}
	fact := ContentDeltaFactV1{
		ContractVersion: ContentDeltaContractV1, SchemaRef: ContentDeltaFactSchemaV1,
		DeltaID: deltaID, Revision: 1, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest,
		Scope: scope, Owner: owner, Base: source.Base, Target: source.Target,
		BaseChunks: append([]ChunkRef{}, source.BaseChunks...), TargetRecipe: recipe,
		ReusedChunks: reused, AddedChunks: added, RemovedChunks: removed,
		SharedBytes: sharedBytes, AddedBytes: addedBytes, RemovedBytes: removedBytes,
		CreatedUnixNano: now.UnixNano(),
	}
	digest, err := fact.CanonicalDigest()
	if err != nil {
		return ContentDeltaFactV1{}, err
	}
	fact.Digest = digest
	if err := fact.Validate(); err != nil {
		return ContentDeltaFactV1{}, err
	}
	return fact, nil
}

type contentDeltaChunkIdentityV1 struct {
	SchemaVersion string
	Digest        string
	Length        int64
}

func deriveContentDeltaV1(base, target []ChunkRef) ([]ContentDeltaRecipeEntryV1, []ChunkRef, []ChunkRef, []ChunkRef, int64, int64, int64, error) {
	baseSet := make(map[contentDeltaChunkIdentityV1]ChunkRef, len(base))
	targetSet := make(map[contentDeltaChunkIdentityV1]ChunkRef, len(target))
	for _, chunk := range base {
		if err := chunk.Validate(); err != nil {
			return nil, nil, nil, nil, 0, 0, 0, err
		}
		baseSet[contentDeltaChunkIdentityV1{chunk.SchemaVersion, chunk.Digest, chunk.Length}] = chunk
	}
	recipe := make([]ContentDeltaRecipeEntryV1, len(target))
	var sharedBytes, addedBytes int64
	for i, chunk := range target {
		if err := chunk.Validate(); err != nil {
			return nil, nil, nil, nil, 0, 0, 0, err
		}
		key := contentDeltaChunkIdentityV1{chunk.SchemaVersion, chunk.Digest, chunk.Length}
		targetSet[key] = chunk
		kind := ContentDeltaAdd
		if _, ok := baseSet[key]; ok {
			kind = ContentDeltaReuse
			sharedBytes += chunk.Length
		} else {
			addedBytes += chunk.Length
		}
		recipe[i] = ContentDeltaRecipeEntryV1{Ordinal: uint32(i), Chunk: chunk, Kind: kind}
	}
	reused := make([]ChunkRef, 0)
	added := make([]ChunkRef, 0)
	removed := make([]ChunkRef, 0)
	for key, chunk := range targetSet {
		if _, ok := baseSet[key]; ok {
			reused = append(reused, chunk)
		} else {
			added = append(added, chunk)
		}
	}
	var removedBytes int64
	for key, chunk := range baseSet {
		if _, ok := targetSet[key]; !ok {
			removed = append(removed, chunk)
		}
	}
	for _, chunk := range base {
		key := contentDeltaChunkIdentityV1{chunk.SchemaVersion, chunk.Digest, chunk.Length}
		if _, ok := targetSet[key]; !ok {
			removedBytes += chunk.Length
		}
	}
	sortChunkRefsV1(reused)
	sortChunkRefsV1(added)
	sortChunkRefsV1(removed)
	return recipe, reused, added, removed, sharedBytes, addedBytes, removedBytes, nil
}

func validateContentDeltaChunksV1(chunks []ChunkRef, total int64, schemaVersion, field string) error {
	if len(chunks) == 0 || len(chunks) > MaxReferenceCount {
		return NewError(ErrInvalidArgument, field, "one or more bounded chunks are required")
	}
	var sum int64
	for _, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			return err
		}
		if chunk.SchemaVersion != schemaVersion {
			return NewError(ErrRevisionConflict, field, "chunk schema does not match object schema")
		}
		sum += chunk.Length
	}
	if sum != total {
		return NewError(ErrContentDigestMismatch, field, "chunk lengths do not match object total")
	}
	return nil
}

func recipeChunksV1(recipe []ContentDeltaRecipeEntryV1) []ChunkRef {
	result := make([]ChunkRef, len(recipe))
	for i := range recipe {
		result[i] = recipe[i].Chunk
	}
	return result
}

func sortChunkRefsV1(values []ChunkRef) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].SchemaVersion != values[j].SchemaVersion {
			return values[i].SchemaVersion < values[j].SchemaVersion
		}
		if values[i].Digest != values[j].Digest {
			return values[i].Digest < values[j].Digest
		}
		return values[i].Length < values[j].Length
	})
}

func sameChunkRefsV1(left, right []ChunkRef) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sameContentDeltaRecipeV1(left, right []ContentDeltaRecipeEntryV1) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateContentDeltaOwnerV1(owner OwnerBinding) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	if owner.ComponentID != ContinuityComponentID || owner.Capability != ContentDeltaCapabilityV1 || owner.FactKind != "content_delta_fact_v1" {
		return NewError(ErrInvalidArgument, "owner_binding", "wrong Continuity Content Delta owner")
	}
	return nil
}
