package blackbox_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestToolSurfaceManifestCurrentBlackboxV1(t *testing.T) {
	newRepo := func(t *testing.T) *surface.InMemoryToolSurfaceManifestCurrentRepositoryV1 {
		t.Helper()
		repo, err := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(func() time.Time { return testkit.FixedTime })
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}

	t.Run("C2-022 exact hit returns a fresh clone without writes", func(t *testing.T) {
		repo := newRepo(t)
		winner, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), testkit.ToolSurfaceManifestCurrentRequestV1(1))
		if err != nil {
			t.Fatal(err)
		}
		got, err := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), winner.Ref)
		if err != nil || !reflect.DeepEqual(got, winner) {
			t.Fatalf("exact inspection drifted: %v", err)
		}
		got.Manifest.Entries[0].EffectKinds[0] = "evil/effect"
		again, err := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), winner.Ref)
		if err != nil || again.Manifest.Entries[0].EffectKinds[0] == "evil/effect" {
			t.Fatal("exact inspection returned an internal alias")
		}
	})

	t.Run("C2-023 unknown exact Ref is NotFound", func(t *testing.T) {
		repo := newRepo(t)
		ref := contract.ToolSurfaceManifestCurrentRefV1{ContractVersion: contract.ToolSurfaceManifestCurrentContractVersionV1, ID: "surface_missing", Revision: 1, Digest: testkit.Digest("missing")}
		if _, err := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), ref); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("unknown Ref was not NotFound: %v", err)
		}
	})

	t.Run("C2-049 Plan ToolSurface coordinate maps losslessly", func(t *testing.T) {
		repo := newRepo(t)
		manifest := testkit.ToolSurfaceManifestV1(1)
		planRef := contract.ObjectRef{ID: manifest.ID, Revision: manifest.Revision, Digest: manifest.Digest}
		currentRef := contract.ToolSurfaceManifestCurrentRefV1{ContractVersion: contract.ToolSurfaceManifestCurrentContractVersionV1, ID: planRef.ID, Revision: planRef.Revision, Digest: planRef.Digest}
		request := contract.ToolSurfaceManifestCurrentEnsureRequestV1{ContractVersion: contract.ToolSurfaceManifestCurrentContractVersionV1, Manifest: manifest}
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		got, err := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), currentRef)
		if err != nil || got.Ref.ID != planRef.ID || got.Ref.Revision != planRef.Revision || got.Ref.Digest != planRef.Digest {
			t.Fatalf("Plan coordinate did not resolve exact current: %v", err)
		}
	})
}
