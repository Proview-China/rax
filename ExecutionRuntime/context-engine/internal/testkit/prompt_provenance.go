package testkit

import "github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"

type PromptProvenanceFixtureV1 struct {
	Provenance contract.PromptUpstreamProvenanceV1
	Artifacts  []contract.PromptUpstreamArtifactBytesV1
	License    []byte
	Generated  []contract.PromptGeneratedContentBytesV1
}

func PromptProvenanceV1() PromptProvenanceFixtureV1 {
	artifact := []byte("official coding agent prompt\nwith {{dynamic_workdir}}\n")
	license := []byte("Apache License 2.0 fixture\n")
	stable := []byte("act as a careful coding agent")
	dynamic := []byte("workdir={{dynamic_workdir}}")
	stableRef := contract.ContentRef{Ref: "prompt-stable-1", Digest: contract.DigestBytes(stable), Length: uint64(len(stable))}
	dynamicRef := contract.ContentRef{Ref: "prompt-dynamic-1", Digest: contract.DigestBytes(dynamic), Length: uint64(len(dynamic))}
	provenance, err := contract.SealPromptUpstreamProvenanceV1(contract.PromptUpstreamProvenanceV1{
		ID:            "prompt-provenance-1",
		Revision:      1,
		SourceClass:   contract.PromptSourceOfficialCodingAgentV1,
		SourceProduct: "official-agent-fixture",
		Artifacts: []contract.PromptUpstreamArtifactV1{{
			ID: "system-prompt", Repository: "https://github.com/example/official-agent", Commit: "0123456789abcdef0123456789abcdef01234567",
			Path: "prompts/system.md", MediaType: "text/markdown", ByteLength: uint64(len(artifact)), ContentDigest: contract.DigestBytes(artifact),
			ExtractedRanges: []contract.PromptUpstreamRangeV1{{Start: 0, End: uint64(len(artifact)), Digest: contract.DigestBytes(artifact)}},
		}},
		License: contract.PromptUpstreamLicenseV1{
			SPDXID: "Apache-2.0", Repository: "https://github.com/example/official-agent", Commit: "0123456789abcdef0123456789abcdef01234567",
			Path: "LICENSE", ByteLength: uint64(len(license)), ContentDigest: contract.DigestBytes(license), ReviewEvidence: []contract.EvidenceRef{Evidence("license-review")},
		},
		TransformChain: []contract.PromptTransformStepV1{{
			ID: "canonical-transform", Revision: 1, Kind: contract.PromptTransformCanonicalizeV1,
			RulesDigest: D("prompt-transform-rules"), ToolDigest: D("prompt-transform-tool"),
		}},
		GeneratedContent: []contract.ContentRef{stableRef, dynamicRef},
		Closure:          contract.PromptClosureManifestV1{Stable: []contract.ContentRef{stableRef}, DynamicTemplate: []contract.ContentRef{dynamicRef}},
		Evidence:         []contract.EvidenceRef{Evidence("official-source"), Evidence("transform-review")},
		CreatedUnixNano:  Now - 100,
		ExpiresUnixNano:  Now + 100,
	})
	if err != nil {
		panic(err)
	}
	return PromptProvenanceFixtureV1{
		Provenance: provenance,
		Artifacts:  []contract.PromptUpstreamArtifactBytesV1{{ArtifactID: "system-prompt", Bytes: artifact}},
		License:    license,
		Generated: []contract.PromptGeneratedContentBytesV1{
			{Ref: stableRef, Bytes: stable},
			{Ref: dynamicRef, Bytes: dynamic},
		},
	}
}

func PromptPresetReferenceProvenanceV1() PromptProvenanceFixtureV1 {
	fixture := PromptProvenanceV1()
	fixture.Provenance.SourceClass = contract.PromptSourceOfficialSDKPresetV1
	fixture.Provenance.SourceProduct = "official-sdk-preset-fixture"
	fixture.Provenance.GeneratedContent = nil
	fixture.Provenance.Closure = contract.PromptClosureManifestV1{}
	fixture.Generated = nil
	sealed, err := contract.SealPromptUpstreamProvenanceV1(fixture.Provenance)
	if err != nil {
		panic(err)
	}
	fixture.Provenance = sealed
	return fixture
}
