package runtimeadapter

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHumanQuorumPolicyReceiptV5KeepsNominalSourceSeparateFromOwnerProjection(t *testing.T) {
	base := time.Unix(2_300_000_000, 0)
	digest := core.DigestBytes([]byte("human-policy-current-v5"))
	source := contract.HumanQuorumPolicyBindingV2{TenantID: "tenant-human-v5", Ref: "human-policy-current-v5", Revision: 2, Digest: digest, Domain: "review/human", CheckedUnixNano: base.UnixNano(), ExpiresUnixNano: base.Add(10 * time.Minute).UnixNano()}
	projection := runtimeports.HumanQuorumPolicyCurrentProjectionRefV2{ID: source.Ref, Revision: source.Revision, Digest: source.Digest}
	target := contract.HumanTargetExactRefV2{TenantID: source.TenantID, ID: "target-human-v5", Revision: 3, Digest: core.DigestBytes([]byte("target-human-v5"))}
	cutExpires := base.Add(5 * time.Minute).UnixNano()

	receipt, err := SealOwnerCurrentReceiptV5(OwnerCurrentReceiptV5{
		Kind:                        "policy",
		Target:                      target,
		HumanQuorumPolicySource:     &source,
		HumanQuorumPolicyProjection: &projection,
		SourceRef:                   source.Ref,
		SourceRevision:              source.Revision,
		SourceDigest:                source.Digest,
		Projection:                  runtimeports.OperationGovernanceFactRefV3{Ref: projection.ID, Revision: projection.Revision, Digest: projection.Digest, ExpiresUnixNano: cutExpires},
		Current:                     true,
		CheckedUnixNano:             base.UnixNano(),
		SourceExpiresUnixNano:       source.ExpiresUnixNano,
		ExpiresUnixNano:             cutExpires,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PolicyDecisionRef != "" || receipt.PolicyOperationNotRequired || receipt.HumanQuorumPolicySource == nil || receipt.HumanQuorumPolicyProjection == nil {
		t.Fatalf("Human Policy receipt was type-punned as a legacy Policy decision: %+v", receipt)
	}

	source.Ref = "mutated-source"
	projection.ID = "mutated-projection"
	if receipt.HumanQuorumPolicySource.Ref != "human-policy-current-v5" || receipt.HumanQuorumPolicyProjection.ID != "human-policy-current-v5" {
		t.Fatal("sealed Human Policy receipt retained caller-owned pointer aliases")
	}

	drifted := receipt
	drifted.Projection.Ref = "different-owner-projection"
	drifted.ProjectionDigest = ""
	if _, err = SealOwnerCurrentReceiptV5(drifted); err == nil {
		t.Fatal("Human Policy nominal source was accepted with a different Owner projection")
	}
}
