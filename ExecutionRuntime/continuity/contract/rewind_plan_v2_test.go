package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestRewindPlanV2ExactWorkspaceOnlyValidation(t *testing.T) {
	now := time.Unix(1_752_577_200, 0)
	plan := testkit.RewindPlanV2(now)
	if err := plan.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*contract.RewindPlanFactV2){
		"cross tenant":          func(v *contract.RewindPlanFactV2) { v.KeepChangeSetRefs[0].TenantID = "tenant-2" },
		"wrong workspace owner": func(v *contract.RewindPlanFactV2) { v.PlannedChangeSetRef.Owner.ComponentID = "praxis/tool" },
		"same keep and drop":    func(v *contract.RewindPlanFactV2) { v.DropChangeSetRefs[0] = v.KeepChangeSetRefs[0] },
		"same coordinate changed digest": func(v *contract.RewindPlanFactV2) {
			v.DropChangeSetRefs[0] = v.KeepChangeSetRefs[0]
			v.DropChangeSetRefs[0].Digest = "changed-content-digest"
		},
		"accepted review fact": func(v *contract.RewindPlanFactV2) {
			v.ReviewRequirementRefs[0].Owner.FactKind = "review_authorization_fact_v2"
		},
		"external conflict domain": func(v *contract.RewindPlanFactV2) { v.ConflictDomain = "tenant/tenant-1/external/mail" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := plan.Clone()
			mutate(&changed)
			testkit.RefreshRewindPlanV2(&changed)
			if err := changed.Validate(); !contract.HasCode(err, contract.ErrRewindConflict) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestRewindPlanV2TTLAndStateBoundary(t *testing.T) {
	now := time.Unix(1_752_577_200, 0)
	plan := testkit.RewindPlanV2(now)
	if err := plan.ValidateCurrent(time.Unix(0, plan.ExpiresUnixNano)); !contract.HasCode(err, contract.ErrRewindConflict) {
		t.Fatalf("expiry boundary = %v", err)
	}
	if err := contract.AdvanceRewindPlanStateV2(plan, contract.RewindPlanSubmittedV2, now); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("state skip = %v", err)
	}
	plan.ResidualRefs = []contract.ExactFactRefV2{testkit.ExactRefV2("workspace-unknown-1", "praxis/continuity", "rewind_residual_fact_v1")}
	plan.State = contract.RewindPlanAdmittedV2
	testkit.RefreshRewindPlanV2(&plan)
	if err := plan.Validate(); !contract.HasCode(err, contract.ErrRewindConflict) {
		t.Fatalf("admitted residual = %v", err)
	}
}

func TestRewindPlanV2CanonicalSelectionAndClone(t *testing.T) {
	plan := testkit.RewindPlanV2(time.Unix(1_752_577_200, 0))
	clone := plan.Clone()
	clone.KeepChangeSetRefs[0].ID = "mutated"
	if plan.KeepChangeSetRefs[0].ID == "mutated" {
		t.Fatal("Clone aliases exact refs")
	}
	reordered := plan.Clone()
	reordered.KeepChangeSetRefs = append(reordered.KeepChangeSetRefs, testkit.ExactRefV2("workspace-change-keep-2", "praxis/sandbox", "workspace_change_set_v1"))
	testkit.RefreshRewindPlanV2(&reordered)
	reordered.KeepChangeSetRefs[0], reordered.KeepChangeSetRefs[1] = reordered.KeepChangeSetRefs[1], reordered.KeepChangeSetRefs[0]
	if digest, err := reordered.CanonicalDigest(); err != nil || digest != reordered.Digest {
		t.Fatalf("canonical ordering = (%s,%v)", digest, err)
	}
}
