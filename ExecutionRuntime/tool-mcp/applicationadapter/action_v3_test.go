package applicationadapter

import (
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestActionCandidateV3ExactModelPayloadAndInputContract(t *testing.T) {
	projection := testkit.ModelProjection(1)
	source, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	input := toolInputContractProjectionFixtureV1(t, testkit.FixedTime)
	candidate := actionCandidateV3FixtureV1(t, projection.Observation.Calls[0].CanonicalArguments, source, input)
	if err = candidate.ValidateAgainstModelProjection(projection); err != nil {
		t.Fatal(err)
	}
	if err = candidate.ValidateAgainstInputContract(input); err != nil {
		t.Fatal(err)
	}

	splice := toolcontract.CloneActionCandidateV3(candidate)
	splice.Payload.Inline = []byte(`{"value":2}`)
	splice.Payload.Length = uint64(len(splice.Payload.Inline))
	splice.Payload.ContentDigest = core.DigestBytes(splice.Payload.Inline)
	splice.Digest = ""
	if _, err = toolcontract.SealActionCandidateV3(splice); err == nil {
		t.Fatal("same Model lineage with another payload was accepted")
	}

	drift := toolcontract.CloneActionCandidateV3(candidate)
	drift.InputContractCurrentRef.Digest = testkit.Digest("input-contract-drift")
	drift.Digest = ""
	if _, err = toolcontract.SealActionCandidateV3(drift); err == nil {
		t.Fatal("Input Contract exact Ref drift was accepted")
	}
}

func TestModelSourceCandidateHistoricalRefV1N1Only(t *testing.T) {
	if _, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(testkit.ModelProjection(2)); err == nil {
		t.Fatal("N>1 Model observation was accepted")
	}
	projection := testkit.ModelProjection(1)
	historical, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	projection.Ref.ObservationDigest = testkit.Digest("observation-drift")
	if err = historical.ValidateAgainstProjection(projection); err == nil {
		t.Fatal("Model Projection exact Ref drift was accepted")
	}
}

func TestActionCandidateV3ConcurrentSealDeterministic(t *testing.T) {
	projection := testkit.ModelProjection(1)
	source, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	input := toolInputContractProjectionFixtureV1(t, testkit.FixedTime)
	base := actionCandidateV3UnsealedFixtureV1(projection.Observation.Calls[0].CanonicalArguments, source, input)
	const workers = 64
	var wg sync.WaitGroup
	refs := make(chan toolcontract.ObjectRef, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidate, err := toolcontract.SealActionCandidateV3(base)
			if err == nil {
				refs <- candidate.ObjectRef()
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(refs)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var winner toolcontract.ObjectRef
	for ref := range refs {
		if winner == (toolcontract.ObjectRef{}) {
			winner = ref
		} else if ref != winner {
			t.Fatalf("canonical CandidateV3 was not deterministic: %#v != %#v", ref, winner)
		}
	}
}

func actionCandidateV3FixtureV1(t *testing.T, arguments []byte, source toolcontract.ModelSourceCandidateHistoricalRefV1, input toolcontract.ToolInputContractCurrentProjectionV1) toolcontract.ActionCandidateV3 {
	t.Helper()
	candidate, err := toolcontract.SealActionCandidateV3(actionCandidateV3UnsealedFixtureV1(arguments, source, input))
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func actionCandidateV3UnsealedFixtureV1(arguments []byte, source toolcontract.ModelSourceCandidateHistoricalRefV1, input toolcontract.ToolInputContractCurrentProjectionV1) toolcontract.ActionCandidateV3 {
	bytes := append([]byte(nil), arguments...)
	payload := runtimeports.OpaquePayloadV2{
		Schema: input.BindingSubject.InputSchema, ContentDigest: core.DigestBytes(bytes), Length: uint64(len(bytes)), Inline: bytes, LimitPolicy: input.BindingSubject.LimitPolicy,
	}
	return toolcontract.ActionCandidateV3{
		TenantID: "tenant-v3", RunID: "run-v3", SessionID: "session-v3", TurnID: "1",
		PendingAction: input.BindingSubject.PendingAction, SourceCandidate: source, Surface: input.BindingSubject.Surface,
		Capability: input.BindingSubject.Capability, Tool: input.BindingSubject.Tool, InputSchema: input.BindingSubject.InputSchema,
		Payload: payload, PayloadRevision: 1, LimitPolicy: input.BindingSubject.LimitPolicy, InputContractCurrentRef: input.Ref,
		SurfaceCurrent: input.SurfaceCurrent.Ref, CapabilityCurrent: input.CapabilityCurrent.Ref, ToolCurrent: input.ToolCurrent.Ref, InputSchemaCurrent: input.InputSchemaCurrent,
		OperationScopeDigest: input.BindingSubject.OperationScopeDigest, EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3,
		ExpectedOwner: input.BindingSubject.ExpectedOwner, ConflictDomain: "tenant/tenant-v3/tool/example", IdempotencyKey: "action-v3",
		CreatedUnixNano: testkit.FixedTime.UnixNano(), RequestedExpiresUnixNano: testkit.FixedTime.Add(10 * time.Second).UnixNano(),
	}
}
