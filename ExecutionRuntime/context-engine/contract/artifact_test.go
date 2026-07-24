package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestArtifactReadRequiresProvenBase(t *testing.T) {
	range1 := contract.ArtifactRange{Start: 10, End: 20}
	frame := contract.FactRef{ID: "frame-1", Revision: 1, Digest: testkit.D("frame")}
	anchor := contract.ArtifactAnchor{
		ContractVersion: contract.Version, ID: "anchor-1", Revision: 1, ArtifactOwner: testkit.Owner(), ArtifactRef: "artifact-1", ArtifactVersion: "v1", ArtifactDigest: testkit.D("v1"), Range: range1,
		FrameRef: frame, GenerationID: "gen-1", Evidence: testkit.Evidence("anchor-evidence"), CreatedUnixNano: testkit.Now - int64(time.Minute), ExpiresUnixNano: testkit.Now + int64(time.Minute),
	}
	anchorDigest, err := anchor.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	delta := contract.ArtifactDelta{
		ContractVersion: contract.Version, ID: "delta-1", Revision: 1,
		BaseAnchor: contract.FactRef{ID: anchor.ID, Revision: 1, Digest: anchorDigest}, BaseDigest: anchor.ArtifactDigest,
		TargetVersion: "v2", TargetDigest: testkit.D("v2"), Range: range1,
		Delta: contract.ContentRef{Ref: "delta-content", Digest: testkit.D("delta"), Length: 5}, ChainDepth: 1, Evidence: testkit.Evidence("delta-evidence"),
	}
	mode, err := contract.PlanArtifactRead(anchor, "v2", testkit.D("v2"), range1, 2, &delta, testkit.Now)
	if err != nil || mode != contract.ArtifactUseDelta {
		t.Fatalf("valid delta: mode=%q err=%v", mode, err)
	}
	delta.BaseDigest = testkit.D("wrong-base")
	mode, err = contract.PlanArtifactRead(anchor, "v2", testkit.D("v2"), range1, 2, &delta, testkit.Now)
	if err != nil || mode != contract.ArtifactRematerialize {
		t.Fatalf("unproven delta must rematerialize: mode=%q err=%v", mode, err)
	}
}

func TestArtifactDiffAndCompactionAnchorCounterexamples(t *testing.T) {
	range1 := contract.ArtifactRange{Start: 10, End: 20}
	anchor := contract.ArtifactAnchor{
		ContractVersion: contract.Version, ID: "anchor-1", Revision: 1, ArtifactOwner: testkit.Owner(), ArtifactRef: "file-1", ArtifactVersion: "v1", ArtifactDigest: testkit.D("file-v1"), Range: range1,
		FrameRef: contract.FactRef{ID: "frame-1", Revision: 1, Digest: testkit.D("frame")}, GenerationID: "generation-1", Evidence: testkit.Evidence("anchor-evidence"), CreatedUnixNano: testkit.Now - int64(time.Minute), ExpiresUnixNano: testkit.Now + int64(time.Minute),
	}
	anchorDigest, _ := anchor.DigestValue()
	delta := contract.ArtifactDelta{
		ContractVersion: contract.Version, ID: "delta-1", Revision: 1,
		BaseAnchor: contract.FactRef{ID: anchor.ID, Revision: anchor.Revision, Digest: anchorDigest}, BaseDigest: anchor.ArtifactDigest,
		TargetVersion: "v2", TargetDigest: testkit.D("file-v2"), Range: range1,
		Delta: contract.ContentRef{Ref: "delta-content", Digest: testkit.D("delta"), Length: 5}, ChainDepth: 1, Evidence: testkit.Evidence("delta-evidence"),
	}
	generation := contract.ContextGeneration{
		ContractVersion: contract.Version, ID: "generation-2", Revision: 1, Ordinal: 2,
		Parent: &contract.FactRef{ID: "generation-1", Revision: 1, Digest: testkit.D("generation-1")}, RootFrame: contract.FactRef{ID: "frame-2", Revision: 1, Digest: testkit.D("frame-2")},
		RetainedAnchors: []contract.FactRef{{ID: anchor.ID, Revision: anchor.Revision, Digest: anchorDigest}}, CreatedUnixNano: testkit.Now,
	}
	mode, err := contract.PlanArtifactReadAfterCompaction(anchor, generation, "v2", delta.TargetDigest, range1, 2, &delta, testkit.Now)
	if err != nil || mode != contract.ArtifactUseDelta {
		t.Fatalf("retained exact anchor should permit delta: mode=%q err=%v", mode, err)
	}

	tests := []struct {
		name   string
		mutate func(*contract.ArtifactAnchor, *contract.ContextGeneration, *contract.ArtifactDelta, *contract.ArtifactRange, *uint32, *int64)
	}{
		{"anchor_not_retained", func(_ *contract.ArtifactAnchor, g *contract.ContextGeneration, _ *contract.ArtifactDelta, _ *contract.ArtifactRange, _ *uint32, _ *int64) {
			g.RetainedAnchors = nil
		}},
		{"anchor_digest_drift", func(_ *contract.ArtifactAnchor, g *contract.ContextGeneration, _ *contract.ArtifactDelta, _ *contract.ArtifactRange, _ *uint32, _ *int64) {
			g.RetainedAnchors[0].Digest = testkit.D("wrong-anchor")
		}},
		{"delta_anchor_drift", func(_ *contract.ArtifactAnchor, _ *contract.ContextGeneration, d *contract.ArtifactDelta, _ *contract.ArtifactRange, _ *uint32, _ *int64) {
			d.BaseAnchor.Digest = testkit.D("wrong-anchor")
		}},
		{"file_base_digest_drift", func(_ *contract.ArtifactAnchor, _ *contract.ContextGeneration, d *contract.ArtifactDelta, _ *contract.ArtifactRange, _ *uint32, _ *int64) {
			d.BaseDigest = testkit.D("wrong-file-version")
		}},
		{"target_digest_drift", func(_ *contract.ArtifactAnchor, _ *contract.ContextGeneration, d *contract.ArtifactDelta, _ *contract.ArtifactRange, _ *uint32, _ *int64) {
			d.TargetDigest = testkit.D("wrong-target")
		}},
		{"range_drift", func(_ *contract.ArtifactAnchor, _ *contract.ContextGeneration, _ *contract.ArtifactDelta, r *contract.ArtifactRange, _ *uint32, _ *int64) {
			*r = contract.ArtifactRange{Start: 20, End: 30}
		}},
		{"chain_too_deep", func(_ *contract.ArtifactAnchor, _ *contract.ContextGeneration, _ *contract.ArtifactDelta, _ *contract.ArtifactRange, max *uint32, _ *int64) {
			*max = 0 + 1
		}},
		{"anchor_expired", func(a *contract.ArtifactAnchor, _ *contract.ContextGeneration, _ *contract.ArtifactDelta, _ *contract.ArtifactRange, _ *uint32, now *int64) {
			*now = a.ExpiresUnixNano
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, g, d, requested, maxChain, now := anchor, generation, delta, range1, uint32(2), testkit.Now
			g.RetainedAnchors = append([]contract.FactRef(nil), generation.RetainedAnchors...)
			test.mutate(&a, &g, &d, &requested, &maxChain, &now)
			if test.name == "chain_too_deep" {
				d.ChainDepth = 2
				maxChain = 1
			}
			mode, err := contract.PlanArtifactReadAfterCompaction(a, g, "v2", testkit.D("file-v2"), requested, maxChain, &d, now)
			if err != nil || mode != contract.ArtifactRematerialize {
				t.Fatalf("mode=%q err=%v", mode, err)
			}
		})
	}
}
