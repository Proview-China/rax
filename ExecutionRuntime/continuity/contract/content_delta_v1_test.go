package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestContentDeltaDerivesExactRecipeAndRejectsTamper(t *testing.T) {
	source := contentDeltaSourceV1()
	fact, err := contract.NewContentDeltaFactV1("delta-1", "delta-request-1", "request-digest-1", testkit.Scope(), testkit.ContentDeltaOwnerV1(), source, time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if fact.SharedBytes != 8 || fact.AddedBytes != 4 || fact.RemovedBytes != 4 || len(fact.ReusedChunks) != 2 || len(fact.AddedChunks) != 1 || len(fact.RemovedChunks) != 1 {
		t.Fatalf("derived delta = %#v", fact)
	}
	if fact.TargetRecipe[0].Kind != contract.ContentDeltaReuse || fact.TargetRecipe[2].Kind != contract.ContentDeltaAdd {
		t.Fatalf("recipe = %#v", fact.TargetRecipe)
	}
	tampered := fact.Clone()
	tampered.TargetRecipe[0].Kind = contract.ContentDeltaAdd
	if err := tampered.Validate(); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("caller-forged reuse classification accepted: %v", err)
	}
}

func TestContentDeltaExactChunkIdentityIncludesSchemaAndLength(t *testing.T) {
	source := contentDeltaSourceV1()
	source.Target.SchemaVersion = "content/v2"
	for i := range source.TargetChunks {
		source.TargetChunks[i].SchemaVersion = "content/v2"
	}
	fact, err := contract.NewContentDeltaFactV1("delta-1", "delta-request-1", "request-digest-1", testkit.Scope(), testkit.ContentDeltaOwnerV1(), source, time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if fact.TargetRecipe[0].Kind != contract.ContentDeltaAdd {
		t.Fatal("same digest/length with another schema was reused")
	}
}

func contentDeltaSourceV1() contract.ContentDeltaSourceProjectionV1 {
	chunk := func(schema string, data string) contract.ChunkRef {
		return contract.ChunkRef{SchemaVersion: schema, Digest: contract.DigestBytes([]byte(data)), Length: int64(len(data))}
	}
	baseChunks := []contract.ChunkRef{chunk("content/v1", "AAAA"), chunk("content/v1", "BBBB"), chunk("content/v1", "CCCC")}
	targetChunks := []contract.ChunkRef{chunk("content/v1", "AAAA"), chunk("content/v1", "BBBB"), chunk("content/v1", "DDDD")}
	return contract.ContentDeltaSourceProjectionV1{
		Base:         contract.ContentObjectRefV1{ObjectID: "base-1", SchemaVersion: "content/v1", ManifestDigest: "base-manifest-digest", ContentDigest: contract.DigestBytes([]byte("AAAABBBBCCCC")), TotalLength: 12, Compression: "identity", EncryptionRef: "key-envelope-1", Classification: "sensitive", OwnerID: "continuity", ScopeDigest: testkit.Scope().ExecutionScopeDigest, RetentionPolicyRef: "retention-1"},
		BaseChunks:   baseChunks,
		Target:       contract.ContentObjectRefV1{ObjectID: "target-1", SchemaVersion: "content/v1", ManifestDigest: "target-manifest-digest", ContentDigest: contract.DigestBytes([]byte("AAAABBBBDDDD")), TotalLength: 12, Compression: "identity", EncryptionRef: "key-envelope-1", Classification: "sensitive", OwnerID: "continuity", ScopeDigest: testkit.Scope().ExecutionScopeDigest, RetentionPolicyRef: "retention-1"},
		TargetChunks: targetChunks,
	}
}
