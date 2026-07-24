package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

const Now = int64(1_700_000_000_000_000_000)

func D(value string) contract.Digest { return contract.DigestBytes([]byte(value)) }

func Execution() contract.ExecutionBinding {
	return contract.ExecutionBinding{ScopeDigest: D("scope"), RunID: "run-1", Turn: 1, AuthorityDigest: D("authority")}
}

func Owner() contract.OwnerRef {
	return contract.OwnerRef{ComponentID: "context/source", BindingDigest: D("owner")}
}

func Evidence(id string) contract.EvidenceRef {
	return contract.EvidenceRef{ID: id, Digest: D("evidence:" + id)}
}

func Recipe() contract.ContextRecipe {
	return contract.ContextRecipe{
		ContractVersion: contract.Version,
		ID:              "recipe-1",
		SemanticVersion: "1.0.0",
		Revision:        1,
		Owner:           Owner(),
		Rules: []contract.FragmentRule{
			{Kind: contract.FragmentInstruction, Region: contract.RegionStablePrefix, Required: true, MaxTokens: 100, Degradation: contract.DegradeReject},
			{Kind: contract.FragmentArtifactInline, Region: contract.RegionSemiStable, MaxTokens: 100, Degradation: contract.DegradeExclude},
			{Kind: contract.FragmentConversation, Region: contract.RegionDynamicTail, MaxTokens: 100, Degradation: contract.DegradeExclude},
		},
		Budget:          contract.BudgetPolicy{TotalTokens: 180, StablePrefixMax: 100, SemiStableMax: 100, DynamicTailMax: 100},
		RenderVersion:   "render-v1",
		CreatedUnixNano: Now - int64(time.Hour),
		ExpiresUnixNano: Now + int64(time.Hour),
	}
}

func Candidate(id string, kind contract.FragmentKind, content contract.ContentRef, tokens uint64) contract.ContextCandidate {
	trust := contract.TrustObservation
	if kind == contract.FragmentInstruction {
		trust = contract.TrustAuthoritativeInstruction
	}
	return contract.ContextCandidate{
		ContractVersion: contract.Version,
		ID:              id,
		Revision:        1,
		Kind:            kind,
		Owner:           Owner(),
		Execution:       Execution(),
		SourceRef:       "source:" + id,
		SourceRevision:  1,
		Content:         content,
		Trust:           trust,
		Sensitivity:     contract.SensitivityInternal,
		Mode:            contract.MaterializationInline,
		TokenEstimate:   tokens,
		EstimatorDigest: D("estimator-v1"),
		CacheStability:  50,
		Evidence:        Evidence("evidence-" + id),
		IdempotencyKey:  "idempotency:" + id,
		CreatedUnixNano: Now - int64(time.Minute),
		ExpiresUnixNano: Now + int64(time.Minute),
	}
}
