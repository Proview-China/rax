package contract_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestPromptUpstreamProvenanceSealExactClosureV1(t *testing.T) {
	fixture := testkit.PromptProvenanceV1()
	if err := fixture.Provenance.Validate(); err != nil {
		t.Fatal(err)
	}
	ref, err := fixture.Provenance.RefV1()
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID != fixture.Provenance.ID || ref.Revision != fixture.Provenance.Revision || ref.Digest != fixture.Provenance.ProvenanceDigest {
		t.Fatal("provenance exact ref drifted")
	}
	if fixture.Provenance.TransformChain[0].InputDigest != fixture.Provenance.SourceSetDigest || fixture.Provenance.TransformChain[len(fixture.Provenance.TransformChain)-1].OutputDigest != fixture.Provenance.GeneratedSetDigest {
		t.Fatal("transform chain was not sealed to source/generated sets")
	}
	if len(fixture.Provenance.Closure.Stable) != 1 || len(fixture.Provenance.Closure.DynamicTemplate) != 1 || len(fixture.Provenance.Closure.SemiStable) != 0 {
		t.Fatal("closure regions drifted")
	}

	resealed, err := contract.SealPromptUpstreamProvenanceV1(fixture.Provenance)
	if err != nil {
		t.Fatal(err)
	}
	if resealed.ProvenanceDigest != fixture.Provenance.ProvenanceDigest {
		t.Fatal("same provenance did not seal deterministically")
	}
}

func TestPromptUpstreamPresetReferenceAllowsNoGeneratedBodyV1(t *testing.T) {
	fixture := testkit.PromptPresetReferenceProvenanceV1()
	if err := fixture.Provenance.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Provenance.GeneratedContent) != 0 {
		t.Fatal("opaque SDK preset fabricated generated content")
	}
	changed := fixture.Provenance
	changed.SourceClass = contract.PromptSourceOfficialCodingAgentV1
	if _, err := contract.SealPromptUpstreamProvenanceV1(changed); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("coding-agent source without generated content error = %v", err)
	}
}

func TestPromptUpstreamProvenanceFailClosedV1(t *testing.T) {
	base := testkit.PromptProvenanceV1().Provenance
	tests := map[string]func(*contract.PromptUpstreamProvenanceV1){
		"floating_commit": func(v *contract.PromptUpstreamProvenanceV1) { v.Artifacts[0].Commit = "main" },
		"http_repository": func(v *contract.PromptUpstreamProvenanceV1) {
			v.Artifacts[0].Repository = "http://github.com/example/agent"
		},
		"path_escape": func(v *contract.PromptUpstreamProvenanceV1) { v.Artifacts[0].Path = "../prompt.md" },
		"overlap": func(v *contract.PromptUpstreamProvenanceV1) {
			v.Artifacts[0].ExtractedRanges = append(v.Artifacts[0].ExtractedRanges, contract.PromptUpstreamRangeV1{Start: 1, End: v.Artifacts[0].ByteLength, Digest: testkit.D("overlap")})
		},
		"license_evidence_missing": func(v *contract.PromptUpstreamProvenanceV1) { v.License.ReviewEvidence = nil },
		"transform_chain_drift": func(v *contract.PromptUpstreamProvenanceV1) {
			v.TransformChain[0].InputDigest = testkit.D("other-source")
		},
		"closure_duplicate": func(v *contract.PromptUpstreamProvenanceV1) {
			v.Closure.SemiStable = append(v.Closure.SemiStable, v.Closure.Stable[0])
		},
		"same_content_id_changed_digest": func(v *contract.PromptUpstreamProvenanceV1) {
			changed := v.GeneratedContent[0]
			changed.Digest = testkit.D("changed-content")
			v.GeneratedContent = append(v.GeneratedContent, changed)
		},
		"zero_length_content": func(v *contract.PromptUpstreamProvenanceV1) {
			v.GeneratedContent[0].Length = 0
			v.Closure.DynamicTemplate[0].Length = 0
		},
		"unknown_source_class": func(v *contract.PromptUpstreamProvenanceV1) { v.SourceClass = "vendor_claimed" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Artifacts = append([]contract.PromptUpstreamArtifactV1(nil), base.Artifacts...)
			changed.Artifacts[0].ExtractedRanges = append([]contract.PromptUpstreamRangeV1(nil), base.Artifacts[0].ExtractedRanges...)
			changed.License.ReviewEvidence = append([]contract.EvidenceRef(nil), base.License.ReviewEvidence...)
			changed.TransformChain = append([]contract.PromptTransformStepV1(nil), base.TransformChain...)
			changed.GeneratedContent = append([]contract.ContentRef(nil), base.GeneratedContent...)
			changed.Closure.Stable = append([]contract.ContentRef(nil), base.Closure.Stable...)
			changed.Closure.SemiStable = append([]contract.ContentRef(nil), base.Closure.SemiStable...)
			changed.Closure.DynamicTemplate = append([]contract.ContentRef(nil), base.Closure.DynamicTemplate...)
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("invalid provenance was accepted")
			}
		})
	}
}
