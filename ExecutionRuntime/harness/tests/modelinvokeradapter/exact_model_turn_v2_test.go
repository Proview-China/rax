package modelinvokeradapter_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	bridgecontract "github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	modelinvokeradapter "github.com/Proview-China/rax/ExecutionRuntime/harness/modelinvokeradapter"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var exactModelTurnNowV2 = time.Unix(1_900_000_000, 0)

type exactModelTurnFixtureV2 struct {
	prepared modelinvoker.PreparedModelInvocationFactV1
	current  modelinvoker.PreparedModelInvocationCurrentProjectionV1
	ack      modelinvoker.PreparedModelInvocationCommitAckV1
	material modelinvoker.InvocationMaterialV2
	envelope bridgecontract.ModelTurnExactEnvelopeV2
}

type preparedClosureReadersV2 struct {
	mu sync.Mutex

	prepared modelinvoker.PreparedModelInvocationFactV1
	current  modelinvoker.PreparedModelInvocationCurrentProjectionV1
	ack      modelinvoker.PreparedModelInvocationCommitAckV1

	preparedCalls int
	currentCalls  int
	ackCalls      int

	preparedDriftAt int
	currentDriftAt  int
	ackDriftAt      int

	preparedDrift modelinvoker.PreparedModelInvocationFactV1
	currentDrift  modelinvoker.PreparedModelInvocationCurrentProjectionV1
	ackDrift      modelinvoker.PreparedModelInvocationCommitAckV1

	unavailableRole string
}

func (r *preparedClosureReadersV2) InspectExactPreparedModelInvocationV1(
	_ context.Context,
	_ modelinvoker.PreparedModelInvocationRefV1,
) (modelinvoker.PreparedModelInvocationFactV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preparedCalls++
	if r.unavailableRole == "prepared" {
		return modelinvoker.PreparedModelInvocationFactV1{}, core.NewError(
			core.ErrorUnavailable,
			core.ReasonComponentMissing,
			"prepared history unavailable",
		)
	}
	if r.preparedDriftAt == r.preparedCalls {
		return r.preparedDrift.Clone(), nil
	}
	return r.prepared.Clone(), nil
}

func (r *preparedClosureReadersV2) InspectExactPreparedModelInvocationCurrentV1(
	_ context.Context,
	_ modelinvoker.PreparedModelInvocationCurrentRefV1,
) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentCalls++
	if r.unavailableRole == "current" {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, core.NewError(
			core.ErrorUnavailable,
			core.ReasonComponentMissing,
			"prepared current unavailable",
		)
	}
	if r.currentDriftAt == r.currentCalls {
		return r.currentDrift.Clone(), nil
	}
	return r.current.Clone(), nil
}

func (r *preparedClosureReadersV2) InspectExactAck(
	_ context.Context,
	_ modelinvoker.PreparedModelInvocationCommitAckRefV1,
) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ackCalls++
	if r.unavailableRole == "ack" {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, core.NewError(
			core.ErrorUnavailable,
			core.ReasonComponentMissing,
			"prepared ACK unavailable",
		)
	}
	if r.ackDriftAt == r.ackCalls {
		return r.ackDrift.Clone(), nil
	}
	return r.ack.Clone(), nil
}

type materialReaderV2 struct {
	mu       sync.Mutex
	material modelinvoker.InvocationMaterialV2
	calls    int
	driftAt  int
}

func (r *materialReaderV2) InspectExactInvocationMaterialV2(
	_ context.Context,
	_ modelinvoker.InvocationMaterialRefV2,
) (modelinvoker.InvocationMaterialV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	material := r.material.CloneV2()
	if r.driftAt > 0 && r.calls == r.driftAt {
		material.Authorization.SourceLineage.ContextFrame.ID += "-drift"
	}
	return material, nil
}

type pairReadersV2 struct {
	mu                sync.Mutex
	lineage           modelinvoker.InvocationMaterialSourceLineageV2
	checkedUnixNano   int64
	expiresUnixNano   int64
	contextCalls      int
	toolCalls         int
	driftContextAt    int
	driftToolAt       int
	driftContextField string
	driftToolField    string
}

func (r *pairReadersV2) InspectExactInvocationContextPairV2(
	_ context.Context,
	frame modelinvoker.InvocationMaterialExactSourceRefV1,
	material modelinvoker.InvocationMaterialExactSourceRefV1,
	mappedInput core.Digest,
) (modelinvoker.InvocationMaterialContextPairProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contextCalls++
	projection := modelinvoker.InvocationMaterialContextPairProjectionV2{
		ContextFrame:             frame,
		ContextMaterial:          material,
		ContextMappedInputDigest: mappedInput,
		CheckedUnixNano:          r.checkedUnixNano,
		ExpiresUnixNano:          r.expiresUnixNano,
	}
	if r.driftContextAt == r.contextCalls {
		switch r.driftContextField {
		case "frame":
			projection.ContextFrame.ID += "-drift"
		case "material":
			projection.ContextMaterial.ID += "-drift"
		}
	}
	return modelinvoker.SealInvocationMaterialContextPairProjectionV2(projection)
}

func (r *pairReadersV2) InspectExactInvocationToolPairV2(
	_ context.Context,
	injection modelinvoker.InvocationMaterialExactSourceRefV1,
	surface modelinvoker.InvocationMaterialExactSourceRefV1,
	requestTools core.Digest,
) (modelinvoker.InvocationMaterialToolPairProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toolCalls++
	projection := modelinvoker.InvocationMaterialToolPairProjectionV2{
		ToolInjectionMaterial:   injection,
		ToolSurface:             surface,
		ExpectedInjectionDigest: r.lineage.ExpectedInjectionDigest,
		CompiledToolsDigest:     r.lineage.CompiledToolsDigest,
		RequestToolsDigest:      requestTools,
		CheckedUnixNano:         r.checkedUnixNano,
		ExpiresUnixNano:         r.expiresUnixNano,
	}
	if r.driftToolAt == r.toolCalls {
		switch r.driftToolField {
		case "injection":
			projection.ToolInjectionMaterial.ID += "-drift"
		case "surface":
			projection.ToolSurface.ID += "-drift"
		}
	}
	return modelinvoker.SealInvocationMaterialToolPairProjectionV2(projection)
}

type governedModelTurnPortV3 struct {
	mu        sync.Mutex
	outcomes  map[string]modelinvoker.GovernedModelTurnOutcomeV3
	now       time.Time
	starts    int
	loseReply bool
}

type deadlineProbeDispatchRepositoryV2 struct {
	inner           *modelinvokeradapter.SQLiteModelTurnDispatchV2
	cancel          context.CancelFunc
	deadlines       chan time.Time
	mu              sync.Mutex
	recoverOnEnsure bool
	recoverOnBind   bool
	recoveryPending bool
}

func (r *deadlineProbeDispatchRepositoryV2) EnsureModelTurnDispatchAttemptV2(
	ctx context.Context,
	fact bridgecontract.ModelTurnDispatchFactV2,
) (bridgecontract.ModelTurnDispatchFactV2, error) {
	stored, err := r.inner.EnsureModelTurnDispatchAttemptV2(ctx, fact)
	if err != nil || !r.recoverOnEnsure {
		return stored, err
	}
	r.mu.Lock()
	r.recoveryPending = true
	r.mu.Unlock()
	r.cancel()
	return bridgecontract.ModelTurnDispatchFactV2{}, core.NewError(
		core.ErrorIndeterminate,
		core.ReasonEffectUnknownOutcome,
		"simulated lost Ensure reply",
	)
}

func (r *deadlineProbeDispatchRepositoryV2) BindModelTurnDispatchOutcomeV2(
	ctx context.Context,
	fact bridgecontract.ModelTurnDispatchFactV2,
) (bridgecontract.ModelTurnDispatchFactV2, error) {
	stored, err := r.inner.BindModelTurnDispatchOutcomeV2(ctx, fact)
	if err != nil || !r.recoverOnBind {
		return stored, err
	}
	r.mu.Lock()
	r.recoveryPending = true
	r.mu.Unlock()
	r.cancel()
	return bridgecontract.ModelTurnDispatchFactV2{}, core.NewError(
		core.ErrorIndeterminate,
		core.ReasonEffectUnknownOutcome,
		"simulated lost Bind reply",
	)
}

func (r *deadlineProbeDispatchRepositoryV2) InspectExactModelTurnDispatchV2(
	ctx context.Context,
	ref bridgecontract.ModelTurnDispatchRefV2,
) (bridgecontract.ModelTurnDispatchFactV2, error) {
	r.mu.Lock()
	recovery := r.recoveryPending
	if recovery {
		r.recoveryPending = false
	}
	r.mu.Unlock()
	if !recovery {
		return r.inner.InspectExactModelTurnDispatchV2(ctx, ref)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		r.deadlines <- time.Time{}
	} else {
		r.deadlines <- deadline
	}
	<-ctx.Done()
	return bridgecontract.ModelTurnDispatchFactV2{}, core.NewError(
		core.ErrorUnavailable,
		core.ReasonInvalidState,
		"deadline probe Inspect ended",
	)
}

type deadlineProbeModelTurnPortV3 struct {
	inner     *governedModelTurnPortV3
	cancel    context.CancelFunc
	deadlines chan time.Time
}

func (p *deadlineProbeModelTurnPortV3) StartOrInspectGovernedModelTurnV3(
	ctx context.Context,
	command modelinvoker.GovernedModelTurnCommandV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	if _, err := p.inner.StartOrInspectGovernedModelTurnV3(ctx, command); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	p.cancel()
	return modelinvoker.GovernedModelTurnOutcomeV3{}, &modelinvoker.GovernedModelInvocationErrorV1{
		Kind:      modelinvoker.GovernedModelInvocationErrorIndeterminate,
		Operation: "start_or_inspect_governed_model_turn_v3",
		Message:   "simulated lost Model reply",
	}
}

func (p *deadlineProbeModelTurnPortV3) InspectGovernedModelTurnAttemptV3(
	ctx context.Context,
	_ modelinvoker.GovernedModelTurnAttemptRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		p.deadlines <- time.Time{}
	} else {
		p.deadlines <- deadline
	}
	<-ctx.Done()
	return modelinvoker.GovernedModelTurnOutcomeV3{}, &modelinvoker.GovernedModelInvocationErrorV1{
		Kind:      modelinvoker.GovernedModelInvocationErrorUnavailable,
		Operation: "inspect_governed_model_turn_attempt_v3",
		Message:   "deadline probe Inspect ended",
	}
}

func (p *deadlineProbeModelTurnPortV3) InspectExactGovernedModelTurnV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	return p.inner.InspectExactGovernedModelTurnV3(ctx, ref)
}

func (p *governedModelTurnPortV3) StartOrInspectGovernedModelTurnV3(
	_ context.Context,
	command modelinvoker.GovernedModelTurnCommandV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.starts++
	attempt, err := modelinvoker.DeriveGovernedModelTurnAttemptRefV3(command)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	if outcome, exists := p.outcomes[attempt.ID]; exists {
		return outcome, nil
	}
	outcome, err := modelinvoker.NewPreparedGovernedModelTurnV3(command, p.now)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	p.outcomes[attempt.ID] = outcome
	if p.loseReply {
		p.loseReply = false
		return modelinvoker.GovernedModelTurnOutcomeV3{}, &modelinvoker.GovernedModelInvocationErrorV1{
			Kind:      modelinvoker.GovernedModelInvocationErrorIndeterminate,
			Operation: "start_or_inspect_governed_model_turn_v3",
			Message:   "simulated lost Model reply",
		}
	}
	return outcome, nil
}

func (p *governedModelTurnPortV3) InspectGovernedModelTurnAttemptV3(
	_ context.Context,
	attempt modelinvoker.GovernedModelTurnAttemptRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	outcome, exists := p.outcomes[attempt.ID]
	if !exists {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, modelNotFoundV2("attempt")
	}
	if outcome.AttemptRefV3() != attempt {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, modelConflictV2("attempt drift")
	}
	return outcome, nil
}

func (p *governedModelTurnPortV3) InspectExactGovernedModelTurnV3(
	_ context.Context,
	ref modelinvoker.GovernedModelTurnRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	outcome, exists := p.outcomes[ref.ID]
	if !exists {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, modelNotFoundV2("outcome")
	}
	if outcome.RefV3() != ref {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, modelConflictV2("outcome drift")
	}
	return outcome, nil
}

func (p *governedModelTurnPortV3) startCountV2() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.starts
}

func modelNotFoundV2(subject string) error {
	return &modelinvoker.GovernedModelInvocationErrorV1{
		Kind:      modelinvoker.GovernedModelInvocationErrorNotFound,
		Operation: "inspect_" + subject,
		Message:   subject + " was not found",
	}
}

func modelConflictV2(message string) error {
	return &modelinvoker.GovernedModelInvocationErrorV1{
		Kind:      modelinvoker.GovernedModelInvocationErrorConflict,
		Operation: "inspect_exact",
		Message:   message,
	}
}

func TestExactModelTurnV2PersistsAttemptAndOutcomeAcrossRestart(t *testing.T) {
	fixture := newExactModelTurnFixtureV2(t)
	path := filepath.Join(t.TempDir(), "model-turn-v2.db")
	store := openDispatchStoreV2(t, path)
	model := newGovernedModelTurnPortV3(t)
	adapter := newExactModelTurnAdapterV2(t, fixture, store, model, constantClockV2(exactModelTurnNowV2))

	result, err := adapter.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != bridgecontract.ModelTurnDispatchOutcomeBoundV2 || result.Outcome == nil {
		t.Fatalf("state=%q outcome=%v", result.State, result.Outcome)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openDispatchStoreV2(t, path)
	defer reopened.Close()
	restarted := newExactModelTurnAdapterV2(t, fixture, reopened, model, constantClockV2(exactModelTurnNowV2))
	replayed, err := restarted.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Digest != result.Digest || replayed.Revision != 2 {
		t.Fatal("restart did not return the exact durable outcome-bound winner")
	}
	if err := reopened.IntegrityCheckV2(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestExactModelTurnV2ContractRejectsSpliceAndNonCanonicalJSON(t *testing.T) {
	fixture := newExactModelTurnFixtureV2(t)
	spliced := fixture.envelope
	spliced.Command.DispatchSequence++
	if err := spliced.Validate(); err == nil {
		t.Fatal("Envelope accepted a DispatchSequence splice under the old digest")
	}
	if _, err := bridgecontract.SealModelTurnExactEnvelopeV2(spliced); err == nil {
		t.Fatal("Envelope reseal accepted a caller-supplied stale digest")
	}
	expired := fixture.envelope
	expired.Digest = ""
	expired.ID = ""
	expired.Revision = 0
	expired.ContractVersion = ""
	expired.RequestedNotAfterUnixNano = expired.Material.ExpiresUnixNano + 1
	if _, err := bridgecontract.SealModelTurnExactEnvelopeV2(expired); err == nil {
		t.Fatal("Envelope accepted a requested lifetime beyond the exact Material")
	}
	ackSplice := fixture.envelope
	ackSplice.AckRef.Digest = digestV2("spliced-ack")
	if err := ackSplice.Validate(); err == nil {
		t.Fatal("Envelope accepted a spliced ACK Ref under the old digest")
	}
	shortAck := fixture.ack
	shortAck.ID, shortAck.Digest = "", ""
	shortAck.ExpiresUnixNano = exactModelTurnNowV2.Add(7 * time.Minute).UnixNano()
	shortAck, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(shortAck)
	if err != nil {
		t.Fatal(err)
	}
	ackExpiredEnvelope := fixture.envelope
	ackExpiredEnvelope.ContractVersion = ""
	ackExpiredEnvelope.ID = ""
	ackExpiredEnvelope.Revision = 0
	ackExpiredEnvelope.Digest = ""
	ackExpiredEnvelope.AckRef = shortAck.Ref()
	if _, err := bridgecontract.SealModelTurnExactEnvelopeV2(ackExpiredEnvelope); err == nil {
		t.Fatal("Envelope accepted a requested lifetime beyond the exact ACK")
	}

	ref, err := bridgecontract.DeriveModelTurnDispatchRefV2(fixture.envelope)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := bridgecontract.NewModelTurnDispatchAttemptFactV2(ref)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := bridgecontract.EncodeModelTurnDispatchFactV2(fact)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	unknown := append([]byte(`{"unknown":true,`), wire[1:]...)
	trailing := append(append([]byte(nil), wire...), []byte(` {}`)...)
	stateField := []byte(`"state":"attempt_bound"`)
	duplicate := bytes.Replace(
		wire,
		stateField,
		append(append([]byte(nil), stateField...), append([]byte(","), stateField...)...),
		1,
	)
	for name, payload := range map[string][]byte{
		"unknown":   unknown,
		"trailing":  trailing,
		"duplicate": duplicate,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := bridgecontract.DecodeModelTurnDispatchFactV2(payload); err == nil {
				t.Fatal("non-canonical Dispatch Fact JSON was accepted")
			}
		})
	}
}

func TestExactModelTurnV2RecoversLostSidecarAndModelReplies(t *testing.T) {
	fixture := newExactModelTurnFixtureV2(t)
	store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "lost-reply.db"))
	defer store.Close()
	model := newGovernedModelTurnPortV3(t)
	model.loseReply = true
	store.LoseNextReplyForTestingV2()
	adapter := newExactModelTurnAdapterV2(t, fixture, store, model, constantClockV2(exactModelTurnNowV2))

	result, err := adapter.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != bridgecontract.ModelTurnDispatchOutcomeBoundV2 {
		t.Fatalf("state=%q", result.State)
	}

	secondFixture := newExactModelTurnFixtureWithSequenceV2(t, 2)
	secondModel := newGovernedModelTurnPortV3(t)
	secondRef, err := bridgecontract.DeriveModelTurnDispatchRefV2(secondFixture.envelope)
	if err != nil {
		t.Fatal(err)
	}
	secondAttempt, err := bridgecontract.NewModelTurnDispatchAttemptFactV2(secondRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsureModelTurnDispatchAttemptV2(context.Background(), secondAttempt); err != nil {
		t.Fatal(err)
	}
	store.LoseNextReplyForTestingV2()
	secondAdapter := newExactModelTurnAdapterV2(t, secondFixture, store, secondModel, constantClockV2(exactModelTurnNowV2))
	if result, err = secondAdapter.StartOrInspectExactModelTurnV2(context.Background(), secondFixture.envelope); err != nil {
		t.Fatal(err)
	}
	if result.State != bridgecontract.ModelTurnDispatchOutcomeBoundV2 {
		t.Fatal("lost outcome-bind reply was not recovered by exact Inspect")
	}
}

func TestExactModelTurnV2RejectsFourSourceS2Drift(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		contextField string
		toolField    string
	}{
		{name: "context frame", contextField: "frame"},
		{name: "context material", contextField: "material"},
		{name: "tool injection", toolField: "injection"},
		{name: "tool surface", toolField: "surface"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newExactModelTurnFixtureV2(t)
			store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "drift.db"))
			defer store.Close()
			readers := newPairReadersV2(fixture)
			readers.driftContextField, readers.driftToolField = testCase.contextField, testCase.toolField
			if testCase.contextField != "" {
				readers.driftContextAt = 2
			} else {
				readers.driftToolAt = 2
			}
			adapter := mustExactModelTurnAdapterV2(t, fixture, store, newGovernedModelTurnPortV3(t), readers, constantClockV2(exactModelTurnNowV2))
			if _, err := adapter.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope); err == nil {
				t.Fatal("S2 source drift was accepted")
			}
			ref, err := bridgecontract.DeriveModelTurnDispatchRefV2(fixture.envelope)
			if err != nil {
				t.Fatal(err)
			}
			fact, err := store.InspectExactModelTurnDispatchV2(context.Background(), ref)
			if err != nil {
				t.Fatal(err)
			}
			if fact.State != bridgecontract.ModelTurnDispatchAttemptBoundV2 {
				t.Fatal("source drift advanced the durable Harness outcome")
			}
		})
	}
}

func TestExactModelTurnV2RejectsPreparedCurrentAckBeforeModel(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(*testing.T, exactModelTurnFixtureV2, *preparedClosureReadersV2)
	}{
		{
			name: "prepared historical splice",
			configure: func(t *testing.T, fixture exactModelTurnFixtureV2, readers *preparedClosureReadersV2) {
				readers.preparedDriftAt = 1
				readers.preparedDrift = alternatePreparedHistoricalV2(t, fixture)
			},
		},
		{
			name: "current revision splice",
			configure: func(_ *testing.T, fixture exactModelTurnFixtureV2, readers *preparedClosureReadersV2) {
				readers.currentDriftAt = 1
				readers.currentDrift = fixture.current
				readers.currentDrift.Revision++
			},
		},
		{
			name: "current digest splice",
			configure: func(_ *testing.T, fixture exactModelTurnFixtureV2, readers *preparedClosureReadersV2) {
				readers.currentDriftAt = 1
				readers.currentDrift = fixture.current
				readers.currentDrift.Digest = digestV2("spliced-current-digest")
			},
		},
		{
			name: "current TTL drift",
			configure: func(t *testing.T, fixture exactModelTurnFixtureV2, readers *preparedClosureReadersV2) {
				readers.currentDriftAt = 1
				readers.currentDrift = alternatePreparedCurrentV2(
					t,
					fixture,
					exactModelTurnNowV2.Add(25*time.Minute),
				)
			},
		},
		{
			name: "ACK body splice",
			configure: func(_ *testing.T, fixture exactModelTurnFixtureV2, readers *preparedClosureReadersV2) {
				readers.ackDriftAt = 1
				readers.ackDrift = fixture.ack
				readers.ackDrift.GateImplementationRef.ID += "-spliced"
			},
		},
		{
			name: "ACK commit-gate drift",
			configure: func(t *testing.T, fixture exactModelTurnFixtureV2, readers *preparedClosureReadersV2) {
				readers.ackDriftAt = 1
				readers.ackDrift = alternatePreparedAckV2(
					t,
					fixture,
					exactModelTurnNowV2.Add(11*time.Minute),
					"drifted-gate",
				)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newExactModelTurnFixtureV2(t)
			store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "prepared-negative.db"))
			defer store.Close()
			readers := newPreparedClosureReadersV2(fixture)
			testCase.configure(t, fixture, readers)
			model := newGovernedModelTurnPortV3(t)
			adapter := mustExactModelTurnAdapterWithPreparedReadersV2(
				t,
				fixture,
				store,
				model,
				readers,
				newPairReadersV2(fixture),
				constantClockV2(exactModelTurnNowV2),
			)
			if _, err := adapter.StartOrInspectExactModelTurnV2(
				context.Background(),
				fixture.envelope,
			); err == nil {
				t.Fatal("Prepared/Current/ACK negative was accepted")
			}
			if got := model.startCountV2(); got != 0 {
				t.Fatalf("Model V3 starts=%d, want 0 before exact closure", got)
			}
		})
	}
	t.Run("ACK exact Ref splice", func(t *testing.T) {
		fixture := newExactModelTurnFixtureV2(t)
		fixture.envelope.ContractVersion = ""
		fixture.envelope.ID = ""
		fixture.envelope.Revision = 0
		fixture.envelope.Digest = ""
		fixture.envelope.AckRef.Digest = digestV2("alternate-ack-coordinate")
		envelope, err := bridgecontract.SealModelTurnExactEnvelopeV2(fixture.envelope)
		if err != nil {
			t.Fatal(err)
		}
		fixture.envelope = envelope
		store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "ack-ref-splice.db"))
		defer store.Close()
		model := newGovernedModelTurnPortV3(t)
		adapter := newExactModelTurnAdapterV2(
			t,
			fixture,
			store,
			model,
			constantClockV2(exactModelTurnNowV2),
		)
		if _, err = adapter.StartOrInspectExactModelTurnV2(
			context.Background(),
			fixture.envelope,
		); err == nil {
			t.Fatal("alternate ACK exact Ref was not rejected by exact read")
		}
		if got := model.startCountV2(); got != 0 {
			t.Fatalf("Model V3 starts=%d, want 0", got)
		}
	})
}

func TestExactModelTurnV2RejectsPreparedCurrentAckS2Drift(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(*testing.T, exactModelTurnFixtureV2, *preparedClosureReadersV2)
	}{
		{
			name: "prepared historical",
			configure: func(t *testing.T, fixture exactModelTurnFixtureV2, readers *preparedClosureReadersV2) {
				readers.preparedDriftAt = 2
				readers.preparedDrift = alternatePreparedHistoricalV2(t, fixture)
			},
		},
		{
			name: "prepared current",
			configure: func(t *testing.T, fixture exactModelTurnFixtureV2, readers *preparedClosureReadersV2) {
				readers.currentDriftAt = 2
				readers.currentDrift = alternatePreparedCurrentV2(
					t,
					fixture,
					exactModelTurnNowV2.Add(25*time.Minute),
				)
			},
		},
		{
			name: "commit ACK",
			configure: func(t *testing.T, fixture exactModelTurnFixtureV2, readers *preparedClosureReadersV2) {
				readers.ackDriftAt = 2
				readers.ackDrift = alternatePreparedAckV2(
					t,
					fixture,
					exactModelTurnNowV2.Add(11*time.Minute),
					"drifted-gate",
				)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newExactModelTurnFixtureV2(t)
			store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "prepared-s2-drift.db"))
			defer store.Close()
			readers := newPreparedClosureReadersV2(fixture)
			testCase.configure(t, fixture, readers)
			model := newGovernedModelTurnPortV3(t)
			adapter := mustExactModelTurnAdapterWithPreparedReadersV2(
				t,
				fixture,
				store,
				model,
				readers,
				newPairReadersV2(fixture),
				constantClockV2(exactModelTurnNowV2),
			)
			if _, err := adapter.StartOrInspectExactModelTurnV2(
				context.Background(),
				fixture.envelope,
			); err == nil {
				t.Fatal("Prepared/Current/ACK S2 drift was accepted")
			}
			if got := model.startCountV2(); got != 1 {
				t.Fatalf("Model V3 starts=%d, want the single call between S1 and S2", got)
			}
			ref, err := bridgecontract.DeriveModelTurnDispatchRefV2(fixture.envelope)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := store.InspectExactModelTurnDispatchV2(context.Background(), ref)
			if err != nil {
				t.Fatal(err)
			}
			if stored.State != bridgecontract.ModelTurnDispatchAttemptBoundV2 {
				t.Fatal("S2 drift advanced the durable Harness outcome")
			}
		})
	}
}

func TestExactModelTurnV2RejectsPreparedClosureUnavailableAndAckExpiry(t *testing.T) {
	for _, role := range []string{"prepared", "current", "ack"} {
		t.Run(role+" unavailable", func(t *testing.T) {
			fixture := newExactModelTurnFixtureV2(t)
			store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "prepared-unavailable.db"))
			defer store.Close()
			readers := newPreparedClosureReadersV2(fixture)
			readers.unavailableRole = role
			model := newGovernedModelTurnPortV3(t)
			adapter := mustExactModelTurnAdapterWithPreparedReadersV2(
				t,
				fixture,
				store,
				model,
				readers,
				newPairReadersV2(fixture),
				constantClockV2(exactModelTurnNowV2),
			)
			if _, err := adapter.StartOrInspectExactModelTurnV2(
				context.Background(),
				fixture.envelope,
			); !core.HasCategory(err, core.ErrorUnavailable) {
				t.Fatalf("error=%v, want unavailable", err)
			}
			if got := model.startCountV2(); got != 0 {
				t.Fatalf("Model V3 starts=%d, want 0", got)
			}
		})
	}

	t.Run("ACK now equals expiry", func(t *testing.T) {
		fixture := newExactModelTurnFixtureV2(t)
		expiringAck := alternatePreparedAckV2(
			t,
			fixture,
			exactModelTurnNowV2,
			"expiring-gate",
		)
		fixture.ack = expiringAck
		fixture.envelope.ContractVersion = ""
		fixture.envelope.ID = ""
		fixture.envelope.Revision = 0
		fixture.envelope.Digest = ""
		fixture.envelope.AckRef = expiringAck.Ref()
		fixture.envelope.RequestedNotAfterUnixNano = exactModelTurnNowV2.UnixNano()
		envelope, err := bridgecontract.SealModelTurnExactEnvelopeV2(fixture.envelope)
		if err != nil {
			t.Fatal(err)
		}
		fixture.envelope = envelope
		store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "ack-expired.db"))
		defer store.Close()
		model := newGovernedModelTurnPortV3(t)
		adapter := newExactModelTurnAdapterV2(
			t,
			fixture,
			store,
			model,
			constantClockV2(exactModelTurnNowV2),
		)
		if _, err = adapter.StartOrInspectExactModelTurnV2(
			context.Background(),
			fixture.envelope,
		); err == nil {
			t.Fatal("ACK exact-expiry was accepted")
		}
		if got := model.startCountV2(); got != 0 {
			t.Fatalf("Model V3 starts=%d, want 0", got)
		}
	})
}

func TestExactModelTurnV2RestartRecoversAttemptBoundWithoutRedispatchIdentity(t *testing.T) {
	fixture := newExactModelTurnFixtureV2(t)
	path := filepath.Join(t.TempDir(), "attempt-recovery.db")
	store := openDispatchStoreV2(t, path)
	model := newGovernedModelTurnPortV3(t)
	drifting := newPairReadersV2(fixture)
	drifting.driftToolAt, drifting.driftToolField = 2, "surface"
	first := mustExactModelTurnAdapterV2(t, fixture, store, model, drifting, constantClockV2(exactModelTurnNowV2))
	if _, err := first.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope); err == nil {
		t.Fatal("first process unexpectedly crossed a drifting S2")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openDispatchStoreV2(t, path)
	defer reopened.Close()
	restarted := newExactModelTurnAdapterV2(t, fixture, reopened, model, constantClockV2(exactModelTurnNowV2))
	result, err := restarted.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != bridgecontract.ModelTurnDispatchOutcomeBoundV2 ||
		result.Outcome == nil || result.Attempt != result.Outcome.AttemptRefV3() {
		t.Fatal("restart failed to associate the exact pre-existing Model attempt")
	}
}

func TestExactModelTurnV2FailsClosedOnTTLCrossingAndClockRollback(t *testing.T) {
	t.Run("ttl crossing", func(t *testing.T) {
		fixture := newExactModelTurnFixtureV2(t)
		store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "ttl.db"))
		defer store.Close()
		readers := newPairReadersV2(fixture)
		expiry := time.Unix(0, readers.expiresUnixNano)
		clock := clockSequenceV2(exactModelTurnNowV2, exactModelTurnNowV2, expiry)
		adapter := mustExactModelTurnAdapterV2(t, fixture, store, newGovernedModelTurnPortV3(t), readers, clock)
		if _, err := adapter.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope); !core.HasCategory(err, core.ErrorPreconditionFailed) {
			t.Fatalf("error=%v, want precondition_failed", err)
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newExactModelTurnFixtureV2(t)
		store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "rollback.db"))
		defer store.Close()
		clock := clockSequenceV2(exactModelTurnNowV2, exactModelTurnNowV2.Add(-time.Second))
		adapter := newExactModelTurnAdapterV2(t, fixture, store, newGovernedModelTurnPortV3(t), clock)
		if _, err := adapter.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("error=%v, want clock regression", err)
		}
	})
}

func TestExactModelTurnV2FailsClosedAfterDurableOutcomeBind(t *testing.T) {
	t.Run("normal Bind then clock rollback", func(t *testing.T) {
		fixture := newExactModelTurnFixtureV2(t)
		store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "post-bind-rollback.db"))
		defer store.Close()
		clock := clockSequenceV2(
			exactModelTurnNowV2,
			exactModelTurnNowV2,
			exactModelTurnNowV2,
			exactModelTurnNowV2.Add(-time.Nanosecond),
		)
		adapter := newExactModelTurnAdapterV2(t, fixture, store, newGovernedModelTurnPortV3(t), clock)
		result, err := adapter.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope)
		if !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("error=%v, want post-Bind clock regression", err)
		}
		if result.State == bridgecontract.ModelTurnDispatchOutcomeBoundV2 {
			t.Fatal("post-Bind clock rollback returned outcome_bound")
		}
		assertDurableOutcomeBoundV2(t, store, fixture)
	})

	t.Run("recovered Bind then now equals expiry", func(t *testing.T) {
		fixture := newExactModelTurnFixtureV2(t)
		store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "post-bind-expiry.db"))
		defer store.Close()
		ref, err := bridgecontract.DeriveModelTurnDispatchRefV2(fixture.envelope)
		if err != nil {
			t.Fatal(err)
		}
		attempt, err := bridgecontract.NewModelTurnDispatchAttemptFactV2(ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.EnsureModelTurnDispatchAttemptV2(context.Background(), attempt); err != nil {
			t.Fatal(err)
		}
		store.LoseNextReplyForTestingV2()
		expiry := time.Unix(0, fixture.envelope.RequestedNotAfterUnixNano)
		clock := clockSequenceV2(
			exactModelTurnNowV2,
			exactModelTurnNowV2,
			exactModelTurnNowV2,
			expiry,
		)
		adapter := newExactModelTurnAdapterV2(t, fixture, store, newGovernedModelTurnPortV3(t), clock)
		result, err := adapter.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope)
		if !core.HasCategory(err, core.ErrorPreconditionFailed) ||
			!core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("error=%v, want post-Bind exact-expiry rejection", err)
		}
		if result.State == bridgecontract.ModelTurnDispatchOutcomeBoundV2 {
			t.Fatal("post-Bind exact expiry returned outcome_bound")
		}
		assertDurableOutcomeBoundV2(t, store, fixture)
	})
}

func assertDurableOutcomeBoundV2(
	t *testing.T,
	store *modelinvokeradapter.SQLiteModelTurnDispatchV2,
	fixture exactModelTurnFixtureV2,
) {
	t.Helper()
	ref, err := bridgecontract.DeriveModelTurnDispatchRefV2(fixture.envelope)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.InspectExactModelTurnDispatchV2(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != bridgecontract.ModelTurnDispatchOutcomeBoundV2 || stored.Outcome == nil {
		t.Fatalf("durable post-Bind state=%q outcome=%v", stored.State, stored.Outcome)
	}
}

func TestExactModelTurnV2ConcurrentAdaptersLinearizeOneDurableWinner(t *testing.T) {
	fixture := newExactModelTurnFixtureV2(t)
	path := filepath.Join(t.TempDir(), "concurrent.db")
	store := openDispatchStoreV2(t, path)
	defer store.Close()
	model := newGovernedModelTurnPortV3(t)
	readers := newPairReadersV2(fixture)

	const workers = 64
	results := make(chan bridgecontract.ModelTurnDispatchFactV2, workers)
	failures := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			adapter := mustExactModelTurnAdapterV2(t, fixture, store, model, readers, constantClockV2(exactModelTurnNowV2))
			result, err := adapter.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope)
			if err != nil {
				failures <- err
				return
			}
			results <- result
		}()
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var winner core.Digest
	for result := range results {
		if result.State != bridgecontract.ModelTurnDispatchOutcomeBoundV2 {
			t.Fatalf("state=%q", result.State)
		}
		if winner == "" {
			winner = result.Digest
		} else if result.Digest != winner {
			t.Fatal("concurrent adapters returned different durable winners")
		}
	}
}

func TestExactModelTurnV2TypedNilAndCancellationFailClosed(t *testing.T) {
	var material *materialReaderV2
	fixture := newExactModelTurnFixtureV2(t)
	store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "typed-nil.db"))
	defer store.Close()
	_, err := modelinvokeradapter.NewExactModelTurnAdapterV2(modelinvokeradapter.ExactModelTurnAdapterConfigV2{
		Readers: modelinvokeradapter.ExactModelTurnClosureReadersV2{
			Materials:   material,
			ContextPair: newPairReadersV2(fixture),
			ToolPair:    newPairReadersV2(fixture),
		},
		Dispatches: store,
		Model:      newGovernedModelTurnPortV3(t),
		Clock:      constantClockV2(exactModelTurnNowV2),
	})
	if err == nil {
		t.Fatal("typed-nil Material reader was accepted")
	}
	validPrepared := newPreparedClosureReadersV2(fixture)
	var nilPrepared *preparedClosureReadersV2
	for _, role := range []string{"prepared_history", "prepared_current", "ack_history"} {
		t.Run("typed nil "+role, func(t *testing.T) {
			readers := modelinvokeradapter.ExactModelTurnClosureReadersV2{
				PreparedHistory: validPrepared,
				PreparedCurrent: validPrepared,
				AckHistory:      validPrepared,
				Materials:       &materialReaderV2{material: fixture.material},
				ContextPair:     newPairReadersV2(fixture),
				ToolPair:        newPairReadersV2(fixture),
			}
			switch role {
			case "prepared_history":
				readers.PreparedHistory = nilPrepared
			case "prepared_current":
				readers.PreparedCurrent = nilPrepared
			case "ack_history":
				readers.AckHistory = nilPrepared
			}
			adapter, constructorErr := modelinvokeradapter.NewExactModelTurnAdapterV2(
				modelinvokeradapter.ExactModelTurnAdapterConfigV2{
					Readers:    readers,
					Dispatches: store,
					Model:      newGovernedModelTurnPortV3(t),
					Clock:      constantClockV2(exactModelTurnNowV2),
				},
			)
			if constructorErr == nil || adapter != nil {
				t.Fatal("typed-nil Prepared/ACK reader was accepted")
			}
		})
	}
	adapter := newExactModelTurnAdapterV2(t, fixture, store, newGovernedModelTurnPortV3(t), constantClockV2(exactModelTurnNowV2))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = adapter.StartOrInspectExactModelTurnV2(ctx, fixture.envelope); err == nil {
		t.Fatal("canceled request was accepted")
	}
	var nilAdapter *modelinvokeradapter.ExactModelTurnAdapterV2
	if _, err = nilAdapter.StartOrInspectExactModelTurnV2(context.Background(), fixture.envelope); err == nil {
		t.Fatal("typed-nil adapter receiver was accepted")
	}
	var nilStore *modelinvokeradapter.SQLiteModelTurnDispatchV2
	if _, err = nilStore.InspectExactModelTurnDispatchV2(context.Background(), bridgecontract.ModelTurnDispatchRefV2{}); err == nil {
		t.Fatal("typed-nil SQLite receiver was accepted")
	}
}

func TestExactModelTurnV2RecoveryInspectUsesExactTTLDeadline(t *testing.T) {
	t.Run("attempt ensure", func(t *testing.T) {
		expectedDeadline := time.Now().Add(500 * time.Millisecond)
		now := time.Unix(0, expectedDeadline.UnixNano()).Add(-8 * time.Minute)
		fixture := newExactModelTurnFixtureAtV2(t, 1, now)
		store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "ensure-deadline.db"))
		defer store.Close()
		ctx, cancel := context.WithCancel(context.Background())
		probe := &deadlineProbeDispatchRepositoryV2{
			inner:           store,
			cancel:          cancel,
			deadlines:       make(chan time.Time, 1),
			recoverOnEnsure: true,
		}
		adapter := mustExactModelTurnAdapterV2(
			t,
			fixture,
			probe,
			newGovernedModelTurnPortAtV3(t, now),
			newPairReadersAtV2(fixture, now),
			constantClockV2(now),
		)
		if _, err := adapter.StartOrInspectExactModelTurnV2(ctx, fixture.envelope); err == nil {
			t.Fatal("attempt Ensure deadline probe unexpectedly succeeded")
		}
		assertExactRecoveryDeadlineV2(t, <-probe.deadlines, expectedDeadline)
	})

	t.Run("model attempt", func(t *testing.T) {
		expectedDeadline := time.Now().Add(500 * time.Millisecond)
		now := time.Unix(0, expectedDeadline.UnixNano()).Add(-6 * time.Minute)
		fixture := newExactModelTurnFixtureAtV2(t, 1, now)
		store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "model-deadline.db"))
		defer store.Close()
		ctx, cancel := context.WithCancel(context.Background())
		probe := &deadlineProbeModelTurnPortV3{
			inner:     newGovernedModelTurnPortAtV3(t, now),
			cancel:    cancel,
			deadlines: make(chan time.Time, 1),
		}
		pairs := newPairReadersAtV2(fixture, now)
		pairs.expiresUnixNano = now.Add(6 * time.Minute).UnixNano()
		adapter := mustExactModelTurnAdapterV2(
			t,
			fixture,
			store,
			probe,
			pairs,
			constantClockV2(now),
		)
		if _, err := adapter.StartOrInspectExactModelTurnV2(ctx, fixture.envelope); err == nil {
			t.Fatal("Model attempt deadline probe unexpectedly succeeded")
		}
		assertExactRecoveryDeadlineV2(t, <-probe.deadlines, expectedDeadline)
	})

	t.Run("outcome bind", func(t *testing.T) {
		expectedDeadline := time.Now().Add(500 * time.Millisecond)
		now := time.Unix(0, expectedDeadline.UnixNano()).Add(-6 * time.Minute)
		fixture := newExactModelTurnFixtureAtV2(t, 1, now)
		store := openDispatchStoreV2(t, filepath.Join(t.TempDir(), "bind-deadline.db"))
		defer store.Close()
		ctx, cancel := context.WithCancel(context.Background())
		probe := &deadlineProbeDispatchRepositoryV2{
			inner:         store,
			cancel:        cancel,
			deadlines:     make(chan time.Time, 1),
			recoverOnBind: true,
		}
		pairs := newPairReadersAtV2(fixture, now)
		pairs.expiresUnixNano = now.Add(6 * time.Minute).UnixNano()
		adapter := mustExactModelTurnAdapterV2(
			t,
			fixture,
			probe,
			newGovernedModelTurnPortAtV3(t, now),
			pairs,
			constantClockV2(now),
		)
		if _, err := adapter.StartOrInspectExactModelTurnV2(ctx, fixture.envelope); err == nil {
			t.Fatal("Outcome Bind deadline probe unexpectedly succeeded")
		}
		assertExactRecoveryDeadlineV2(t, <-probe.deadlines, expectedDeadline)
	})
}

func assertExactRecoveryDeadlineV2(t *testing.T, actual time.Time, expected time.Time) {
	t.Helper()
	if actual.IsZero() || actual.UnixNano() != expected.UnixNano() {
		t.Fatalf("recovery deadline=%v, want exact TTL %v", actual, expected)
	}
	if time.Now().Add(25 * time.Millisecond).Before(expected) {
		t.Fatal("recovery Inspect inherited caller cancellation instead of waiting for exact TTL")
	}
}

func TestSQLiteModelTurnDispatchV2RejectsWeakPhysicalSchemaOnReopen(t *testing.T) {
	const factTable = `CREATE TABLE harness_model_turn_dispatch_v2 (
	  dispatch_id TEXT PRIMARY KEY,
	  dispatch_ref_digest TEXT NOT NULL,
	  ack_ref_digest TEXT NOT NULL,
	  fact_digest TEXT NOT NULL,
	  revision INTEGER NOT NULL CHECK(revision > 0),
	  state TEXT NOT NULL,
	  not_after_unix_nano INTEGER NOT NULL CHECK(not_after_unix_nano > 0),
	  canonical_json BLOB NOT NULL
	)`
	const exactIndex = `CREATE INDEX harness_model_turn_dispatch_v2_exact
	  ON harness_model_turn_dispatch_v2(dispatch_id,dispatch_ref_digest,ack_ref_digest,fact_digest,revision,state,not_after_unix_nano)`
	testCases := []struct {
		name   string
		mutate func(*testing.T, *sql.DB)
	}{
		{
			name: "missing primary key",
			mutate: func(t *testing.T, db *sql.DB) {
				rebuildSQLiteModelTurnDispatchFactTableV2(
					t,
					db,
					strings.Replace(factTable, "dispatch_id TEXT PRIMARY KEY", "dispatch_id TEXT NOT NULL", 1),
					exactIndex,
				)
			},
		},
		{
			name: "extra column",
			mutate: func(t *testing.T, db *sql.DB) {
				rebuildSQLiteModelTurnDispatchFactTableV2(
					t,
					db,
					strings.Replace(factTable, "canonical_json BLOB NOT NULL", "canonical_json BLOB NOT NULL,\nextra_column TEXT", 1),
					exactIndex,
				)
			},
		},
		{
			name: "missing ACK digest column",
			mutate: func(t *testing.T, db *sql.DB) {
				rebuildSQLiteModelTurnDispatchFactTableV2(
					t,
					db,
					strings.Replace(factTable, "\n\t  ack_ref_digest TEXT NOT NULL,", "", 1),
					strings.Replace(exactIndex, ",ack_ref_digest", "", 1),
				)
			},
		},
		{
			name: "wrong same-name index",
			mutate: func(t *testing.T, db *sql.DB) {
				rebuildSQLiteModelTurnDispatchFactTableV2(
					t,
					db,
					factTable,
					`CREATE INDEX harness_model_turn_dispatch_v2_exact ON harness_model_turn_dispatch_v2(state)`,
				)
			},
		},
		{
			name: "missing exact index with applied ledger",
			mutate: func(t *testing.T, db *sql.DB) {
				t.Helper()
				if _, err := db.Exec(`DROP INDEX harness_model_turn_dispatch_v2_exact`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing fact table with applied ledger",
			mutate: func(t *testing.T, db *sql.DB) {
				t.Helper()
				if _, err := db.Exec(`DROP TABLE harness_model_turn_dispatch_v2`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "partial schema without ledger",
			mutate: func(t *testing.T, db *sql.DB) {
				t.Helper()
				if _, err := db.Exec(`DROP TABLE harness_model_turn_dispatch_schema_v2`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "weakened check constraint",
			mutate: func(t *testing.T, db *sql.DB) {
				rebuildSQLiteModelTurnDispatchFactTableV2(
					t,
					db,
					strings.Replace(factTable, " CHECK(revision > 0)", "", 1),
					exactIndex,
				)
			},
		},
		{
			name: "extra schema ledger version",
			mutate: func(t *testing.T, db *sql.DB) {
				t.Helper()
				if _, err := db.Exec(
					`INSERT INTO harness_model_turn_dispatch_schema_v2(version,digest,applied_unix_nano) VALUES(999,?,?)`,
					string(digestV2("unknown-schema")),
					exactModelTurnNowV2.UnixNano(),
				); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "weak-schema.db")
			store := openDispatchStoreV2(t, path)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			testCase.mutate(t, db)
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := modelinvokeradapter.OpenSQLiteModelTurnDispatchV2(
				context.Background(),
				modelinvokeradapter.SQLiteModelTurnDispatchConfigV2{
					Path:  path,
					Clock: constantClockV2(exactModelTurnNowV2),
				},
			)
			if reopened != nil {
				_ = reopened.Close()
			}
			if !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("weak physical schema reopen error=%v, want Conflict", err)
			}
		})
	}
}

func TestSQLiteModelTurnDispatchV2RejectsAckDigestRowSplice(t *testing.T) {
	fixture := newExactModelTurnFixtureV2(t)
	path := filepath.Join(t.TempDir(), "ack-row-splice.db")
	store := openDispatchStoreV2(t, path)
	ref, err := bridgecontract.DeriveModelTurnDispatchRefV2(fixture.envelope)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := bridgecontract.NewModelTurnDispatchAttemptFactV2(ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsureModelTurnDispatchAttemptV2(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(
		`UPDATE harness_model_turn_dispatch_v2 SET ack_ref_digest=? WHERE dispatch_id=?`,
		string(digestV2("spliced-ack-row")),
		ref.ID,
	); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openDispatchStoreV2(t, path)
	defer reopened.Close()
	if _, err = reopened.InspectExactModelTurnDispatchV2(
		context.Background(),
		ref,
	); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("ack digest row splice error=%v, want Conflict", err)
	}
}

func TestSQLiteModelTurnDispatchV2PhysicalVerificationSupportsOneConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	store, err := modelinvokeradapter.OpenSQLiteModelTurnDispatchV2(
		ctx,
		modelinvokeradapter.SQLiteModelTurnDispatchConfigV2{
			Path:         filepath.Join(t.TempDir(), "single-connection.db"),
			MaxOpenConns: 1,
			Clock:        constantClockV2(exactModelTurnNowV2),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
}

func rebuildSQLiteModelTurnDispatchFactTableV2(
	t *testing.T,
	db *sql.DB,
	tableDDL string,
	indexDDL string,
) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`DROP TABLE harness_model_turn_dispatch_v2`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(tableDDL); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(indexDDL); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func newExactModelTurnAdapterV2(
	t *testing.T,
	fixture exactModelTurnFixtureV2,
	store *modelinvokeradapter.SQLiteModelTurnDispatchV2,
	model *governedModelTurnPortV3,
	clock func() time.Time,
) *modelinvokeradapter.ExactModelTurnAdapterV2 {
	t.Helper()
	return mustExactModelTurnAdapterV2(t, fixture, store, model, newPairReadersV2(fixture), clock)
}

func mustExactModelTurnAdapterV2(
	t *testing.T,
	fixture exactModelTurnFixtureV2,
	store harnessports.ModelTurnDispatchRepositoryV2,
	model modelinvoker.GovernedModelTurnPortV3,
	pairs *pairReadersV2,
	clock func() time.Time,
) *modelinvokeradapter.ExactModelTurnAdapterV2 {
	t.Helper()
	return mustExactModelTurnAdapterWithPreparedReadersV2(
		t,
		fixture,
		store,
		model,
		newPreparedClosureReadersV2(fixture),
		pairs,
		clock,
	)
}

func mustExactModelTurnAdapterWithPreparedReadersV2(
	t *testing.T,
	fixture exactModelTurnFixtureV2,
	store harnessports.ModelTurnDispatchRepositoryV2,
	model modelinvoker.GovernedModelTurnPortV3,
	prepared *preparedClosureReadersV2,
	pairs *pairReadersV2,
	clock func() time.Time,
) *modelinvokeradapter.ExactModelTurnAdapterV2 {
	t.Helper()
	adapter, err := modelinvokeradapter.NewExactModelTurnAdapterV2(modelinvokeradapter.ExactModelTurnAdapterConfigV2{
		Readers: modelinvokeradapter.ExactModelTurnClosureReadersV2{
			PreparedHistory: prepared,
			PreparedCurrent: prepared,
			AckHistory:      prepared,
			Materials:       &materialReaderV2{material: fixture.material},
			ContextPair:     pairs,
			ToolPair:        pairs,
		},
		Dispatches: store,
		Model:      model,
		Clock:      clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func newPreparedClosureReadersV2(fixture exactModelTurnFixtureV2) *preparedClosureReadersV2 {
	return &preparedClosureReadersV2{
		prepared: fixture.prepared,
		current:  fixture.current,
		ack:      fixture.ack,
	}
}

func alternatePreparedHistoricalV2(
	t *testing.T,
	fixture exactModelTurnFixtureV2,
) modelinvoker.PreparedModelInvocationFactV1 {
	t.Helper()
	prepared := fixture.prepared
	prepared.ContractVersion = ""
	prepared.ID = ""
	prepared.Revision = 0
	prepared.Digest = ""
	prepared.InvocationID += "-drift"
	prepared.InvocationDigest = digestV2("drifted-prepared-request")
	prepared.UnifiedRequestDigest = prepared.InvocationDigest
	sealed, err := modelinvoker.SealPreparedModelInvocationFactV1(prepared)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func alternatePreparedCurrentV2(
	t *testing.T,
	fixture exactModelTurnFixtureV2,
	expires time.Time,
) modelinvoker.PreparedModelInvocationCurrentProjectionV1 {
	t.Helper()
	current := fixture.current
	current.ContractVersion = ""
	current.ID = ""
	current.Revision = 0
	current.Digest = ""
	current.ExpiresUnixNano = expires.UnixNano()
	sealed, err := modelinvoker.SealPreparedModelInvocationCurrentV1(current)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func alternatePreparedAckV2(
	t *testing.T,
	fixture exactModelTurnFixtureV2,
	expires time.Time,
	gateID string,
) modelinvoker.PreparedModelInvocationCommitAckV1 {
	t.Helper()
	ack := fixture.ack
	ack.ContractVersion = ""
	ack.ID = ""
	ack.Revision = 0
	ack.Digest = ""
	ack.ExpiresUnixNano = expires.UnixNano()
	ack.GateImplementationRef.ID = gateID
	ack.GateImplementationRef.Digest = digestV2(gateID)
	sealed, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(ack)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func openDispatchStoreV2(t *testing.T, path string) *modelinvokeradapter.SQLiteModelTurnDispatchV2 {
	t.Helper()
	store, err := modelinvokeradapter.OpenSQLiteModelTurnDispatchV2(
		context.Background(),
		modelinvokeradapter.SQLiteModelTurnDispatchConfigV2{
			Path:  path,
			Clock: constantClockV2(exactModelTurnNowV2),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func newGovernedModelTurnPortV3(t *testing.T) *governedModelTurnPortV3 {
	t.Helper()
	return newGovernedModelTurnPortAtV3(t, exactModelTurnNowV2)
}

func newGovernedModelTurnPortAtV3(t *testing.T, now time.Time) *governedModelTurnPortV3 {
	t.Helper()
	return &governedModelTurnPortV3{
		outcomes: make(map[string]modelinvoker.GovernedModelTurnOutcomeV3),
		now:      now.Add(time.Second),
	}
}

func newPairReadersV2(fixture exactModelTurnFixtureV2) *pairReadersV2 {
	return newPairReadersAtV2(fixture, exactModelTurnNowV2)
}

func newPairReadersAtV2(fixture exactModelTurnFixtureV2, now time.Time) *pairReadersV2 {
	return &pairReadersV2{
		lineage:         fixture.material.Authorization.SourceLineage,
		checkedUnixNano: now.Add(-time.Minute).UnixNano(),
		expiresUnixNano: now.Add(15 * time.Minute).UnixNano(),
	}
}

func newExactModelTurnFixtureV2(t *testing.T) exactModelTurnFixtureV2 {
	t.Helper()
	return newExactModelTurnFixtureWithSequenceV2(t, 1)
}

func newExactModelTurnFixtureWithSequenceV2(t *testing.T, sequence uint64) exactModelTurnFixtureV2 {
	t.Helper()
	return newExactModelTurnFixtureAtV2(t, sequence, exactModelTurnNowV2)
}

func newExactModelTurnFixtureAtV2(t *testing.T, sequence uint64, now time.Time) exactModelTurnFixtureV2 {
	t.Helper()
	strict, parallel := true, false
	call := modelinvoker.RouteCall{
		RouteID: "harness.exact.model-turn.v2",
		Invocation: upstream.InvocationContext{
			Usage:     upstream.InvocationGeneralAPI,
			Subject:   upstream.SubjectService,
			Tenancy:   upstream.TenancyMulti,
			Execution: upstream.ExecutionForeground,
		},
		Request: modelinvoker.Request{
			Model: "test-model",
			Input: []modelinvoker.InputItem{
				modelinvoker.MessageInput(modelinvoker.RoleUser, "read one file"),
			},
			Tools: []modelinvoker.Tool{{
				Name:        "workspace.read",
				Description: "read one bounded file",
				Parameters:  []byte(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
				Strict:      &strict,
			}},
			ToolChoice:        modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired},
			ParallelToolCalls: &parallel,
			Budget:            modelinvoker.Budget{MaxOutputTokens: 64, Timeout: time.Minute},
		},
	}
	requestTools := mustDigestRequestToolsV2(t, call)
	contextMapped := mustDigestContextV2(t, call)
	requestDigest := digestV2("request")
	routeDigest := digestV2("route")
	profileDigest := digestV2("profile")
	expectedInjection := digestV2("expected-injection")
	providerInjection := digestV2("provider-injection")
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{
		InvocationID:                  "harness-model-turn-v2-invocation",
		InvocationDigest:              requestDigest,
		UnifiedRequestDigest:          requestDigest,
		RequestToolsDigest:            requestTools,
		PreparedPlanDigest:            digestV2("plan"),
		RouteDigest:                   routeDigest,
		ProfileDigest:                 profileDigest,
		ActualToolSurfaceDigest:       expectedInjection,
		ActualProviderInjectionDigest: providerInjection,
		CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{
			ContractVersion: "1.0.0",
			ID:              "capabilities",
			Revision:        1,
			Digest:          digestV2("capabilities"),
		},
		RegistrySnapshotRef: runtimeports.RegistrySnapshotRefV1{
			Owner:           ownerV2("registry", "registry-owner"),
			ContractVersion: "1.0.0",
			ID:              "registry",
			Revision:        1,
			Digest:          digestV2("registry"),
		},
		CreatedUnixNano:  now.Add(-5 * time.Minute).UnixNano(),
		NotAfterUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{
		Prepared:                      prepared.Ref(),
		CapabilitySnapshotRef:         prepared.CapabilitySnapshotRef,
		RegistrySnapshotRef:           prepared.RegistrySnapshotRef,
		ActualToolSurfaceDigest:       prepared.ActualToolSurfaceDigest,
		ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
		CheckedUnixNano:               now.Add(-2 * time.Minute).UnixNano(),
		ExpiresUnixNano:               now.Add(30 * time.Minute).UnixNano(),
		NotAfterUnixNano:              prepared.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(
		modelinvoker.PreparedModelInvocationCommitAckV1{
			PreparedRef: prepared.Ref(),
			CurrentRef:  current.Ref(),
			GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{
				Owner:           ownerV2("harness", "prepared-commit-gate"),
				ContractVersion: "1.0.0",
				ID:              "prepared-commit-gate",
				Revision:        1,
				Digest:          digestV2("prepared-commit-gate"),
			},
			SurfaceBindingRef: modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{
				Owner:           ownerV2("tool", "surface-binding-owner"),
				ContractVersion: "1.0.0",
				ID:              "surface-binding",
				Revision:        1,
				Digest:          digestV2("surface-binding"),
			},
			CheckedUnixNano:  now.Add(-time.Minute).UnixNano(),
			ExpiresUnixNano:  now.Add(12 * time.Minute).UnixNano(),
			NotAfterUnixNano: current.NotAfterUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Digest == current.Digest || prepared.Digest == ack.Digest ||
		current.Digest == ack.Digest {
		t.Fatal("Prepared, Current, and ACK fixture digests must be distinct")
	}
	contextOwner, toolOwner := ownerV2("context", "context-owner"), ownerV2("tool", "tool-owner")
	lineage, err := modelinvoker.SealInvocationMaterialSourceLineageV2(modelinvoker.InvocationMaterialSourceLineageV2{
		ContextFrame:             exactOwnedRefV2(contextOwner, modelinvoker.InvocationMaterialContextFrameKindV2, "frame", digestV2("frame")),
		ContextMaterial:          exactOwnedRefV2(contextOwner, modelinvoker.InvocationMaterialContextMaterialKindV2, "material", digestV2("material")),
		ToolInjectionMaterial:    exactOwnedRefV2(toolOwner, modelinvoker.InvocationMaterialToolInjectionMaterialKindV2, "injection", digestV2("injection")),
		ToolSurface:              exactOwnedRefV2(toolOwner, modelinvoker.InvocationMaterialToolSurfaceKindV2, "surface", digestV2("surface")),
		ContextMappedInputDigest: contextMapped,
		ExpectedInjectionDigest:  expectedInjection,
		CompiledToolsDigest:      digestV2("compiled-tools"),
		RequestToolsDigest:       requestTools,
	})
	if err != nil {
		t.Fatal(err)
	}
	routeCallDigest, err := modelinvoker.DigestGovernedModelTurnRouteCallV2(call)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := modelinvoker.SealInvocationMaterialAuthorizationV2(modelinvoker.InvocationMaterialAuthorizationV2{
		PreparedRef:          prepared.Ref(),
		CurrentRef:           current.Ref(),
		RouteCallDigest:      routeCallDigest,
		SourceLineage:        lineage,
		ProviderInjectionRef: exactOwnedRefV2(ownerV2("model", "provider-owner"), "model-provider", "provider", providerInjection),
		RouteRef:             exactOwnedRefV2(ownerV2("model", "route-owner"), "model-route", "route", routeDigest),
		ProfileRef:           exactOwnedRefV2(ownerV2("model", "profile-owner"), "model-profile", "profile", profileDigest),
		AuthorizedUnixNano:   now.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano:      now.Add(20 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	material, err := modelinvoker.SealInvocationMaterialV2(modelinvoker.InvocationMaterialV2{
		PreparedRef:          prepared.Ref(),
		UnifiedRequestDigest: prepared.UnifiedRequestDigest,
		PreparedPlanDigest:   prepared.PreparedPlanDigest,
		RouteDigest:          prepared.RouteDigest,
		ProfileDigest:        prepared.ProfileDigest,
		Authorization:        authorization,
		Call:                 call,
		CreatedUnixNano:      now.Add(-30 * time.Second).UnixNano(),
		ExpiresUnixNano:      now.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	command := modelinvoker.GovernedModelTurnCommandV3{
		PreparedRef:            prepared.Ref(),
		CurrentRef:             current.Ref(),
		MaterialRef:            material.RefV2(),
		AttemptRequestDigest:   prepared.UnifiedRequestDigest,
		RouteCallDigest:        material.RouteCallDigest,
		DispatchSequence:       sequence,
		ProviderAttemptOrdinal: 1,
	}
	envelope, err := bridgecontract.SealModelTurnExactEnvelopeV2(bridgecontract.ModelTurnExactEnvelopeV2{
		Material:                  material.RefV2(),
		Command:                   command,
		AckRef:                    ack.Ref(),
		RequestedNotAfterUnixNano: now.Add(8 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return exactModelTurnFixtureV2{
		prepared: prepared,
		current:  current,
		ack:      ack,
		material: material,
		envelope: envelope,
	}
}

func mustDigestRequestToolsV2(t *testing.T, call modelinvoker.RouteCall) core.Digest {
	t.Helper()
	digest, err := modelinvoker.DigestGovernedModelTurnRequestToolsV2(call)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustDigestContextV2(t *testing.T, call modelinvoker.RouteCall) core.Digest {
	t.Helper()
	digest, err := modelinvoker.DigestGovernedModelTurnContextV2(call)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func exactOwnedRefV2(owner core.OwnerRef, kind, id string, digest core.Digest) modelinvoker.InvocationMaterialExactSourceRefV1 {
	return modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner: owner, Kind: kind, ID: id, Revision: 1, Digest: digest,
	}
}

func ownerV2(domain, id string) core.OwnerRef {
	return core.OwnerRef{Domain: domain, ID: core.OwnerID(id)}
}

func digestV2(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}

func constantClockV2(now time.Time) func() time.Time {
	return func() time.Time { return now }
}

func clockSequenceV2(values ...time.Time) func() time.Time {
	var mu sync.Mutex
	index := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}
