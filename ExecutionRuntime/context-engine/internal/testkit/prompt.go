package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func PromptAssetV1() contract.PromptAssetV1 {
	evidence := []contract.EvidenceRef{Evidence("prompt-example"), Evidence("prompt-instruction"), Evidence("prompt-policy")}
	asset, err := contract.SealPromptAssetV1(contract.PromptAssetV1{
		ID:              "prompt-asset-1",
		SemanticVersion: "1.0.0",
		Revision:        1,
		Owner:           Owner(),
		AuthorityDigest: D("authority"),
		Sensitivity:     contract.SensitivityInternal,
		Fragments: []contract.PromptFragmentSpecV1{
			{ID: "01-instruction", Role: contract.PromptFragmentInstructionV1, Content: PromptContentV1("instruction"), Required: true, TokenEstimate: 20, EstimatorDigest: D("estimator-v1"), CacheStability: 100, Evidence: Evidence("prompt-instruction")},
			{ID: "02-example", Role: contract.PromptFragmentExampleV1, Content: PromptContentV1("example"), TokenEstimate: 10, EstimatorDigest: D("estimator-v1"), CacheStability: 90, Evidence: Evidence("prompt-example")},
			{ID: "03-policy", Role: contract.PromptFragmentPolicyV1, Content: PromptContentV1("policy"), Required: true, TokenEstimate: 15, EstimatorDigest: D("estimator-v1"), CacheStability: 95, Evidence: Evidence("prompt-policy")},
		},
		RenderCompatibility: []contract.FactRef{PromptRenderRefV1("render-a"), PromptRenderRefV1("render-b")},
		Evidence:            evidence,
		CreatedUnixNano:     Now - int64(time.Minute),
		ExpiresUnixNano:     Now + int64(time.Minute),
	})
	if err != nil {
		panic(err)
	}
	return asset
}

func PromptContentV1(id string) contract.ContentRef {
	value := []byte("prompt-content:" + id)
	return contract.ContentRef{Ref: "prompt-content-" + id, Digest: contract.DigestBytes(value), Length: uint64(len(value))}
}

func PromptRenderRefV1(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: D(id)}
}

func PromptAssetRefV1(asset contract.PromptAssetV1) contract.PromptAssetRefV1 {
	ref, err := asset.RefV1()
	if err != nil {
		panic(err)
	}
	return ref
}

func PromptBuildRequestV1(asset contract.PromptAssetV1) contract.BuildPromptCandidatesRequestV1 {
	request, err := contract.SealBuildPromptCandidatesRequestV1(contract.BuildPromptCandidatesRequestV1{
		PromptAssetRef: assetRefV1(asset), Execution: Execution(), RenderCompatibilityRef: asset.RenderCompatibility[0],
		CreatedUnixNano: Now, NotAfterUnixNano: Now + int64(30*time.Second),
	})
	if err != nil {
		panic(err)
	}
	return request
}

func assetRefV1(asset contract.PromptAssetV1) contract.PromptAssetRefV1 {
	return PromptAssetRefV1(asset)
}
