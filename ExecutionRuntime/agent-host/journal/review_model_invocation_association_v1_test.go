package journal

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewModelInvocationAssociationMemoryCorruptionFailsClosedV1(t *testing.T) {
	now := time.Unix(1_900_410_000, 0)
	for _, test := range []struct {
		name    string
		corrupt func(*MemoryReviewModelInvocationAssociationStoreV1, contract.ReviewModelInvocationAssociationFactV1)
	}{
		{name: "current_ref", corrupt: func(store *MemoryReviewModelInvocationAssociationStoreV1, value contract.ReviewModelInvocationAssociationFactV1) {
			ref := store.current[value.ID]
			ref.Digest = core.DigestBytes([]byte("corrupt-current"))
			store.current[value.ID] = ref
		}},
		{name: "history_payload", corrupt: func(store *MemoryReviewModelInvocationAssociationStoreV1, value contract.ReviewModelInvocationAssociationFactV1) {
			store.history[associationHistoryKey(value.RefV1())] = []byte(`{}`)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := associationFactForJournalV1(t, now)
			store := NewMemoryReviewModelInvocationAssociationStoreV1(func() time.Time { return now })
			if _, err := store.CreateReviewModelInvocationAssociationV1(context.Background(), value); err != nil {
				t.Fatal(err)
			}
			store.mu.Lock()
			test.corrupt(store, value)
			store.mu.Unlock()
			if _, err := store.ResolveCurrentReviewModelInvocationAssociationV1(context.Background(), value.Subject); !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("Resolve corrupt state=%v", err)
			}
			if _, err := store.InspectCurrentReviewModelInvocationAssociationV1(context.Background(), value.Subject, value.RefV1()); !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("Inspect corrupt state=%v", err)
			}
		})
	}
}

func associationFactForJournalV1(t *testing.T, now time.Time) contract.ReviewModelInvocationAssociationFactV1 {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	requestDigest := digest("review-command")
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{
		InvocationID: "review-invocation", InvocationDigest: requestDigest, UnifiedRequestDigest: requestDigest,
		RequestToolsDigest: digest("tools"), PreparedPlanDigest: digest("plan"), RouteDigest: digest("route"), ProfileDigest: digest("profile"),
		ActualToolSurfaceDigest: digest("surface"), ActualProviderInjectionDigest: digest("provider"),
		CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability", Revision: 1, Digest: digest("capability")},
		RegistrySnapshotRef:   runtimeports.RegistrySnapshotRefV1{Owner: core.OwnerRef{Domain: "registry", ID: "owner"}, ContractVersion: "1.0.0", ID: "registry", Revision: 1, Digest: digest("registry")},
		CreatedUnixNano:       now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: now.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{
		Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef, RegistrySnapshotRef: prepared.RegistrySnapshotRef,
		ActualToolSurfaceDigest: prepared.ActualToolSurfaceDigest, ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(8 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	strict := true
	call := modelinvoker.RouteCall{RouteID: "openai.direct.payg.responses", Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "review")}, ToolChoice: modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceNone}, Output: modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONSchema, Name: "review", Schema: []byte(`{"type":"object"}`), Strict: &strict}}}
	value, err := contract.SealReviewModelInvocationAssociationV1(contract.ReviewModelInvocationAssociationFactV1{
		Subject:         contract.ReviewModelInvocationAssociationSubjectV1{TenantID: "tenant-a", ReviewAttempt: contract.ReviewAttemptExactCoordinateV1{ID: "attempt-a", Revision: 1, Digest: digest("attempt")}},
		Command:         modelinvoker.GovernedModelInvocationCommandV1{PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), AttemptRequestDigest: requestDigest, DispatchSequence: 1, ProviderAttemptOrdinal: 1, Call: call},
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
