package contract_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestToolSurfaceManifestCurrentContractV1(t *testing.T) {
	t.Run("C2-005 manifest digest tamper", func(t *testing.T) {
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		request.Manifest.Digest = testkit.Digest("tampered-manifest")
		if err := request.Validate(); err == nil {
			t.Fatal("tampered Manifest digest passed validation")
		}
	})

	t.Run("C2-006 wrong expected injection digest", func(t *testing.T) {
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		request.Manifest.ExpectedInjectionDigest = testkit.Digest("wrong-injection")
		if err := request.Validate(); err == nil {
			t.Fatal("wrong expected injection digest passed validation")
		}
	})

	t.Run("C2-007 entry order model name and effect canonical", func(t *testing.T) {
		manifest := testkit.ToolSurfaceManifestV1(1)
		manifest.Entries[0].Order = 9
		if err := manifest.Validate(); err == nil {
			t.Fatal("non-consecutive entry order was accepted")
		}
		manifest = testkit.ToolSurfaceManifestV1(1)
		duplicate := manifest.Entries[0]
		duplicate.Order = 1
		manifest.Entries = append(manifest.Entries, duplicate)
		manifest.Digest = ""
		if _, err := manifest.ComputeDigest(); err == nil {
			t.Fatal("duplicate ModelName was accepted")
		}
		manifest = testkit.ToolSurfaceManifestV1(1)
		manifest.Entries[0].EffectKinds = append(manifest.Entries[0].EffectKinds, manifest.Entries[0].EffectKinds[0])
		manifest.Digest = ""
		if _, err := manifest.ComputeDigest(); err == nil {
			t.Fatal("duplicate effect kind was accepted")
		}
	})

	t.Run("C2-008 owner revision and expiry duplicates drift", func(t *testing.T) {
		projection := testkit.ToolSurfaceManifestCurrentProjectionV1(1)
		cases := []contract.ToolSurfaceManifestCurrentProjectionV1{projection, projection, projection}
		cases[0].Owner.ID = "other-owner"
		cases[1].Ref.Revision++
		cases[2].ExpiresUnixNano--
		for i, candidate := range cases {
			if err := candidate.Validate(); err == nil {
				t.Fatalf("duplicate field drift case %d passed", i)
			}
		}
	})

	t.Run("C2-009 manifest ref digest is exact", func(t *testing.T) {
		projection := testkit.ToolSurfaceManifestCurrentProjectionV1(1)
		projection.Ref.Digest = testkit.Digest("wrong-ref")
		if err := projection.Validate(); err == nil {
			t.Fatal("wrong Manifest Ref digest passed")
		}
	})

	t.Run("C2-032 nil and empty nested canonical", func(t *testing.T) {
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		request.Manifest.Entries[0].EffectKinds = nil
		request.Manifest.Digest = ""
		if err := request.Validate(); err == nil {
			t.Fatal("nil effect kinds passed")
		}
	})

	t.Run("C2-041 projection expiry equals Manifest expiry", func(t *testing.T) {
		projection := testkit.ToolSurfaceManifestCurrentProjectionV1(1)
		projection.ExpiresUnixNano++
		if err := projection.Validate(); err == nil {
			t.Fatal("projection extended Manifest expiry")
		}
	})

	t.Run("C2-042 Ref revision equals Manifest revision", func(t *testing.T) {
		projection := testkit.ToolSurfaceManifestCurrentProjectionV1(1)
		projection.Ref.Revision = 2
		if err := projection.Validate(); err == nil {
			t.Fatal("projection changed Manifest revision")
		}
	})

	t.Run("C2-043 projection digest tamper is independent", func(t *testing.T) {
		projection := testkit.ToolSurfaceManifestCurrentProjectionV1(1)
		projection.ProjectionDigest = testkit.Digest("wrong-projection")
		if err := projection.Validate(); err == nil {
			t.Fatal("wrong Projection digest passed")
		}
	})

	t.Run("C2-044 manifest and projection digests cannot be interchanged", func(t *testing.T) {
		projection := testkit.ToolSurfaceManifestCurrentProjectionV1(1)
		if projection.Manifest.Digest == projection.ProjectionDigest {
			t.Fatal("fixture failed to separate digest domains")
		}
		projection.ProjectionDigest = projection.Manifest.Digest
		if err := projection.Validate(); err == nil {
			t.Fatal("Manifest digest was accepted as Projection digest")
		}
	})

	t.Run("C2-051 current identity is exact Manifest identity", func(t *testing.T) {
		projection := testkit.ToolSurfaceManifestCurrentProjectionV1(1)
		if projection.Ref.ID != projection.Manifest.ID || projection.Ref.Revision != projection.Manifest.Revision || projection.Ref.Digest != projection.Manifest.Digest {
			t.Fatal("Current Ref introduced a second identity lineage")
		}
		if reflect.DeepEqual(projection.Ref, contract.ToolSurfaceManifestCurrentRefV1{}) {
			t.Fatal("fixture returned an empty current Ref")
		}
		if err := projection.ValidateCurrent(projection.Ref, time.Unix(0, projection.CheckedUnixNano)); err != nil {
			t.Fatal(err)
		}
		if projection.Ref.Digest == core.Digest("") {
			t.Fatal("Manifest digest is empty")
		}
	})
}
