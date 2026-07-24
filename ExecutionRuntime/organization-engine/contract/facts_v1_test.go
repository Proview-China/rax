package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestFactCanonicalAndCurrentV1(t *testing.T) {
	now := time.Unix(1800000000, 0)
	v, err := contract.SealIdentityV1(contract.IdentityFactV1{FactMetaV1: contract.FactMetaV1{TenantID: "tenant-a", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), State: contract.FactActiveV1}, SubjectKind: contract.SubjectHumanV1, SubjectID: "human-a", DisplayHandle: "Human A"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := contract.SealIdentityV1(v)
	if err != nil {
		t.Fatal(err)
	}
	if again.Digest != v.Digest || again.ID != v.ID {
		t.Fatal("canonical seal drifted")
	}
	if err = v.ValidateCurrent(v.ExactRef(), now); err != nil {
		t.Fatal(err)
	}
	if err = v.ValidateCurrent(v.ExactRef(), now.Add(time.Minute)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expiry=%v", err)
	}
}

func TestSourceRejectsNonCanonicalRolesAndDelegationShapeV1(t *testing.T) {
	d := core.DigestBytes([]byte("x"))
	base := contract.ReviewEligibilitySourceV1{TenantID: "t", ReviewerSubjectID: "r", RequiredRoles: []string{"z", "a"}, ScopeDigest: d, ResponsibilitySubjectKind: "k", ResponsibilitySubjectID: "i", ResponsibilitySubjectDigest: d, Production: true}
	if err := base.Validate(); err == nil {
		t.Fatal("unsorted roles accepted")
	}
	base.RequiredRoles = []string{"a"}
	base.RequireDelegation = true
	base.DelegatorSubjectID = "d"
	if err := base.Validate(); err == nil {
		t.Fatal("missing delegated role accepted")
	}
}
