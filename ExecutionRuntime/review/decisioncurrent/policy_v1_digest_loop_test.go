package decisioncurrent

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// The legacy ReviewDecisionPolicy V1 subject binds the sealed Target digest,
// while the sealed Target binds the Policy fact digest. Updating either side
// therefore invalidates the other side. Human V5 deliberately uses the
// non-circular HumanQuorumPolicy current subject instead; Auto/Bypass must stay
// fail closed until the Policy Owner publishes an additive V2 applicability
// coordinate.
func TestReviewDecisionPolicyV1ExactTargetBindingHasDigestLoop(t *testing.T) {
	base := time.Unix(2_300_000_000, 0)
	target1 := testkit.Target(base)

	fact1 := runtimeports.ReviewPolicyFactV2{
		Ref:                  "policy-loop-v1",
		Revision:             1,
		SubjectDigest:        target1.Digest,
		Scope:                target1.Scope,
		RunID:                target1.RunID,
		CurrentScope:         target1.CurrentScope,
		RiskClass:            "review/standard",
		ActorAuthorityRef:    target1.ActorAuthority.Ref,
		ReviewerAuthorityRef: "reviewer-authority-loop-v1",
		Active:               true,
		ExpiresUnixNano:      base.Add(10 * time.Minute).UnixNano(),
	}
	var err error
	fact1.Digest, err = fact1.DigestV2()
	if err != nil {
		t.Fatal(err)
	}

	target2 := target1
	target2.Policy = runtimeports.ReviewPolicyBindingRefV2{Ref: fact1.Ref, Revision: fact1.Revision, Digest: fact1.Digest}
	target2.Digest = ""
	target2, err = contract.SealTargetSnapshotV1(target2)
	if err != nil {
		t.Fatal(err)
	}
	if target2.Digest == target1.Digest || fact1.SubjectDigest == target2.Digest {
		t.Fatalf("binding Policy fact into Target unexpectedly preserved the exact Target digest: before=%s after=%s subject=%s", target1.Digest, target2.Digest, fact1.SubjectDigest)
	}

	fact2 := fact1
	fact2.SubjectDigest = target2.Digest
	fact2.Digest = ""
	fact2.Digest, err = fact2.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if fact2.Digest == fact1.Digest {
		t.Fatal("rebinding the legacy Policy fact to the new exact Target did not change its digest")
	}

	target3 := target2
	target3.Policy.Digest = fact2.Digest
	target3.Digest = ""
	target3, err = contract.SealTargetSnapshotV1(target3)
	if err != nil {
		t.Fatal(err)
	}
	if target3.Digest == target2.Digest || fact2.SubjectDigest == target3.Digest {
		t.Fatalf("second legacy Policy/Target reseal unexpectedly closed the digest loop: before=%s after=%s subject=%s", target2.Digest, target3.Digest, fact2.SubjectDigest)
	}
}
