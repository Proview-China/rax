package workspacereadcommand

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	tooladapter "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/owner/surfacebinding"
	ownerrepo "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/owner/workspacereadcommandrepo"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	toolsqlite "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestProducerV1HistoricalAttemptRecoverySkipsFreshReadersAndExpiry(t *testing.T) {
	now := testkit.FixedTime
	fact := testkit.WorkspaceReadExecutionCommandV1(now, "historical")
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	repository := &producerRepositoryFakeV1{historical: fact}
	clock := &producerScriptedClockV1{values: []time.Time{time.Unix(0, fact.NotAfterUnixNano)}}
	producer := &ProducerV1{repository: repository, clock: clock}

	got, err := producer.CreateOrInspectV1(context.Background(), requestFromFactV1(fact))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, fact) {
		t.Fatalf("historical recovery = %+v, want exact fact %+v", got, fact)
	}
	if repository.reverseCalls.Load() != 1 || repository.exactCalls.Load() != 0 ||
		repository.createCalls.Load() != 0 || clock.callCount() != 0 {
		t.Fatalf(
			"historical recovery touched fresh/mutation paths: reverse=%d exact=%d create=%d clock=%d",
			repository.reverseCalls.Load(), repository.exactCalls.Load(),
			repository.createCalls.Load(), clock.callCount(),
		)
	}
}

func TestProducerV1OnlyNotFoundEntersFreshRead(t *testing.T) {
	fact := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "not-found-gate")
	request := requestFromFactV1(fact)
	sentinel := core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "claim probe stopped fresh read")

	for _, test := range []struct {
		name          string
		reverseErr    error
		wantClaimRead int64
		wantReason    core.ReasonCode
	}{
		{
			name:          "non-not-found stops",
			reverseErr:    core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "reverse lookup conflict"),
			wantClaimRead: 0,
			wantReason:    core.ReasonBindingDrift,
		},
		{
			name:          "not-found starts S1",
			reverseErr:    core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "command absent"),
			wantClaimRead: 1,
			wantReason:    core.ReasonComponentMissing,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims := &producerClaimProbeV1{err: sentinel}
			repository := &producerRepositoryFakeV1{reverseErr: test.reverseErr}
			producer := &ProducerV1{repository: repository, claims: claims}
			if _, err := producer.CreateOrInspectV1(context.Background(), request); err == nil ||
				!core.HasReason(err, test.wantReason) {
				t.Fatalf("CreateOrInspect error = %v, want reason %s", err, test.wantReason)
			}
			if claims.calls.Load() != test.wantClaimRead {
				t.Fatalf("claim reads = %d, want %d", claims.calls.Load(), test.wantClaimRead)
			}
			if repository.createCalls.Load() != 0 || repository.exactCalls.Load() != 0 {
				t.Fatalf("failed reverse gate mutated or recovered: create=%d exact=%d",
					repository.createCalls.Load(), repository.exactCalls.Load())
			}
		})
	}
}

func TestProducerV1HistoricalSourceSwapFailsBeforeMutation(t *testing.T) {
	now := testkit.FixedTime
	expected := testkit.WorkspaceReadExecutionCommandV1(now, "source-a")
	spliced := testkit.WorkspaceReadExecutionCommandV1(now, "source-b")
	repository := &producerRepositoryFakeV1{historical: spliced}
	producer := &ProducerV1{repository: repository}

	if _, err := producer.CreateOrInspectV1(context.Background(), requestFromFactV1(expected)); err == nil ||
		!core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("re-sealed SourceCommand swap error = %v, want conflict", err)
	}
	if repository.createCalls.Load() != 0 || repository.exactCalls.Load() != 0 {
		t.Fatalf("source swap crossed mutation boundary: create=%d exact=%d",
			repository.createCalls.Load(), repository.exactCalls.Load())
	}
}

func TestSnapshotV1StableSemanticAllowsCurrentResealButRejectsDrift(t *testing.T) {
	base := producerSemanticSnapshotFixtureV1()

	resigned := mustCloneProducerSnapshotV1(t, base)
	resigned.tool.CheckedUnixNano++
	resigned.tool.ExpiresUnixNano++
	resigned.tool.ProjectionDigest = testkit.Digest("registry-current-resigned")
	resigned.effect.CheckedUnixNano++
	resigned.effect.ExpiresUnixNano++
	resigned.effect.Digest = testkit.Digest("effect-current-resigned")
	resigned.prepared.CheckedUnixNano++
	resigned.prepared.ExpiresUnixNano++
	resigned.prepared.ProjectionDigest = testkit.Digest("prepared-current-resigned")
	if !sameSnapshotV1(base, resigned) {
		t.Fatal("natural current-envelope re-sign changed stable snapshot semantics")
	}

	for _, test := range []struct {
		name   string
		mutate func(*snapshotV1)
	}{
		{
			name: "same Ref registry source body",
			mutate: func(value *snapshotV1) {
				value.tool.Source.State = "deprecated"
			},
		},
		{
			name: "descriptor body",
			mutate: func(value *snapshotV1) {
				value.descriptor.ResultLimitBytes++
			},
		},
		{
			name: "execution state",
			mutate: func(value *snapshotV1) {
				value.state.State = tooladapter.ToolOwnerExecutionInspectOnlyV2
			},
		},
		{
			name: "effect fact revision",
			mutate: func(value *snapshotV1) {
				value.effect.FactRevision++
			},
		},
		{
			name: "effect state",
			mutate: func(value *snapshotV1) {
				value.effect.State = "settled"
			},
		},
		{
			name: "prepared semantic",
			mutate: func(value *snapshotV1) {
				value.prepared.Snapshot.SemanticDigest = testkit.Digest("prepared-semantic-drift")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			drifted := mustCloneProducerSnapshotV1(t, base)
			test.mutate(&drifted)
			if sameSnapshotV1(base, drifted) {
				t.Fatal("stable semantic drift was accepted")
			}
		})
	}
}

func TestBuildFactV1RejectsNonDispatchAndStableSourceDrift(t *testing.T) {
	now := testkit.FixedTime
	fact := testkit.WorkspaceReadExecutionCommandV1(now, "fact-build")
	request := requestFromFactV1(fact)
	snapshot := producerSnapshotFromFactV1(fact)

	rebuilt, err := buildFactV1(request, snapshot, fact.CreatedUnixNano)
	if err != nil {
		t.Fatal(err)
	}
	same, err := toolcontract.SameWorkspaceReadExecutionCommandStableClosureV1(rebuilt, fact)
	if err != nil || !same {
		t.Fatalf("authoritative rebuild differs: same=%t err=%v", same, err)
	}

	nonDispatch := mustCloneProducerSnapshotV1(t, snapshot)
	nonDispatch.effect.State = "settled"
	if _, err = buildFactV1(request, nonDispatch, fact.CreatedUnixNano); err == nil {
		t.Fatal("non-dispatch_intent Effect produced a command fact")
	}

	for _, test := range []struct {
		name   string
		mutate func(*snapshotV1)
	}{
		{
			name: "Source body",
			mutate: func(value *snapshotV1) {
				value.state.Digest = testkit.Digest("state-body-drift")
			},
		},
		{
			name: "Effect fact revision",
			mutate: func(value *snapshotV1) {
				value.effect.FactRevision++
			},
		},
		{
			name: "Prepared semantic digest",
			mutate: func(value *snapshotV1) {
				value.prepared.Snapshot.SemanticDigest = testkit.Digest("semantic-drift")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			drifted := mustCloneProducerSnapshotV1(t, snapshot)
			test.mutate(&drifted)
			if err := validateFactAgainstSnapshotV1(fact, request, drifted, now); err == nil ||
				!core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("stable drift error = %v, want conflict", err)
			}
		})
	}
}

func TestProducerV1OwnerClockAndCurrentBoundary(t *testing.T) {
	now := testkit.FixedTime
	clock := &producerScriptedClockV1{values: []time.Time{
		now, now.Add(time.Nanosecond), now.Add(2 * time.Nanosecond), now.Add(time.Nanosecond),
	}}
	producer := &ProducerV1{clock: clock}
	for index := range 3 {
		got, err := producer.nextNowV1()
		if err != nil {
			t.Fatalf("increasing clock[%d] failed: %v", index, err)
		}
		want := now.Add(time.Duration(index) * time.Nanosecond)
		if !got.Equal(want) {
			t.Fatalf("clock[%d] = %s, want %s", index, got, want)
		}
	}
	if _, err := producer.nextNowV1(); err == nil || !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback error = %v, want clock regression", err)
	}

	fact := testkit.WorkspaceReadExecutionCommandV1(now, "current-boundary")
	checked := time.Unix(0, fact.CreatedUnixNano)
	freshBindingExpiry := checked.Add(8 * time.Second).UnixNano()
	freshInputExpiry := checked.Add(7 * time.Second).UnixNano()
	freshRegistryExpiry := checked.Add(6 * time.Second).UnixNano()
	freshEffectExpiry := checked.Add(5 * time.Second).UnixNano()
	freshPreparedExpiry := checked.Add(4 * time.Second).UnixNano()
	expires := minProducerUnixNanoV1(
		fact.NotAfterUnixNano,
		checked.Add(toolcontract.MaxWorkspaceReadExecutionCommandCurrentTTLV1).UnixNano(),
		freshBindingExpiry,
		freshInputExpiry,
		freshRegistryExpiry,
		freshEffectExpiry,
		freshPreparedExpiry,
	)
	current := toolcontract.WorkspaceReadExecutionCommandCurrentV1{
		ContractVersion:                toolcontract.WorkspaceReadExecutionCommandContractVersionV1,
		Fact:                           fact,
		ToolCurrentProjectionDigest:    testkit.Digest("tool-current"),
		ToolCurrentCheckedUnixNano:     checked.UnixNano(),
		RuntimeEffectCurrentDigest:     testkit.Digest("effect-current"),
		RuntimeEffectCheckedUnixNano:   checked.UnixNano(),
		RuntimePreparedCurrentDigest:   testkit.Digest("prepared-current"),
		RuntimePreparedCheckedUnixNano: checked.UnixNano(),
		CheckedUnixNano:                checked.UnixNano(),
		ExpiresUnixNano:                expires,
	}
	var err error
	current.Digest, err = current.ComputeDigestV1()
	if err != nil {
		t.Fatal(err)
	}
	if err = current.ValidateCurrent(fact.Ref, checked); err != nil {
		t.Fatal(err)
	}
	if current.ExpiresUnixNano != freshPreparedExpiry {
		t.Fatalf("Current expiry = %d, want minimum fresh Prepared expiry %d",
			current.ExpiresUnixNano, freshPreparedExpiry)
	}
	if err = current.ValidateCurrent(fact.Ref, time.Unix(0, current.ExpiresUnixNano)); err == nil ||
		!core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("now==expiry error = %v, want binding expired", err)
	}
	if err = current.ValidateCurrent(fact.Ref, checked.Add(-time.Nanosecond)); err == nil ||
		!core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("current clock rollback error = %v, want clock regression", err)
	}
}

type producerRepositoryFakeV1 struct {
	historical              toolcontract.WorkspaceReadExecutionCommandV1
	reverseErr              error
	exact                   toolcontract.WorkspaceReadExecutionCommandV1
	exactErr                error
	winner                  toolcontract.WorkspaceReadExecutionCommandV1
	createErr               error
	indeterminateAfterStore bool
	reverseCalls            atomic.Int64
	exactCalls              atomic.Int64
	createCalls             atomic.Int64
}

func (r *producerRepositoryFakeV1) CreateWorkspaceReadExecutionCommandOwnedV1(
	_ context.Context,
	capability ownerrepo.WriteCapabilityV1,
	value toolcontract.WorkspaceReadExecutionCommandV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, bool, error) {
	r.createCalls.Add(1)
	if err := capability.Validate(); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
	}
	if r.createErr != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, r.createErr
	}
	if r.winner.Ref.ID == "" {
		r.winner = toolcontract.CloneWorkspaceReadExecutionCommandV1(value)
	}
	r.exact = toolcontract.CloneWorkspaceReadExecutionCommandV1(r.winner)
	if r.indeterminateAfterStore {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false,
			core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost create reply")
	}
	return toolcontract.CloneWorkspaceReadExecutionCommandV1(r.winner), true, nil
}

func (r *producerRepositoryFakeV1) InspectWorkspaceReadExecutionCommandExactV1(
	_ context.Context,
	_ toolcontract.WorkspaceReadExecutionCommandRefV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	r.exactCalls.Add(1)
	if r.exactErr != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, r.exactErr
	}
	return toolcontract.CloneWorkspaceReadExecutionCommandV1(r.exact), nil
}

func (r *producerRepositoryFakeV1) InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(
	_ context.Context,
	_ runtimeports.OperationDispatchAttemptRefV3,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	r.reverseCalls.Add(1)
	if r.reverseErr != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, r.reverseErr
	}
	return toolcontract.CloneWorkspaceReadExecutionCommandV1(r.historical), nil
}

type producerClaimProbeV1 struct {
	tooladapter.ToolOwnerSingleCallClaimStoreV2
	calls atomic.Int64
	err   error
}

func (r *producerClaimProbeV1) InspectToolOwnerSingleCallClaimV2(
	_ context.Context,
	_ applicationcontract.SingleCallToolActionInspectKeyV2,
) (tooladapter.ToolOwnerSingleCallClaimRecordV2, error) {
	r.calls.Add(1)
	return tooladapter.ToolOwnerSingleCallClaimRecordV2{}, r.err
}

type producerScriptedClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	calls  int
}

func (c *producerScriptedClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	index := c.calls
	c.calls++
	if len(c.values) == 0 {
		return time.Time{}
	}
	if index >= len(c.values) {
		index = len(c.values) - 1
	}
	return c.values[index]
}

func (c *producerScriptedClockV1) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func producerSemanticSnapshotFixtureV1() snapshotV1 {
	now := testkit.FixedTime
	tool := testkit.Tool()
	object := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	source, err := toolcontract.SealToolRegistryRecordSourceV1(toolcontract.ToolRegistryRecordSourceV1{
		Kind: "tool", ID: object.ID, ObjectRevision: object.Revision, ObjectDigest: object.Digest,
		State: "active", RegistryRevision: 1, UpdatedUnixNano: now.Add(-time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	current, err := toolcontract.SealToolRegistryObjectCurrentProjectionV1(
		toolcontract.ToolRegistryObjectCurrentProjectionV1{
			Source: source, Object: object, RegistryOwner: tool.Owner,
			CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		panic(err)
	}
	return snapshotV1{
		state:      tooladapter.ToolOwnerSingleCallExecutionStateV2{State: tooladapter.ToolOwnerExecutionStartCommittedV2},
		descriptor: tool,
		tool:       current,
		effect: runtimeports.ControlledOperationEffectCurrentProjectionV2{
			IntentDigest: testkit.Digest("effect-intent"), FactRevision: 1,
			State:           toolcontract.WorkspaceReadExecutionDispatchIntentV1,
			CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(),
			Digest: testkit.Digest("effect-current"),
		},
		prepared: runtimeports.ControlledOperationPreparedCurrentProjectionV2{
			Snapshot: runtimeports.ControlledOperationPreparedSemanticSnapshotV2{
				SemanticDigest: testkit.Digest("prepared-semantic"),
			},
			CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(),
			ProjectionDigest: testkit.Digest("prepared-current"),
		},
	}
}

func producerSnapshotFromFactV1(fact toolcontract.WorkspaceReadExecutionCommandV1) snapshotV1 {
	candidate := toolcontract.ActionCandidateV3{
		ID: fact.Source.Candidate.ID, Revision: fact.Source.Candidate.Revision,
		Digest:                   fact.Source.Candidate.Digest,
		InputContractCurrentRef:  fact.Source.InputContractCurrent,
		Tool:                     fact.Source.Tool,
		ToolCurrent:              fact.Source.ToolCurrent,
		ExpectedOwner:            fact.Source.Owner,
		CreatedUnixNano:          fact.TTL.CandidateCreatedUnixNano,
		RequestedExpiresUnixNano: fact.TTL.CandidateExpiresUnixNano,
	}
	return snapshotV1{
		claim: tooladapter.ToolOwnerSingleCallClaimRecordV2{
			Claim: tooladapter.ToolOwnerSingleCallClaimV2{
				ID: fact.Source.ClaimRef.ID, Revision: fact.Source.ClaimRef.Revision,
				Digest: fact.Source.ClaimRef.Digest, CreatedUnixNano: fact.TTL.ClaimCreatedUnixNano,
			},
			Input: tooladapter.ToolOwnerSingleCallExecutionV2{
				Request: applicationcontract.SingleCallToolActionRequestV2{
					ExpiresUnixNano: fact.TTL.RequestExpiresUnixNano,
				},
			},
		},
		state: tooladapter.ToolOwnerSingleCallExecutionStateV2{
			ID: fact.Source.ExecutionStateRef.ID, Revision: fact.Source.ExecutionStateRef.Revision,
			Digest:               fact.Source.ExecutionStateRef.Digest,
			State:                tooladapter.ToolOwnerExecutionStartCommittedV2,
			ExecutionInputDigest: fact.Source.ExecutionInputDigest,
			ExecutionAttemptID:   fact.Source.ToolExecutionAttemptID,
			UpdatedUnixNano:      fact.TTL.StateUpdatedUnixNano,
			ExpiresUnixNano:      fact.TTL.StateExpiresUnixNano,
		},
		binding: tooladapter.SingleCallToolActionBindingCurrentProjectionV2{
			Ref: fact.Source.BindingCurrent, CandidateRef: fact.Source.Candidate,
			CandidateClosure: tooladapter.SingleCallToolActionCandidateClosureV2{
				Candidate: candidate, ClosureDigest: fact.Source.CandidateClosureDigest,
			},
			CheckedUnixNano: fact.TTL.BindingCheckedUnixNano,
			ExpiresUnixNano: fact.TTL.BindingExpiresUnixNano,
		},
		input: toolcontract.ToolInputContractCurrentProjectionV1{
			Ref: fact.Source.InputContractCurrent, CheckedUnixNano: fact.TTL.InputCheckedUnixNano,
			ExpiresUnixNano: fact.TTL.InputExpiresUnixNano,
		},
		tool: toolcontract.ToolRegistryObjectCurrentProjectionV1{Ref: fact.Source.ToolCurrent},
		effect: runtimeports.ControlledOperationEffectCurrentProjectionV2{
			Intent: runtimeports.OperationEffectIntentV3{
				ExpiresUnixNano: fact.TTL.EffectIntentExpiresUnixNano,
			},
			IntentDigest: fact.RuntimeEffectIntentDigest, FactRevision: fact.RuntimeEffectFactRevision,
			State: fact.RuntimeEffectState,
		},
		prepared: runtimeports.ControlledOperationPreparedCurrentProjectionV2{
			Snapshot: runtimeports.ControlledOperationPreparedSemanticSnapshotV2{
				Prepared: fact.Prepared, Attempt: fact.RuntimeAttempt,
				SemanticDigest: fact.PreparedSemanticDigest,
				PayloadSchema:  fact.PayloadSchema, PayloadDigest: fact.PayloadDigest,
				PayloadRevision: fact.PayloadRevision,
			},
		},
	}
}

func mustCloneProducerSnapshotV1(t *testing.T, value snapshotV1) snapshotV1 {
	t.Helper()
	cloned, err := cloneSnapshotV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return cloned
}

func minProducerUnixNanoV1(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if result == 0 || value < result {
			result = value
		}
	}
	return result
}

type producerBindingFixtureV1 struct {
	now             time.Time
	request         applicationcontract.SingleCallToolActionRequestV2
	binding         tooladapter.SingleCallToolActionBindingCurrentProjectionV2
	descriptor      toolcontract.ToolDescriptor
	registryCurrent toolcontract.ToolRegistryObjectCurrentProjectionV1
	inputCurrent    toolcontract.ToolInputContractCurrentProjectionV1
	operation       runtimeports.OperationSubjectV3
	provider        runtimeports.ProviderBindingRefV2
	bindingReader   *producerBindingReaderV1
	inputReader     *producerInputContractReaderV1
	registryReader  *producerRegistryReaderV1
}

func newProducerBindingFixtureV1(t *testing.T) producerBindingFixtureV1 {
	t.Helper()
	now := testkit.FixedTime
	clock := testkit.NewManualClock(now)
	modelProjection := producerModelProjectionV1(t)
	descriptor := producerWorkspaceReadDescriptorV1(t)

	surfaceRepository, err := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	surfaceRequest := producerSurfaceCurrentRequestV1(t, descriptor)
	surfaceCurrent, err := surfaceRepository.EnsureExactToolSurfaceManifestCurrentV1(
		context.Background(), surfaceRequest,
	)
	if err != nil {
		t.Fatal(err)
	}
	preparedInvocation := testkit.PreparedSurfaceInvocationFactV1()
	preparedInvocation.ID, preparedInvocation.Digest = "", ""
	preparedInvocation.InvocationID = modelProjection.Ref.InvocationID
	preparedInvocation.InvocationDigest = modelProjection.Ref.InvocationDigest
	preparedInvocation.UnifiedRequestDigest = modelProjection.Ref.InvocationDigest
	preparedInvocation.ActualToolSurfaceDigest = surfaceCurrent.Manifest.ExpectedInjectionDigest
	preparedInvocation, err = modelinvoker.SealPreparedModelInvocationFactV1(preparedInvocation)
	if err != nil {
		t.Fatal(err)
	}
	preparedInvocationCurrent := testkit.PreparedSurfaceInvocationCurrentV1(preparedInvocation)
	assembly := testkit.ModelPreDispatchAssemblyCurrentV1(surfaceCurrent, preparedInvocation)
	surfaceBindingRepository, err := surfacebinding.NewInMemoryRepositoryV1(testkit.Owner(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = surfaceBindingRepository.EnsureToolSurfaceInvocationBindingV1(
		context.Background(),
		toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1{
			Invocation: toolcontract.ToolSurfaceInvocationCoordinateV1{
				InvocationID: modelProjection.Ref.InvocationID, InvocationDigest: modelProjection.Ref.InvocationDigest,
			},
			PreparedFactRef: preparedInvocation.Ref(), PreparedHistoricalFact: preparedInvocation,
			PreparedCurrentRef: preparedInvocationCurrent.Ref(), PreparedCurrent: preparedInvocationCurrent,
			SurfaceCurrent: surfaceCurrent, AssemblyCurrentRef: assembly.Ref,
			AssemblyRegistrySnapshot: preparedInvocation.RegistrySnapshotRef, AssemblyCurrent: assembly,
			RequestedNotAfterUnixNano: now.Add(10 * time.Minute).UnixNano(),
		},
	); err != nil {
		t.Fatal(err)
	}

	registryReader, capability, registryDescriptor := producerRegistryFixtureV1(t, clock, descriptor)
	provider := runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-set-workspace-read-v1", BindingSetRevision: 1,
		ComponentID: testkit.SettlementOwner().ComponentID, ManifestDigest: testkit.SettlementOwner().ManifestDigest,
		ArtifactDigest: registryDescriptor.ArtifactDigest,
		Capability:     runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3),
	}
	route := testkit.ControlledProviderRouteV2(now, provider)
	generation, association := producerGenerationAssociationV1(t, now, route)
	providerCurrent, err := runtimeports.SealProviderBindingCurrentProjectionV2(
		runtimeports.ProviderBindingCurrentProjectionV2{
			ContractVersion: runtimeports.ProviderBindingCurrentnessContractVersionV2,
			Ref:             provider, State: runtimeports.ProviderBindingCurrentActiveV2,
			BindingSetDigest: route.BindingSetDigest, BindingSetSemanticDigest: route.BindingSetSemanticDigest,
			BindingID: "provider-binding-workspace-read-v1", BindingRevision: 1,
			GrantDigest:     testkit.Digest("provider-grant-workspace-read-v1"),
			IssuedUnixNano:  now.Add(-time.Second).UnixNano(),
			ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	applicationRequest, applicationInput := producerApplicationRequestV2(
		t, now, modelProjection, provider, association.RefV1(), route.Ref,
	)
	inputResolver, err := tooladapter.NewToolInputContractCurrentResolverV1(
		surfaceRepository, registryReader,
		tooladapter.NewInMemoryToolInputContractLeaseStoreV1(), clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := tooladapter.NewCandidateBuilderV3(registryReader, clock)
	if err != nil {
		t.Fatal(err)
	}
	bindingReader, err := tooladapter.NewBindingCurrentReaderV2(
		&producerApplicationInputReaderV1{projection: applicationInput},
		&producerModelReaderV1{projection: modelProjection},
		surfaceBindingRepository, surfaceRepository, registryReader, inputResolver, candidate,
		&producerAssociationReaderV1{fact: association},
		&producerGenerationReaderV1{projection: generation},
		&producerRouteReaderV1{projection: route},
		&producerProviderReaderV1{projection: providerCurrent},
		tooladapter.NewInMemorySingleCallToolActionBindingLeaseStoreV2(), clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	resolve := tooladapter.SingleCallToolActionBindingResolveRequestV2{
		ApplicationRequest: applicationRequest, SourceSubject: applicationRequest.Action.PendingSubject,
		RequestedExpiresUnixNano: now.Add(4 * time.Second).UnixNano(),
	}
	binding, err := bindingReader.ResolveSingleCallToolActionBindingCurrentV2(context.Background(), resolve)
	if err != nil {
		t.Fatal(err)
	}
	_, registryCurrent, err := registryReader.InspectExactToolDescriptorCurrentV1(
		context.Background(), binding.CandidateClosure.Candidate.Tool,
		binding.CandidateClosure.Candidate.ToolCurrent,
	)
	if err != nil {
		t.Fatal(err)
	}
	staticBinding := &producerBindingReaderV1{value: binding}
	staticInput := &producerInputContractReaderV1{value: binding.CandidateClosure.InputContract}
	resigningRegistry := &producerRegistryReaderV1{
		descriptor: registryDescriptor, value: registryCurrent,
		nextChecked: now.Add(10 * time.Nanosecond), step: time.Nanosecond,
		expires: now.Add(3 * time.Second),
	}
	_ = capability
	return producerBindingFixtureV1{
		now: now, request: applicationRequest, binding: binding, descriptor: registryDescriptor,
		registryCurrent: registryCurrent, inputCurrent: binding.CandidateClosure.InputContract,
		operation: applicationRequest.Action.PendingSubject.Binding.OwnerInputs.ModelTurnOperation,
		provider:  provider, bindingReader: staticBinding, inputReader: staticInput,
		registryReader: resigningRegistry,
	}
}

func producerWorkspaceReadDescriptorV1(t *testing.T) toolcontract.ToolDescriptor {
	t.Helper()
	descriptor := testkit.Tool()
	descriptor.ID = "tool/workspace-read-local"
	descriptor.ConflictDomain = "workspace/read"
	descriptor.Digest = ""
	sealed, err := toolcontract.SealTool(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func producerSurfaceCurrentRequestV1(
	t *testing.T,
	descriptor toolcontract.ToolDescriptor,
) toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1 {
	t.Helper()
	manifest := testkit.ToolSurfaceManifestV1(1)
	manifest.Entries[0].Tool = toolcontract.ObjectRef{
		ID: string(descriptor.ID), Revision: descriptor.Revision, Digest: descriptor.Digest,
	}
	manifest.Entries[0].ModelName = toolcontract.CoreToolWorkspaceReadV1
	manifest.Entries[0].InputSchema = descriptor.InputSchema
	manifest.Digest = ""
	var err error
	manifest, err = toolcontract.SealSurface(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1,
		Manifest:        manifest,
	}
}

func producerRegistryFixtureV1(
	t *testing.T,
	clock *testkit.ManualClock,
	descriptor toolcontract.ToolDescriptor,
) (*tooladapter.RegistryObjectCurrentReaderV1, toolcontract.CapabilityDescriptor, toolcontract.ToolDescriptor) {
	t.Helper()
	registryStore := registry.New()
	capability := testkit.Capability()
	capabilityRecord, err := registryStore.SubmitCapability(capability, testkit.FixedTime.Add(-3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	capabilityRecord, err = registryStore.Transition(
		"capability", string(capability.ID), capabilityRecord.RegistryRevision,
		registry.StateAdmitted, testkit.FixedTime.Add(-2*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registryStore.Transition(
		"capability", string(capability.ID), capabilityRecord.RegistryRevision,
		registry.StateActive, testkit.FixedTime.Add(-time.Second),
	); err != nil {
		t.Fatal(err)
	}
	toolRecord, err := registryStore.SubmitTool(descriptor, testkit.FixedTime.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err = registryStore.Transition(
		"tool", string(descriptor.ID), toolRecord.RegistryRevision,
		registry.StateAdmitted, testkit.FixedTime.Add(-time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registryStore.Transition(
		"tool", string(descriptor.ID), toolRecord.RegistryRevision,
		registry.StateActive, testkit.FixedTime,
	); err != nil {
		t.Fatal(err)
	}
	reader, err := tooladapter.NewRegistryObjectCurrentReaderV1(registryStore, clock)
	if err != nil {
		t.Fatal(err)
	}
	return reader, capability, descriptor
}

func producerModelProjectionV1(t *testing.T) modelinvoker.ToolCallCandidateObservationProjectionV1 {
	t.Helper()
	call := modelinvoker.FunctionCall{
		ID: "workspace-read-call-v1", Name: toolcontract.CoreToolWorkspaceReadV1,
		Arguments: json.RawMessage(`{"path":"README.md"}`),
	}
	response := modelinvoker.Response{
		ID: "workspace-read-response-v1", Status: modelinvoker.ResponseStatusCompleted,
		StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{
			Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call,
		}},
	}
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(
		testkit.Digest("workspace-read-model-invocation-v1"), response,
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := modelinvoker.NewToolCallCandidateObservationProjectionV1(
		"workspace-read-invocation-v1", 1, response.ID, observation,
	)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func producerGenerationAssociationV1(
	t *testing.T,
	now time.Time,
	route runtimeports.ControlledOperationProviderRouteCurrentProjectionV2,
) (runtimeports.GenerationCurrentProjectionV1, runtimeports.GenerationBindingAssociationFactV1) {
	t.Helper()
	component := runtimeports.GenerationComponentManifestRefV1{
		ComponentID:    route.ProviderBinding.ComponentID,
		ManifestDigest: route.ProviderBinding.ManifestDigest,
		ArtifactDigest: route.ProviderBinding.ArtifactDigest,
	}
	extension := runtimeports.GenerationGovernanceExtensionRefV1{
		Kind: "praxis.tool/test-extension", Contract: testkit.Schema("generation-extension"),
		Digest: testkit.Digest("generation-extension"),
	}
	generation, err := runtimeports.SealGenerationCurrentProjectionV1(
		runtimeports.GenerationCurrentProjectionV1{
			Generation: route.Generation, ComponentManifests: []runtimeports.GenerationComponentManifestRefV1{component},
			Extension: extension, State: runtimeports.GenerationCurrentSealedV1, Current: true,
			Watermark: 1, ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := runtimeports.SealGenerationBindingSetCurrentProjectionV1(
		runtimeports.GenerationBindingSetCurrentProjectionV1{
			BindingSetID: route.BindingSetID, BindingSetRevision: route.BindingSetRevision,
			BindingSetDigest: route.BindingSetDigest, BindingSetSemanticDigest: route.BindingSetSemanticDigest,
			PlanDigest:                 testkit.Digest("binding-plan-workspace-read-v1"),
			GovernanceDigest:           testkit.Digest("binding-governance-workspace-read-v1"),
			ComponentManifestSetDigest: runtimeports.GenerationComponentManifestSetDigestV1(generation.ComponentManifests),
			CurrentnessDigest:          route.BindingSetCurrentnessDigest,
			IssuedUnixNano:             now.Add(-time.Second).UnixNano(),
			ExpiresUnixNano:            now.Add(5 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	activationOperation := testkit.BoundaryFixture(now).Operation
	activationOperation.Kind = runtimeports.OperationScopeActivationV3
	activationOperation.RunID = ""
	activationOperation.ActivationAttemptID = "activation-attempt-workspace-read-v1"
	activationOperation.CurrentProjectionRef = "activation-current-workspace-read-v1"
	activationOperation.CurrentProjectionDigest = testkit.Digest("activation-current-workspace-read-v1")
	activationDigest, err := activationOperation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	activation, err := runtimeports.SealGenerationActivationCurrentProjectionV1(
		runtimeports.GenerationActivationCurrentProjectionV1{
			Operation: activationOperation, OperationDigest: activationDigest, Active: true,
			Watermark: 1, CurrentnessDigest: testkit.Digest("activation-currentness-workspace-read-v1"),
			ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := runtimeports.SealGenerationBindingAssociationCandidateV1(
		runtimeports.GenerationBindingAssociationCandidateV1{
			AssociationID: "generation-association-workspace-read-v1",
			Generation:    generation, Binding: binding, Activation: activation,
			RequestedExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := runtimeports.SealGenerationBindingAssociationFactV1(
		runtimeports.GenerationBindingAssociationFactV1{
			ID: candidate.AssociationID, Revision: 1,
			State:     runtimeports.GenerationBindingAssociationActiveV1,
			Candidate: candidate, CandidateDigest: candidate.Digest,
			CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.UnixNano(),
			ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return generation, fact
}

func producerApplicationRequestV2(
	t *testing.T,
	now time.Time,
	model modelinvoker.ToolCallCandidateObservationProjectionV1,
	provider runtimeports.ProviderBindingRefV2,
	association runtimeports.GenerationBindingAssociationRefV1,
	route runtimeports.ControlledOperationProviderRouteCurrentRefV2,
) (applicationcontract.SingleCallToolActionRequestV2, applicationcontract.SingleCallToolActionInputCurrentProjectionV2) {
	t.Helper()
	digest := testkit.Digest
	operation := testkit.BoundaryFixture(now).Operation
	arguments := append([]byte(nil), model.Observation.Calls[0].CanonicalArguments...)
	argumentsDigest := core.DigestBytes(arguments)
	schema := testkit.Schema("input")
	domainSchema := testkit.Schema("model-domain-result")
	domainDigest := digest("model-domain-result-workspace-read-v1")
	pending := applicationcontract.SingleCallPendingActionCoordinateV1{
		ActionRef: "pending-action-workspace-read-v1", RequestDigest: digest("pending-request-workspace-read-v1"),
		Capability:    runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3),
		PayloadSchema: schema, PayloadDigest: argumentsDigest,
		SourceCandidateID: "source-candidate-workspace-read-v1", SourceCandidateRevision: 1,
		SourceCandidateDigest: digest("source-candidate-workspace-read-v1"), ProjectionDigest: model.Ref.Digest,
	}
	identity, err := applicationcontract.SealSingleCallModelPendingActionIdentityCoordinateV2(
		applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{
			IdentityContractVersion: applicationcontract.SingleCallIdentityContractVersionV1,
			IdentityID:              "identity-workspace-read-v1", IdentityRevision: 1,
			IdentityDigest:    digest("identity-owner-workspace-read-v1"),
			CreatedUnixNano:   now.Add(-time.Second).UnixNano(),
			ModelProjectionID: model.Ref.ID, ModelProjectionRevision: model.Ref.Revision,
			ModelProjectionDigest: model.Ref.Digest, ModelInvocationID: model.Ref.InvocationID,
			ModelInvocationDigest: model.Ref.InvocationDigest, ModelObservationDigest: model.Ref.ObservationDigest,
			ModelSourceResponseID: model.Ref.Source.ResponseID, ModelSourceSequence: model.Ref.Source.SourceSequence,
			SourceKeyDigest:            digest("source-key-workspace-read-v1"),
			SourceExecutionScopeDigest: operation.ExecutionScopeDigest,
			SourceRunID:                string(operation.RunID), SourceSessionID: "session-workspace-read-v1", SourceTurn: 1,
			CallOrdinalEncodingVersion: applicationcontract.SingleCallCallOrdinalEncodingVersionV1,
			CallOrdinalPresent:         true, CallOrdinalValue: 0,
			SettlementOwner: provider, CallID: model.Observation.Calls[0].CallID,
			CallName: model.Observation.Calls[0].Name, CanonicalArgumentsDigest: argumentsDigest,
			PendingActionRef: pending.ActionRef, PendingActionRequestDigest: pending.RequestDigest,
			PayloadSchema: schema, PayloadContentDigest: argumentsDigest, Capability: pending.Capability,
			SourceCandidateID: pending.SourceCandidateID, SourceCandidateRevision: pending.SourceCandidateRevision,
			SourceCandidateDigest: pending.SourceCandidateDigest, DomainResultDigest: domainDigest,
			NotAfterUnixNano: now.Add(8 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	identityRef := applicationcontract.SingleCallModelPendingActionIdentityRefCoordinateV2{
		ID: identity.IdentityID, Revision: identity.IdentityRevision, Digest: identity.IdentityDigest,
		ModelProjectionID: identity.ModelProjectionID, ModelProjectionRevision: identity.ModelProjectionRevision,
		ModelProjectionDigest: identity.ModelProjectionDigest, PendingActionRef: identity.PendingActionRef,
		PendingActionRequestDigest: identity.PendingActionRequestDigest,
		DomainResultDigest:         identity.DomainResultDigest, SourceKeyDigest: identity.SourceKeyDigest,
	}
	domainFact := applicationcontract.SingleCallSettledTurnDomainResultFactRefCoordinateV2{
		FactID: "domain-fact-workspace-read-v1", Revision: 1,
		FactDigest: digest("domain-fact-workspace-read-v1"), SourceKeyDigest: identity.SourceKeyDigest,
		Schema: domainSchema, ContentDigest: domainDigest, IdentityRef: identityRef,
	}
	owner := runtimeports.EffectOwnerRefV2{
		Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID,
		ManifestDigest: provider.ManifestDigest,
	}
	settlement := runtimeports.OperationSettlementRefV3{
		ID: "model-settlement-workspace-read-v1", Revision: 1,
		Digest:      digest("model-settlement-workspace-read-v1"),
		Attempt:     testkit.BoundaryFixture(now).Attempt,
		Disposition: runtimeports.OperationSettlementAppliedV3, Owner: owner,
		Evidence: []runtimeports.EvidenceRecordRefV2{{
			LedgerScopeDigest: digest("ledger-workspace-read-v1"), Sequence: 1,
			RecordDigest: digest("record-workspace-read-v1"),
		}},
		DomainResultSchema: &domainSchema, DomainResultDigest: domainDigest,
	}
	base, err := applicationcontract.SealSingleCallHarnessBaseBindingCoordinateV2(
		applicationcontract.SingleCallHarnessBaseBindingCoordinateV2{
			PendingAction: pending, IdentityRef: identityRef, DomainResultFact: domainFact,
			ModelTurnSettlement: settlement,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ownerInputs, err := applicationcontract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(
		applicationcontract.SingleCallHarnessOwnerCurrentInputsCoordinateV2{
			HarnessContractVersion: applicationcontract.SingleCallHarnessOwnerCurrentInputsVersionV1,
			ModelTurnOperation:     operation, GenerationBindingAssociation: association,
			RouteCurrent: route, RouteMatrix: runtimeports.OperationScopeEvidenceActionMatrixV3(),
			ContextApplicability: runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{
				Kind: runtimeports.OperationScopeEvidenceContextParentKindV3,
				ID:   "context-workspace-read-v1", Revision: 1, Digest: digest("context-workspace-read-v1"),
			},
			HarnessDigest: digest("harness-workspace-read-v1"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := applicationcontract.SealSingleCallHarnessApplicationBindingCoordinateV2(
		applicationcontract.SingleCallHarnessApplicationBindingCoordinateV2{
			BindingVersion: applicationcontract.SingleCallHarnessBindingContractVersionV2,
			Base:           base, OwnerInputs: ownerInputs,
			HarnessBindingDigest: digest("binding-owner-workspace-read-v1"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	run, err := applicationcontract.SealSingleCallRunSubjectCoordinateV2(
		applicationcontract.SingleCallRunSubjectCoordinateV2{
			ExecutionScope: operation.ExecutionScope, RunID: operation.RunID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := applicationcontract.SealSingleCallPendingActionSubjectCoordinateV2(
		applicationcontract.SingleCallPendingActionSubjectCoordinateV2{
			Run: run, SessionID: "session-workspace-read-v1", SessionRevision: 1,
			SessionDigest: digest("session-workspace-read-v1"), Turn: 1,
			PendingActionRef: pending.ActionRef, PendingActionDigest: pending.RequestDigest,
			Binding: binding, Identity: identity,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	action, err := applicationcontract.SealSingleCallActionCoordinateV2(
		applicationcontract.SingleCallActionCoordinateV2{
			ExecutionScope: operation.ExecutionScope, PendingSubject: subject,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	authority := runtimeports.AuthorityBindingRefV2{
		Ref: "authority-workspace-read-v1", Revision: 1,
		Digest: digest("authority-workspace-read-v1"), Epoch: operation.ExecutionScope.AuthorityEpoch,
	}
	request, err := applicationcontract.SealSingleCallToolActionRequestV2(
		applicationcontract.SingleCallToolActionRequestV2{
			Action: action, Authority: authority,
			CreatedUnixNano: now.Add(-time.Second).UnixNano(),
			ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := applicationcontract.SealSingleCallModelToolCallProjectionProofV2(
		applicationcontract.SingleCallModelToolCallProjectionProofV2{
			ProjectionContractVersion: applicationcontract.SingleCallModelProjectionContractVersionV1,
			ProjectionID:              model.Ref.ID, ProjectionRevision: model.Ref.Revision,
			ProjectionDigest: model.Ref.Digest, InvocationID: model.Ref.InvocationID,
			InvocationDigest: model.Ref.InvocationDigest, ObservationDigest: model.Ref.ObservationDigest,
			SourceResponseID: model.Ref.Source.ResponseID, SourceSequence: model.Ref.Source.SourceSequence,
			CallID: identity.CallID, CallName: identity.CallName, CanonicalArguments: arguments,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	identityCurrentRequest, err := applicationcontract.SealSingleCallModelPendingActionIdentityCurrentRequestV2(
		applicationcontract.SingleCallModelPendingActionIdentityCurrentRequestV2{
			Run: run, SessionID: subject.SessionID, Turn: subject.Turn,
			IdentityRef: identityRef, DomainResultFact: domainFact,
			RequestedNotAfterUnixNano: request.ExpiresUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	identityCurrent, err := applicationcontract.SealSingleCallModelPendingActionIdentityCurrentV2(
		applicationcontract.SingleCallModelPendingActionIdentityCurrentV2{
			IdentityRef: identityRef, DomainResultFact: domainFact, Identity: identity,
			Projection: proof, CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(),
			ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
		identityCurrentRequest, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	harnessCurrent, err := applicationcontract.SealSingleCallHarnessOwnerCurrentProofV3(
		applicationcontract.SingleCallHarnessOwnerCurrentProofV3{
			Subject: subject, Binding: binding,
			HarnessCurrentContractVersion: applicationcontract.SingleCallHarnessCurrentContractVersionV3,
			HarnessCurrentDigest:          digest("harness-current-workspace-read-v1"),
			IdentityCurrent:               identityCurrent, CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(),
			ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	authorityCurrent, err := applicationcontract.SealSingleCallAuthorityCurrentProofV2(
		applicationcontract.SingleCallAuthorityCurrentProofV2{
			Ref: authority, ExecutionScopeDigest: action.ExecutionScopeDigest,
			ActionCoordinateDigest: action.Digest, FactRevision: 1,
			FactDigest:      digest("authority-fact-workspace-read-v1"),
			CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(),
			ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
		request, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	input, err := applicationcontract.SealSingleCallToolActionInputCurrentProjectionV2(
		applicationcontract.SingleCallToolActionInputCurrentProjectionV2{
			HarnessCurrent: harnessCurrent, AuthorityCurrent: authorityCurrent,
			CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(),
			ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
		},
		request, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return request, input
}

type producerApplicationInputReaderV1 struct {
	projection applicationcontract.SingleCallToolActionInputCurrentProjectionV2
}

func (r *producerApplicationInputReaderV1) InspectSingleCallToolActionInputCurrentV2(
	context.Context,
	applicationcontract.SingleCallToolActionRequestV2,
) (applicationcontract.SingleCallToolActionInputCurrentProjectionV2, error) {
	return applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(r.projection), nil
}

type producerModelReaderV1 struct {
	projection modelinvoker.ToolCallCandidateObservationProjectionV1
}

func (r *producerModelReaderV1) InspectExactProjectionV1(
	_ context.Context,
	exact modelinvoker.ToolCallCandidateObservationRefV1,
) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	if r.projection.Ref != exact {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{},
			core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model exact Ref drifted")
	}
	return r.projection.Clone(), nil
}

type producerAssociationReaderV1 struct {
	fact runtimeports.GenerationBindingAssociationFactV1
}

func (r *producerAssociationReaderV1) InspectCurrentGenerationBindingAssociationV1(
	context.Context,
	string,
) (runtimeports.GenerationBindingAssociationFactV1, error) {
	return r.fact, nil
}

type producerGenerationReaderV1 struct {
	projection runtimeports.GenerationCurrentProjectionV1
}

func (r *producerGenerationReaderV1) InspectGenerationCurrentV1(
	context.Context,
	runtimeports.GenerationArtifactRefV1,
) (runtimeports.GenerationCurrentProjectionV1, error) {
	return r.projection, nil
}

type producerRouteReaderV1 struct {
	projection runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
}

func (r *producerRouteReaderV1) InspectCurrentControlledOperationProviderRouteV2(
	context.Context,
	runtimeports.ControlledOperationProviderRouteCurrentRefV2,
	runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3,
) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	return r.projection, nil
}

type producerProviderReaderV1 struct {
	projection runtimeports.ProviderBindingCurrentProjectionV2
}

func (r *producerProviderReaderV1) InspectProviderBindingCurrentV2(
	context.Context,
	runtimeports.ProviderBindingRefV2,
) (runtimeports.ProviderBindingCurrentProjectionV2, error) {
	return r.projection, nil
}

type producerBindingReaderV1 struct {
	tooladapter.SingleCallToolActionBindingCurrentReaderV2
	value  tooladapter.SingleCallToolActionBindingCurrentProjectionV2
	calls  atomic.Int64
	mutate func(int64, *tooladapter.SingleCallToolActionBindingCurrentProjectionV2)
}

func (r *producerBindingReaderV1) InspectExactSingleCallToolActionBindingCurrentV2(
	_ context.Context,
	request tooladapter.SingleCallToolActionBindingInspectExactRequestV2,
) (tooladapter.SingleCallToolActionBindingCurrentProjectionV2, error) {
	call := r.calls.Add(1)
	value := tooladapter.CloneSingleCallToolActionBindingCurrentProjectionV2(r.value)
	if request.Expected != value.Ref {
		return tooladapter.SingleCallToolActionBindingCurrentProjectionV2{},
			core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding exact Ref drifted")
	}
	if r.mutate != nil {
		r.mutate(call, &value)
	}
	return value, nil
}

type producerInputContractReaderV1 struct {
	toolcontract.ToolInputContractCurrentReaderV1
	value toolcontract.ToolInputContractCurrentProjectionV1
	calls atomic.Int64
}

func (r *producerInputContractReaderV1) InspectExactToolInputContractCurrentV1(
	_ context.Context,
	request toolcontract.ToolInputContractInspectExactRequestV1,
) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	r.calls.Add(1)
	value := toolcontract.CloneToolInputContractCurrentProjectionV1(r.value)
	if request.Expected != value.Ref {
		return toolcontract.ToolInputContractCurrentProjectionV1{},
			core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Input Contract exact Ref drifted")
	}
	return value, nil
}

type producerRegistryReaderV1 struct {
	toolcontract.ToolRegistryObjectCurrentReaderV1
	descriptor  toolcontract.ToolDescriptor
	value       toolcontract.ToolRegistryObjectCurrentProjectionV1
	nextChecked time.Time
	step        time.Duration
	expires     time.Time
	calls       atomic.Int64
	mutate      func(int64, *toolcontract.ToolDescriptor, *toolcontract.ToolRegistryObjectCurrentProjectionV1)
}

func (r *producerRegistryReaderV1) InspectExactToolDescriptorCurrentV1(
	_ context.Context,
	object toolcontract.ObjectRef,
	expected toolcontract.ToolRegistryObjectCurrentRefV1,
) (toolcontract.ToolDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	call := r.calls.Add(1)
	descriptor := r.descriptor
	current := r.value
	if object != current.Object || expected != current.Ref {
		return toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{},
			core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Descriptor exact Ref drifted")
	}
	checked := r.nextChecked.Add(time.Duration(call-1) * r.step)
	current.CheckedUnixNano = checked.UnixNano()
	current.ExpiresUnixNano = r.expires.UnixNano()
	current.ProjectionDigest = ""
	var err error
	current, err = toolcontract.SealToolRegistryObjectCurrentProjectionV1(current)
	if err != nil {
		return toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	if r.mutate != nil {
		r.mutate(call, &descriptor, &current)
	}
	return descriptor, current, nil
}

func TestProducerV1AuthoritativeCreateCurrentAndLostReplyE2EV1(t *testing.T) {
	t.Run("create and current use complete authoritative S1 S2", func(t *testing.T) {
		fixture := newProducerE2EFixtureV1(t)

		fact, err := fixture.producer.CreateOrInspectV1(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		if err = fact.Validate(); err != nil {
			t.Fatal(err)
		}
		if fixture.repository.createCalls.Load() != 1 {
			t.Fatalf("repository creates = %d, want 1", fixture.repository.createCalls.Load())
		}
		if fixture.registry.calls.Load() != 2 || fixture.effects.calls.Load() != 2 ||
			fixture.prepared.calls.Load() != 2 {
			t.Fatalf(
				"create S1/S2 reads registry=%d effect=%d prepared=%d, want 2 each",
				fixture.registry.calls.Load(), fixture.effects.calls.Load(), fixture.prepared.calls.Load(),
			)
		}

		current, err := fixture.producer.InspectWorkspaceReadExecutionCommandCurrentV1(
			context.Background(), fact.Ref,
		)
		if err != nil {
			t.Fatal(err)
		}
		checked := time.Unix(0, current.CheckedUnixNano)
		if err = current.ValidateCurrent(fact.Ref, checked); err != nil {
			t.Fatal(err)
		}
		if current.ExpiresUnixNano != fixture.prepared.expires.UnixNano() {
			t.Fatalf(
				"Current expiry = %d, want minimum Prepared current expiry %d",
				current.ExpiresUnixNano, fixture.prepared.expires.UnixNano(),
			)
		}
		if fixture.registry.calls.Load() != 4 || fixture.effects.calls.Load() != 4 ||
			fixture.prepared.calls.Load() != 4 {
			t.Fatalf(
				"create+current S1/S2 reads registry=%d effect=%d prepared=%d, want 4 each",
				fixture.registry.calls.Load(), fixture.effects.calls.Load(), fixture.prepared.calls.Load(),
			)
		}
	})

	t.Run("commit lost reply recovers creator exact fact", func(t *testing.T) {
		fixture := newProducerE2EFixtureV1(t)
		fixture.repository.indeterminateAfterStore = true

		fact, err := fixture.producer.CreateOrInspectV1(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		if err = fact.Validate(); err != nil {
			t.Fatal(err)
		}
		if fixture.repository.createCalls.Load() != 1 || fixture.repository.exactCalls.Load() != 1 {
			t.Fatalf(
				"lost-reply recovery create=%d exact=%d, want 1/1",
				fixture.repository.createCalls.Load(), fixture.repository.exactCalls.Load(),
			)
		}
	})

	t.Run("historical reverse wins after sources expire", func(t *testing.T) {
		fixture := newProducerE2EFixtureV1(t)
		fact, err := fixture.producer.CreateOrInspectV1(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		before := []int64{
			fixture.claims.inspectCalls.Load(),
			fixture.states.inspectCalls.Load(),
			fixture.binding.calls.Load(),
			fixture.input.calls.Load(),
			fixture.registry.calls.Load(),
			fixture.effects.calls.Load(),
			fixture.prepared.calls.Load(),
			fixture.repository.createCalls.Load(),
			fixture.repository.exactCalls.Load(),
			fixture.repository.reverseCalls.Load(),
		}
		clockCalls := fixture.clock.callCount()

		recovered, err := fixture.producer.CreateOrInspectV1(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(recovered, fact) {
			t.Fatal("historical reverse did not recover the exact SQLite fact")
		}
		after := []int64{
			fixture.claims.inspectCalls.Load(),
			fixture.states.inspectCalls.Load(),
			fixture.binding.calls.Load(),
			fixture.input.calls.Load(),
			fixture.registry.calls.Load(),
			fixture.effects.calls.Load(),
			fixture.prepared.calls.Load(),
			fixture.repository.createCalls.Load(),
			fixture.repository.exactCalls.Load(),
			fixture.repository.reverseCalls.Load(),
		}
		for index := range before[:9] {
			if after[index] != before[index] {
				t.Fatalf("historical reverse changed non-reverse counter[%d] from %d to %d", index, before[index], after[index])
			}
		}
		if after[9] != before[9]+1 || fixture.clock.callCount() != clockCalls {
			t.Fatalf(
				"historical reverse delta=%d clock delta=%d, want 1/0",
				after[9]-before[9], fixture.clock.callCount()-clockCalls,
			)
		}
	})
}

func TestProducerV1AuthoritativeDescriptorAndEffectGatesE2EV1(t *testing.T) {
	t.Run("valid but wrong descriptor is rejected before write", func(t *testing.T) {
		fixture := newProducerE2EFixtureV1(t)
		fixture.registry.mutate = func(
			call int64,
			descriptor *toolcontract.ToolDescriptor,
			_ *toolcontract.ToolRegistryObjectCurrentProjectionV1,
		) {
			if call != 2 {
				return
			}
			descriptor.ID = "tool/workspace-read-spliced"
			descriptor.Digest = ""
			sealed, err := toolcontract.SealTool(*descriptor)
			if err != nil {
				t.Fatal(err)
			}
			*descriptor = sealed
		}

		if _, err := fixture.producer.CreateOrInspectV1(
			context.Background(), fixture.request,
		); err == nil || !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("wrong descriptor error = %v, want Conflict", err)
		}
		if fixture.repository.createCalls.Load() != 0 {
			t.Fatalf("wrong descriptor crossed write boundary: %d", fixture.repository.createCalls.Load())
		}
	})

	t.Run("non dispatch intent effect is rejected before write", func(t *testing.T) {
		fixture := newProducerE2EFixtureV1(t)
		fixture.effects.state = "settled"

		if _, err := fixture.producer.CreateOrInspectV1(
			context.Background(), fixture.request,
		); err == nil || !core.HasReason(err, core.ReasonEffectStateConflict) {
			t.Fatalf("non-dispatch Effect error = %v, want EffectStateConflict", err)
		}
		if fixture.repository.createCalls.Load() != 0 {
			t.Fatalf("non-dispatch Effect crossed write boundary: %d", fixture.repository.createCalls.Load())
		}
	})

	t.Run("effect stable semantics drift between S1 and S2", func(t *testing.T) {
		fixture := newProducerE2EFixtureV1(t)
		fixture.effects.mutate = func(
			call int64,
			value *runtimeports.ControlledOperationEffectCurrentProjectionV2,
		) {
			if call == 2 {
				value.FactRevision++
			}
		}

		if _, err := fixture.producer.CreateOrInspectV1(
			context.Background(), fixture.request,
		); err == nil || !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("Effect S1/S2 drift error = %v, want Conflict", err)
		}
		if fixture.repository.createCalls.Load() != 0 {
			t.Fatalf("Effect drift crossed write boundary: %d", fixture.repository.createCalls.Load())
		}
	})

	t.Run("prepared stable semantics drift between S1 and S2", func(t *testing.T) {
		fixture := newProducerE2EFixtureV1(t)
		fixture.prepared.mutate = func(
			call int64,
			value *runtimeports.ControlledOperationPreparedCurrentProjectionV2,
		) {
			if call != 2 {
				return
			}
			value.Snapshot.PersistedEnforcement.ReceiptDigest = testkit.Digest("prepared-receipt-drift")
			value.Snapshot.SemanticDigest = ""
			sealed, err := runtimeports.SealControlledOperationPreparedSemanticSnapshotV2(value.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			value.Snapshot = sealed
		}

		if _, err := fixture.producer.CreateOrInspectV1(
			context.Background(), fixture.request,
		); err == nil || !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("Prepared S1/S2 drift error = %v, want Conflict", err)
		}
		if fixture.repository.createCalls.Load() != 0 {
			t.Fatalf("Prepared drift crossed write boundary: %d", fixture.repository.createCalls.Load())
		}
	})

	t.Run("requested TTL crosses before commit", func(t *testing.T) {
		fixture := newProducerE2EFixtureV1(t)
		fixture.request.RequestedNotAfter = fixture.now.Add(2500 * time.Microsecond).UnixNano()

		if _, err := fixture.producer.CreateOrInspectV1(
			context.Background(), fixture.request,
		); err == nil || !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("Prepared TTL crossing error = %v, want BindingExpired", err)
		}
		if fixture.repository.createCalls.Load() != 0 {
			t.Fatalf("Prepared TTL crossing wrote %d commands", fixture.repository.createCalls.Load())
		}
	})
}

func TestProducerV1SQLiteConcurrentNaturalCurrentResealConvergesV1(t *testing.T) {
	fixture := newProducerE2EFixtureV1(t)
	const contenders = 64
	fixture.repository.reverseBarrier = newProducerReverseBarrierV1(contenders)
	results := make(chan toolcontract.WorkspaceReadExecutionCommandV1, contenders)
	errs := make(chan error, contenders)
	var wait sync.WaitGroup
	for index := range contenders {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			offset := time.Duration(index*10) * time.Microsecond
			producerClock := &producerScriptedClockV1{values: []time.Time{
				fixture.now.Add(offset + time.Microsecond),
				fixture.now.Add(offset + 2*time.Microsecond),
				fixture.now.Add(offset + 3*time.Microsecond),
			}}
			producer, err := NewProducerV1(
				fixture.producer.claims,
				fixture.producer.states,
				fixture.producer.bindings,
				fixture.producer.inputContracts,
				fixture.producer.registry,
				fixture.producer.effects,
				fixture.producer.prepared,
				fixture.repository,
				producerClock,
			)
			if err != nil {
				errs <- err
				return
			}
			value, err := producer.CreateOrInspectV1(context.Background(), fixture.request)
			if err != nil {
				errs <- err
				return
			}
			results <- value
		}(index)
	}
	wait.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var winner toolcontract.WorkspaceReadExecutionCommandV1
	count := 0
	for value := range results {
		if count == 0 {
			winner = value
		} else if !reflect.DeepEqual(value, winner) {
			t.Fatal("concurrent Producers did not converge on one exact SQLite winner")
		}
		count++
	}
	if count != contenders {
		t.Fatalf("Producer results = %d, want %d", count, contenders)
	}
	if fixture.repository.createCalls.Load() != contenders {
		t.Fatalf(
			"concurrent Producers reached SQLite create %d times, want %d independently-created candidates",
			fixture.repository.createCalls.Load(), contenders,
		)
	}
}

type producerE2EFixtureV1 struct {
	now        time.Time
	producer   *ProducerV1
	request    CreateRequestV1
	repository *producerCountingRepositoryV1
	claims     *producerCountingClaimStoreV1
	states     *producerCountingStateStoreV1
	binding    *producerBindingReaderV1
	input      *producerInputContractReaderV1
	registry   *producerRegistryReaderV1
	effects    *producerEffectReaderV1
	prepared   *producerPreparedReaderV1
	clock      *producerScriptedClockV1
}

func newProducerE2EFixtureV1(t *testing.T) producerE2EFixtureV1 {
	t.Helper()
	binding := newProducerBindingFixtureV1(t)
	now := binding.now
	candidate := binding.binding.CandidateClosure.Candidate
	execution := tooladapter.ToolOwnerSingleCallExecutionV2{
		Request: binding.request,
		Binding: binding.binding,
	}
	if err := execution.Validate(); err != nil {
		t.Fatal(err)
	}
	claimID, err := toolcontract.StableID(
		"tool-owner-single-call-claim-v2",
		execution.Request.ID,
		string(execution.Request.Digest),
		string(execution.Request.Action.ExecutionScopeDigest),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim := tooladapter.ToolOwnerSingleCallClaimV2{
		ContractVersion:      "praxis.tool-mcp.single-call-owner-claim/v2",
		ID:                   claimID,
		Revision:             1,
		RequestID:            execution.Request.ID,
		RequestDigest:        execution.Request.Digest,
		ActionDigest:         execution.Request.Action.Digest,
		ExecutionScopeDigest: execution.Request.Action.ExecutionScopeDigest,
		BindingRef:           execution.Binding.Ref,
		CreatedUnixNano:      execution.Request.CreatedUnixNano,
	}
	claim.Digest, err = claim.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	record := tooladapter.ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: execution}
	if err = record.Validate(); err != nil {
		t.Fatal(err)
	}
	claimStore := tooladapter.NewInMemoryToolOwnerSingleCallClaimStoreV2()
	if _, created, createErr := claimStore.CreateToolOwnerSingleCallClaimV2(
		context.Background(), record,
	); createErr != nil || !created {
		t.Fatalf("claim create created=%t err=%v", created, createErr)
	}
	state, err := tooladapter.NewToolOwnerSingleCallExecutionStartV2(
		record, now.UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	stateStore := tooladapter.NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	if _, created, createErr := stateStore.CreateExecutionStartV2(
		context.Background(), state,
	); createErr != nil || !created {
		t.Fatalf("state create created=%t err=%v", created, createErr)
	}
	claims := &producerCountingClaimStoreV1{ToolOwnerSingleCallClaimStoreV2: claimStore}
	states := &producerCountingStateStoreV1{ToolOwnerSingleCallExecutionStateStoreV2: stateStore}

	operationDigest, err := binding.operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	provider := binding.provider
	if candidate.ExpectedOwner.ComponentID != provider.ComponentID ||
		candidate.ExpectedOwner.ManifestDigest != provider.ManifestDigest {
		t.Fatalf(
			"Candidate owner %v does not match Provider %v",
			candidate.ExpectedOwner, provider,
		)
	}
	owners := []runtimeports.EffectOwnerRefV2{
		{
			Role: runtimeports.OwnerCleanup, ComponentID: provider.ComponentID,
			ManifestDigest: provider.ManifestDigest,
		},
		{
			Role: runtimeports.OwnerEffect, ComponentID: provider.ComponentID,
			ManifestDigest: provider.ManifestDigest,
		},
		{
			Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID,
			ManifestDigest: provider.ManifestDigest,
		},
	}
	intent := runtimeports.OperationEffectIntentV3{
		ContractVersion:   runtimeports.OperationEffectContractVersionV3,
		ID:                "workspace-read-effect-v1",
		Revision:          1,
		Operation:         binding.operation,
		Kind:              candidate.EffectKind,
		RiskClass:         "praxis.tool/controlled",
		ActionScopeDigest: candidate.OperationScopeDigest,
		Payload:           candidate.Payload,
		PayloadRevision:   candidate.PayloadRevision,
		Target:            candidate.ID,
		ConflictDomain: runtimeports.ConflictDomainBindingV2{
			Domain:      "workspace/read",
			ScopeClass:  runtimeports.EffectStableScopeTenantV2,
			ScopeDigest: runtimeports.StableTenantScopeDigestV2(binding.operation.ExecutionScope.Identity.TenantID),
		},
		Owners:   owners,
		Provider: provider,
		Authority: runtimeports.AuthorityBindingRefV2{
			Ref: binding.request.Authority.Ref, Revision: binding.request.Authority.Revision,
			Digest: binding.request.Authority.Digest, Epoch: binding.request.Authority.Epoch,
		},
		Review: runtimeports.OperationReviewBindingRefV3{
			CaseRef: "workspace-read-review-v1", CandidateDigest: candidate.Digest,
			CandidateRevision: candidate.Revision, PolicyDigest: testkit.Digest("workspace-read-review-policy-v1"),
		},
		Budget: runtimeports.OperationBudgetBindingRefV3{
			Ref: "workspace-read-budget-v1", Revision: 1,
			Digest: testkit.Digest("workspace-read-budget-v1"), PolicyDigest: testkit.Digest("workspace-read-budget-policy-v1"),
			SubjectDigest: operationDigest,
		},
		Policy: runtimeports.OperationPolicyBindingRefV3{
			Ref: "workspace-read-policy-v1", Revision: 1,
			Digest: testkit.Digest("workspace-read-policy-v1"), SubjectDigest: operationDigest,
		},
		Idempotency: runtimeports.IdempotencyBindingV2{
			Key: candidate.IdempotencyKey, ScopeClass: runtimeports.EffectStableScopeTenantV2,
			ScopeDigest: runtimeports.StableTenantScopeDigestV2(binding.operation.ExecutionScope.Identity.TenantID),
			Class:       core.IdempotencyQueryable,
		},
		CredentialLeases: []runtimeports.CredentialLeaseRefV2{},
		ExpiresUnixNano:  now.Add(4 * time.Second).UnixNano(),
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{
		ID: "workspace-read-delegation-v1", Revision: 2,
		Digest: testkit.Digest("workspace-read-delegation-v1"),
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: intent.ID, IntentRevision: intent.Revision,
		IntentDigest: intentDigest, PermitID: "workspace-read-permit-v1", PermitRevision: 1,
		PermitDigest: testkit.Digest("workspace-read-permit-v1"), AttemptID: "workspace-read-runtime-attempt-v1",
		Delegation: &delegation,
	}
	prepared := testkit.PreparedAttemptFor(
		now,
		testkit.BoundaryFixtureV1{Operation: binding.operation, Attempt: attempt},
		provider,
		candidate.InputSchema,
		candidate.Payload.ContentDigest,
		candidate.PayloadRevision,
	)
	persisted := runtimeports.PersistedOperationEnforcementRefV3{
		PermitID: attempt.PermitID, PermitRevision: attempt.PermitRevision,
		PermitDigest: attempt.PermitDigest, AttemptID: attempt.AttemptID,
		OperationDigest: attempt.OperationDigest, Provider: provider,
		ReceiptDigest: testkit.Digest("workspace-read-persisted-enforcement-v1"), RecordedRevision: 1,
	}
	semantics, err := runtimeports.SealControlledOperationPreparedSemanticSnapshotV2(
		runtimeports.ControlledOperationPreparedSemanticSnapshotV2{
			Prepared: prepared, Delegation: delegation, PersistedEnforcement: persisted,
			OperationDigest: attempt.OperationDigest, EffectID: attempt.EffectID,
			IntentRevision: attempt.IntentRevision, IntentDigest: attempt.IntentDigest,
			Attempt: attempt, ProviderBinding: provider, PayloadSchema: candidate.InputSchema,
			PayloadDigest: candidate.Payload.ContentDigest, PayloadRevision: candidate.PayloadRevision,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	effectCurrent, err := runtimeports.SealControlledOperationEffectCurrentProjectionV2(
		runtimeports.ControlledOperationEffectCurrentProjectionV2{
			Intent: intent, IntentDigest: intentDigest, FactRevision: 1,
			State:           toolcontract.WorkspaceReadExecutionDispatchIntentV1,
			CheckedUnixNano: now.Add(20 * time.Nanosecond).UnixNano(),
			ExpiresUnixNano: now.Add(2500 * time.Millisecond).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	preparedCurrent, err := runtimeports.SealControlledOperationPreparedCurrentProjectionV2(
		runtimeports.ControlledOperationPreparedCurrentProjectionV2{
			Snapshot: semantics, CheckedUnixNano: now.Add(20 * time.Nanosecond).UnixNano(),
			ExpiresUnixNano: now.Add(2 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	store, err := toolsqlite.OpenWorkspaceReadExecutionCommandStoreV1(
		context.Background(),
		toolsqlite.ConfigV1{
			Path:  filepath.Join(t.TempDir(), "producer-workspace-read-command.db"),
			Clock: func() time.Time { return now.Add(10 * time.Millisecond) },
			Owner: testkit.Owner(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repository := &producerCountingRepositoryV1{inner: store}
	effects := &producerEffectReaderV1{
		value: effectCurrent, nextChecked: now.Add(20 * time.Nanosecond),
		step: time.Nanosecond, expires: now.Add(2500 * time.Millisecond),
	}
	preparedReader := &producerPreparedReaderV1{
		value: preparedCurrent, nextChecked: now.Add(20 * time.Nanosecond),
		step: time.Nanosecond, expires: now.Add(2 * time.Second),
	}
	clock := &producerScriptedClockV1{values: []time.Time{
		now.Add(time.Millisecond), now.Add(2 * time.Millisecond), now.Add(3 * time.Millisecond),
		now.Add(4 * time.Millisecond), now.Add(5 * time.Millisecond), now.Add(6 * time.Millisecond),
		now.Add(7 * time.Millisecond), now.Add(8 * time.Millisecond),
	}}
	producer, err := NewProducerV1(
		claims, states, binding.bindingReader, binding.inputReader, binding.registryReader,
		effects, preparedReader, repository, clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(binding.request)
	if err != nil {
		t.Fatal(err)
	}
	request := CreateRequestV1{
		RequestKey: key, Binding: binding.binding.Ref, Action: candidate.ObjectRef(),
		Operation: binding.operation, Prepared: prepared, RuntimeAttempt: attempt,
		RequestedNotAfter: now.Add(3500 * time.Millisecond).UnixNano(),
	}
	if err = request.validate(); err != nil {
		t.Fatal(err)
	}
	return producerE2EFixtureV1{
		now: now, producer: producer, request: request, repository: repository,
		claims: claims, states: states, binding: binding.bindingReader, input: binding.inputReader,
		registry: binding.registryReader, effects: effects, prepared: preparedReader, clock: clock,
	}
}

type producerCountingClaimStoreV1 struct {
	tooladapter.ToolOwnerSingleCallClaimStoreV2
	inspectCalls atomic.Int64
}

func (r *producerCountingClaimStoreV1) InspectToolOwnerSingleCallClaimV2(
	ctx context.Context,
	key applicationcontract.SingleCallToolActionInspectKeyV2,
) (tooladapter.ToolOwnerSingleCallClaimRecordV2, error) {
	r.inspectCalls.Add(1)
	return r.ToolOwnerSingleCallClaimStoreV2.InspectToolOwnerSingleCallClaimV2(ctx, key)
}

type producerCountingStateStoreV1 struct {
	tooladapter.ToolOwnerSingleCallExecutionStateStoreV2
	inspectCalls atomic.Int64
}

func (r *producerCountingStateStoreV1) InspectExecutionStateV2(
	ctx context.Context,
	key applicationcontract.SingleCallToolActionInspectKeyV2,
) (tooladapter.ToolOwnerSingleCallExecutionStateV2, error) {
	r.inspectCalls.Add(1)
	return r.ToolOwnerSingleCallExecutionStateStoreV2.InspectExecutionStateV2(ctx, key)
}

type producerCountingRepositoryV1 struct {
	inner                   ownerrepo.RepositoryV1
	indeterminateAfterStore bool
	reverseBarrier          *producerReverseBarrierV1
	reverseCalls            atomic.Int64
	exactCalls              atomic.Int64
	createCalls             atomic.Int64
}

func (r *producerCountingRepositoryV1) CreateWorkspaceReadExecutionCommandOwnedV1(
	ctx context.Context,
	capability ownerrepo.WriteCapabilityV1,
	value toolcontract.WorkspaceReadExecutionCommandV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, bool, error) {
	r.createCalls.Add(1)
	winner, created, err := r.inner.CreateWorkspaceReadExecutionCommandOwnedV1(
		ctx, capability, value,
	)
	if err != nil {
		return winner, created, err
	}
	if r.indeterminateAfterStore {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false,
			core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost SQLite commit reply")
	}
	return winner, created, nil
}

func (r *producerCountingRepositoryV1) InspectWorkspaceReadExecutionCommandExactV1(
	ctx context.Context,
	exact toolcontract.WorkspaceReadExecutionCommandRefV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	r.exactCalls.Add(1)
	return r.inner.InspectWorkspaceReadExecutionCommandExactV1(ctx, exact)
}

func (r *producerCountingRepositoryV1) InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(
	ctx context.Context,
	attempt runtimeports.OperationDispatchAttemptRefV3,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	r.reverseCalls.Add(1)
	value, err := r.inner.InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(ctx, attempt)
	if r.reverseBarrier != nil {
		r.reverseBarrier.wait()
	}
	return value, err
}

type producerReverseBarrierV1 struct {
	target  int64
	arrived atomic.Int64
	ready   chan struct{}
	once    sync.Once
}

func newProducerReverseBarrierV1(target int) *producerReverseBarrierV1 {
	return &producerReverseBarrierV1{target: int64(target), ready: make(chan struct{})}
}

func (b *producerReverseBarrierV1) wait() {
	if b.arrived.Add(1) == b.target {
		b.once.Do(func() { close(b.ready) })
	}
	<-b.ready
}

type producerEffectReaderV1 struct {
	runtimeports.ControlledOperationEffectCurrentReaderV2
	value       runtimeports.ControlledOperationEffectCurrentProjectionV2
	nextChecked time.Time
	step        time.Duration
	expires     time.Time
	state       string
	calls       atomic.Int64
	mutate      func(int64, *runtimeports.ControlledOperationEffectCurrentProjectionV2)
}

func (r *producerEffectReaderV1) InspectCurrentControlledOperationEffectV2(
	_ context.Context,
	operation runtimeports.OperationSubjectV3,
	effectID core.EffectIntentID,
) (runtimeports.ControlledOperationEffectCurrentProjectionV2, error) {
	call := r.calls.Add(1)
	value := r.value
	if !runtimeports.SameOperationSubjectV3(value.Intent.Operation, operation) ||
		value.Intent.ID != effectID {
		return runtimeports.ControlledOperationEffectCurrentProjectionV2{},
			core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Runtime Effect exact coordinates drifted")
	}
	if r.state != "" {
		value.State = r.state
	}
	value.CheckedUnixNano = r.nextChecked.Add(time.Duration(call-1) * r.step).UnixNano()
	value.ExpiresUnixNano = r.expires.UnixNano()
	if r.mutate != nil {
		r.mutate(call, &value)
	}
	value.Digest = ""
	return runtimeports.SealControlledOperationEffectCurrentProjectionV2(value)
}

type producerPreparedReaderV1 struct {
	runtimeports.ControlledOperationPreparedCurrentReaderV2
	value       runtimeports.ControlledOperationPreparedCurrentProjectionV2
	nextChecked time.Time
	step        time.Duration
	expires     time.Time
	calls       atomic.Int64
	mutate      func(int64, *runtimeports.ControlledOperationPreparedCurrentProjectionV2)
}

func (r *producerPreparedReaderV1) InspectCurrentControlledOperationPreparedV2(
	_ context.Context,
	exact runtimeports.PreparedProviderAttemptRefV2,
) (runtimeports.ControlledOperationPreparedCurrentProjectionV2, error) {
	call := r.calls.Add(1)
	value := r.value
	if value.Snapshot.Prepared != exact {
		return runtimeports.ControlledOperationPreparedCurrentProjectionV2{},
			core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Runtime Prepared exact Ref drifted")
	}
	value.CheckedUnixNano = r.nextChecked.Add(time.Duration(call-1) * r.step).UnixNano()
	value.ExpiresUnixNano = r.expires.UnixNano()
	if r.mutate != nil {
		r.mutate(call, &value)
	}
	value.ProjectionDigest = ""
	return runtimeports.SealControlledOperationPreparedCurrentProjectionV2(value)
}
