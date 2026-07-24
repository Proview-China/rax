package conformance

import (
	"context"
	"testing"

	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
)

type HumanOrganizationCurrentFixtureV2 struct {
	Reader  reviewport.HumanOrganizationCurrentReaderV2
	Request reviewport.HumanOrganizationCurrentRequestV2
	Close   func()
}

type HumanOrganizationCurrentFactoryV2 func(*testing.T) HumanOrganizationCurrentFixtureV2

// RunHumanOrganizationCurrentV2 is the reusable consumer conformance suite.
// Owner-specific current mutation/revocation tests remain in the Owner suite;
// this verifies Review's exact/deep-clone/bounded-set public behavior.
func RunHumanOrganizationCurrentV2(t *testing.T, factory HumanOrganizationCurrentFactoryV2) {
	t.Helper()
	fixture := factory(t)
	if fixture.Close != nil {
		defer fixture.Close()
	}
	ctx := context.Background()
	first, err := fixture.Reader.InspectHumanOrganizationCurrentV2(ctx, []reviewport.HumanOrganizationCurrentRequestV2{fixture.Request})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].Assignment != fixture.Request.Assignment.ExactRef() || first.Items[0].ReviewerIdentity != fixture.Request.Assignment.ReviewerIdentity {
		t.Fatal("human Organization current cut lost exact Review coordinates")
	}
	first.Items[0].OwnerProjectionRef.Source.RequiredRoles[0] = "alias-mutated"
	second, err := fixture.Reader.InspectHumanOrganizationCurrentV2(ctx, []reviewport.HumanOrganizationCurrentRequestV2{fixture.Request})
	if err != nil {
		t.Fatal(err)
	}
	if second.Items[0].OwnerProjectionRef.Source.RequiredRoles[0] == "alias-mutated" {
		t.Fatal("human Organization current Reader leaked an alias")
	}
	if _, err = fixture.Reader.InspectHumanOrganizationCurrentV2(ctx, []reviewport.HumanOrganizationCurrentRequestV2{fixture.Request, fixture.Request}); err == nil {
		t.Fatal("duplicate Assignment/reviewer set was accepted")
	}
	bad := fixture.Request.Clone()
	bad.ReviewerSubjectID = ""
	if _, err = fixture.Reader.InspectHumanOrganizationCurrentV2(ctx, []reviewport.HumanOrganizationCurrentRequestV2{bad}); err == nil {
		t.Fatal("incomplete reviewer subject was accepted")
	}
}
