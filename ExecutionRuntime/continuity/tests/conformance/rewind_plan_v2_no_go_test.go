package conformance_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestRewindPlanGovernanceV2HasNoExecutionAuthority(t *testing.T) {
	typeOf := reflect.TypeOf((*ports.RewindPlanGovernancePortV2)(nil)).Elem()
	for i := 0; i < typeOf.NumMethod(); i++ {
		name := strings.ToLower(typeOf.Method(i).Name)
		for _, forbidden := range []string{"execute", "provider", "permit", "fence", "settle", "workspacecommit", "writefile", "rollback"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("NO-GO: Rewind Plan governance exposes %s", typeOf.Method(i).Name)
			}
		}
	}
}

func TestRewindPlanV2CannotEncodeExternalRollbackOrAcceptedReview(t *testing.T) {
	typeOf := reflect.TypeOf(contract.RewindPlanFactV2{})
	for _, forbidden := range []string{"Provider", "Authorization", "Verdict", "Permit", "Fence", "Payload", "Rollback", "Outcome", "Disposition"} {
		if _, ok := typeOf.FieldByName(forbidden); ok {
			t.Fatalf("NO-GO: Rewind Plan copies %s authority or semantics", forbidden)
		}
	}
}
