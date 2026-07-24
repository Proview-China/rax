package fakes_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestControlledOperationProviderV2LostEnterReplyRecoversOnlyByExactInspect(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "lost-enter")
	fixture.entries.LoseNextControlledOperationProviderCreateReplyV2()

	result, err := fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ports.ControlledOperationProviderEnteredV2 || result.Error != ports.ControlledOperationProviderInspectionRequiredV2 {
		t.Fatalf("lost create reply did not return an inspect-only Entry: %#v", result)
	}
	if fixture.transport.calls.Load() != 0 {
		t.Fatal("lost create reply reconstructed the opaque claim and called Provider transport")
	}

	key := mustControlledOperationProviderEntryKeyV2(t, fixture.request)
	recovered, err := fixture.gateway.InspectControlledOperationProviderV2(context.Background(), ports.ControlledOperationProviderInspectRequestV2{Operation: fixture.request.Operation, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.EntryRef != result.EntryRef || recovered.Status != ports.ControlledOperationProviderEnteredV2 {
		t.Fatalf("exact Inspect did not recover the entered Entry: %#v", recovered)
	}
	if fixture.transport.calls.Load() != 0 || fixture.providerInspect.calls.Load() != 2 {
		t.Fatalf("Inspect crossed the Provider boundary: transport=%d inspect=%d", fixture.transport.calls.Load(), fixture.providerInspect.calls.Load())
	}

	badKeys := []ports.ControlledOperationProviderInspectKeyV2{
		{EntryID: "wrong-entry", StableKeyDigest: key.StableKeyDigest, ExpectedRequestDigest: key.ExpectedRequestDigest},
		{EntryID: key.EntryID, StableKeyDigest: key.StableKeyDigest, ExpectedRequestDigest: digestV3("wrong-request")},
	}
	for _, bad := range badKeys {
		before := fixture.providerInspect.calls.Load()
		if _, inspectErr := fixture.gateway.InspectControlledOperationProviderV2(context.Background(), ports.ControlledOperationProviderInspectRequestV2{Operation: fixture.request.Operation, Key: bad}); inspectErr == nil {
			t.Fatal("wrong exact Inspect key was accepted")
		}
		if fixture.providerInspect.calls.Load() != before {
			t.Fatal("wrong Inspect key reached Provider Inspect")
		}
	}

	wrongStable := digestV3("wrong-stable")
	wrongEntryID, err := ports.DeriveControlledOperationProviderEntryIDV2(wrongStable)
	if err != nil {
		t.Fatal(err)
	}
	before := fixture.providerInspect.calls.Load()
	_, err = fixture.gateway.InspectControlledOperationProviderV2(context.Background(), ports.ControlledOperationProviderInspectRequestV2{
		Operation: fixture.request.Operation,
		Key: ports.ControlledOperationProviderInspectKeyV2{
			EntryID:               wrongEntryID,
			StableKeyDigest:       wrongStable,
			ExpectedRequestDigest: key.ExpectedRequestDigest,
		},
	})
	if !core.HasCategory(err, core.ErrorNotFound) || fixture.providerInspect.calls.Load() != before {
		t.Fatalf("wrong stable key did not fail before Provider Inspect: err=%v", err)
	}
}

func TestControlledOperationProviderV2LostProviderReplyNeverReentersTransport(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "lost-provider")
	fixture.transport.loseReply.Store(true)

	result, err := fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ports.ControlledOperationProviderUnknownV2 || result.AdmissionReceipt != nil || result.Observation != nil || fixture.transport.calls.Load() != 1 || fixture.providerInspect.calls.Load() == 0 {
		t.Fatalf("lost Provider reply was not recovered through Inspect only: %#v calls=%d inspect=%d", result, fixture.transport.calls.Load(), fixture.providerInspect.calls.Load())
	}
	forgedReceipt, err := ports.SealControlledOperationProviderAdmissionReceiptRefV2(ports.ControlledOperationProviderAdmissionReceiptRefV2{ID: "forged-unknown-receipt", Revision: 1, StableKeyDigest: result.EntryRef.StableKeyDigest, Admitted: true})
	if err != nil {
		t.Fatal(err)
	}
	forged := result
	forged.AdmissionReceipt = &forgedReceipt
	forged.ResultDigest = ""
	forged.ResultDigest, err = forged.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if err := forged.Validate(); err == nil {
		t.Fatal("unknown result accepted a forged admission sidecar")
	}

	if _, err = fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	if fixture.transport.calls.Load() != 1 {
		t.Fatalf("replay re-entered Provider transport: %d", fixture.transport.calls.Load())
	}
}

func TestControlledOperationProviderV2CreateIdempotencyIgnoresFreshClosureAndClock(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "fresh-create")
	fixture.entries.LoseNextControlledOperationProviderCreateReplyV2()
	if _, err := fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	key := mustControlledOperationProviderEntryKeyV2(t, fixture.request)
	stored, err := fixture.entries.InspectControlledOperationProviderEntryV2(context.Background(), fixture.request.Operation, key.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	changed := stored
	changed.FreshBindings = append([]ports.ProviderBindingCurrentProjectionV2{}, stored.FreshBindings...)
	changed.EnteredUnixNano++
	changed.UpdatedUnixNano = changed.EnteredUnixNano
	changedBinding := changed.FreshBindings[0]
	changedBinding.IssuedUnixNano++
	changedBinding, err = ports.SealProviderBindingCurrentProjectionV2(changedBinding)
	if err != nil {
		t.Fatal(err)
	}
	changed.FreshBindings[0] = changedBinding
	changed, err = control.SealControlledOperationProviderEntryFactV2(changed)
	if err != nil {
		t.Fatal(err)
	}
	created, err := fixture.entries.CreateControlledOperationProviderEntryV2(context.Background(), changed)
	if err != nil {
		t.Fatal(err)
	}
	if created.Disposition != control.ControlledOperationProviderEntryExistingV2 || created.HasOpaqueClaimV2() || created.Fact.EntryID != stored.EntryID {
		t.Fatalf("same immutable request with refreshed closure/time was not idempotent: %#v", created)
	}
}

func TestControlledOperationProviderV2LostCreateRecoveryAcceptsProgressedSameEntry(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "lost-create-progressed")
	progressing := &controlledOperationProviderProgressOnCreateStoreV2{delegate: fixture.entries, now: fixture.now}
	fixture.gateway.Entries = progressing
	result, err := fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ports.ControlledOperationProviderUnknownV2 || result.AdmissionReceipt != nil || result.Observation != nil || fixture.transport.calls.Load() != 0 || fixture.providerInspect.calls.Load() == 0 {
		t.Fatalf("progressed lost-create recovery was not inspect-only: %#v transport=%d inspect=%d", result, fixture.transport.calls.Load(), fixture.providerInspect.calls.Load())
	}
}

func TestControlledOperationProviderV2CASRecoveryAcceptsObservedSuccessorOnly(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "cas-progressed")
	progressing := &controlledOperationProviderProgressOnCASStoreV2{delegate: fixture.entries, now: fixture.now}
	fixture.gateway.Entries = progressing
	result, err := fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ports.ControlledOperationProviderObservedV2 || result.Observation == nil || fixture.transport.calls.Load() != 1 || fixture.providerInspect.calls.Load() != 0 {
		t.Fatalf("CAS recovery did not accept the exact observed successor: %#v transport=%d inspect=%d", result, fixture.transport.calls.Load(), fixture.providerInspect.calls.Load())
	}

	key := mustControlledOperationProviderEntryKeyV2(t, fixture.request)
	observed, err := fixture.entries.InspectControlledOperationProviderEntryV2(context.Background(), fixture.request.Operation, key.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	expectedUnknown := observed
	expectedUnknown.Revision = 2
	expectedUnknown.State = control.ControlledOperationProviderEntryUnknownV2
	expectedUnknown.Observation = nil
	expectedUnknown.UpdatedUnixNano = observed.EnteredUnixNano
	expectedUnknown, err = control.SealControlledOperationProviderEntryFactV2(expectedUnknown)
	if err != nil {
		t.Fatal(err)
	}
	predecessor := expectedUnknown
	predecessor.Revision = 1
	predecessor.State = control.ControlledOperationProviderEntryEnteredV2
	predecessor.AdmissionReceipt = nil
	predecessor.UpdatedUnixNano = predecessor.EnteredUnixNano
	predecessor, err = control.SealControlledOperationProviderEntryFactV2(predecessor)
	if err != nil {
		t.Fatal(err)
	}
	if control.IsControlledOperationProviderEntryRecoverySuccessV2(expectedUnknown, predecessor) {
		t.Fatal("CAS recovery accepted a lower-revision predecessor")
	}
	sibling := observed
	sibling.State = control.ControlledOperationProviderEntryRejectedNoEffectV2
	sibling.Observation = nil
	sibling.AdmissionReceipt = nil
	if control.IsControlledOperationProviderEntryRecoverySuccessV2(observed, sibling) {
		t.Fatal("CAS recovery accepted a sibling terminal state")
	}
}

func TestControlledOperationProviderV2SevenBindingSetDriftFailsBeforeCreate(t *testing.T) {
	for index := 0; index < 7; index++ {
		for _, field := range []string{"set_digest", "semantic_digest"} {
			t.Run(field+"_role_"+string(rune('a'+index)), func(t *testing.T) {
				fixture := newControlledOperationProviderFixtureV2(t, "binding-drift-"+field+"-"+string(rune('a'+index)))
				ref := []ports.ProviderBindingRefV2{
					fixture.readers.route.ToolAdapterBinding,
					fixture.readers.route.GatewayBinding,
					fixture.readers.route.ProviderTransportBinding,
					fixture.readers.route.PreparedReaderBinding,
					fixture.readers.route.BoundaryReaderBinding,
					fixture.readers.route.ProviderInspectBinding,
					fixture.readers.route.ProviderBinding,
				}[index]
				current := fixture.readers.bindings[ref]
				if field == "set_digest" {
					current.BindingSetDigest = digestV3("wrong-set")
				} else {
					current.BindingSetSemanticDigest = digestV3("wrong-semantics")
				}
				var err error
				current, err = ports.SealProviderBindingCurrentProjectionV2(current)
				if err != nil {
					t.Fatal(err)
				}
				fixture.readers.bindings[ref] = current
				if _, err := fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request); !core.HasReason(err, core.ReasonBindingDrift) {
					t.Fatalf("Binding set drift reached Entry create: %v", err)
				}
				key := mustControlledOperationProviderEntryKeyV2(t, fixture.request)
				if _, err := fixture.entries.InspectControlledOperationProviderEntryV2(context.Background(), fixture.request.Operation, key.EntryID); !core.HasCategory(err, core.ErrorNotFound) {
					t.Fatalf("Binding drift wrote an Entry: %v", err)
				}
				if fixture.transport.calls.Load() != 0 {
					t.Fatal("Binding drift reached Provider transport")
				}
			})
		}
	}
}

func TestControlledOperationProviderV2Concurrent64LinearizesOneEntryClaim(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "concurrent")
	const workers = 64
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	results := make(chan ports.ControlledOperationProviderResultV2, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}
	wait.Wait()
	close(errors)
	close(results)
	for err := range errors {
		t.Fatal(err)
	}
	var entry ports.ControlledOperationProviderEntryRefV2
	count := 0
	for result := range results {
		count++
		if entry.EntryID == "" {
			entry = result.EntryRef
		} else if result.EntryRef.EntryID != entry.EntryID || result.EntryRef.StableKeyDigest != entry.StableKeyDigest {
			t.Fatalf("concurrent calls returned different logical Entries: %#v %#v", entry, result.EntryRef)
		}
	}
	if count != workers || fixture.transport.calls.Load() != 1 || fixture.transport.logicalAdmissions.Load() != 1 {
		t.Fatalf("concurrent calls did not linearize one logical admission: results=%d calls=%d logical=%d", count, fixture.transport.calls.Load(), fixture.transport.logicalAdmissions.Load())
	}
}

func TestControlledOperationProviderV2InspectRejectsMaliciousStoreFactsBeforeProviderInspect(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "malicious-store")
	fixture.entries.LoseNextControlledOperationProviderCreateReplyV2()
	if _, err := fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	key := mustControlledOperationProviderEntryKeyV2(t, fixture.request)
	valid, err := fixture.entries.InspectControlledOperationProviderEntryV2(context.Background(), fixture.request.Operation, key.EntryID)
	if err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		name   string
		mutate func(*control.ControlledOperationProviderEntryFactV2)
	}{
		{name: "wrong_entry_id", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) { fact.EntryID = "another-entry" }},
		{name: "operation", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) {
			fact.Request.Operation.CurrentProjectionRevision++
		}},
		{name: "prepared", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) {
			fact.Request.Prepared.AttemptID = "another-attempt"
		}},
		{name: "attempt", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) {
			fact.Request.Attempt.AttemptID = "another-attempt"
		}},
		{name: "malformed_digest", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) { fact.Digest = "not-a-digest" }},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			forged := valid
			testCase.mutate(&forged)
			inspect := &controlledOperationProviderInspectStubV2{}
			gateway := kernel.NewControlledOperationProviderGatewayV2(
				controlledOperationProviderMaliciousStoreV2{fact: forged},
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				inspect, nil, func() time.Time { return fixture.now },
			)
			if _, err := gateway.InspectControlledOperationProviderV2(context.Background(), ports.ControlledOperationProviderInspectRequestV2{Operation: fixture.request.Operation, Key: key}); err == nil {
				t.Fatal("malicious Store fact was accepted")
			}
			if inspect.calls.Load() != 0 {
				t.Fatal("malicious Store fact reached Provider Inspect")
			}
		})
	}
}

func TestControlledOperationPhysicalAuthorizationV3IssuesExactCurrentClosureWithoutProviderCall(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "physical-authorization")
	request, association := controlledOperationPhysicalAuthorizationFixtureV3(t, fixture)
	reader := &preparedDomainCommandAssociationReaderStubV1{projection: association}
	gateway, err := kernel.NewControlledOperationPhysicalAuthorizationGatewayV3(&fixture.gateway, reader)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := gateway.AuthorizeControlledOperationPhysicalV3(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorization.ValidateCurrent(fixture.now); err != nil {
		t.Fatal(err)
	}
	if authorization.Association != association.Ref || authorization.DomainCommand != association.DomainCommand || authorization.ProviderTransport != fixture.readers.route.ProviderTransportBinding || authorization.UnifiedNotAfterUnixNano > association.ExpiresUnixNano {
		t.Fatal("physical authorization did not preserve the exact closure")
	}
	if fixture.transport.calls.Load() != 0 || fixture.providerInspect.calls.Load() != 0 {
		t.Fatal("authorization issuance reached Provider transport or inspect")
	}
}

func TestControlledOperationPhysicalAuthorizationV3FailsClosedOnDriftClockAndTypedNil(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "physical-authorization-negative")
	request, association := controlledOperationPhysicalAuthorizationFixtureV3(t, fixture)

	t.Run("association_attempt_drift", func(t *testing.T) {
		changed := association
		changed.Attempt.AttemptID = "another-attempt"
		changed.Ref.Digest, changed.ProjectionDigest = "", ""
		changed, err := ports.SealPreparedDomainCommandAssociationCurrentProjectionV1(changed)
		if err == nil {
			t.Fatal("drifted association unexpectedly resealed")
		}
	})

	t.Run("reader_returns_other_command", func(t *testing.T) {
		changed := association
		changed.DomainCommand.ID = "another-command"
		changed.Ref = ports.PreparedDomainCommandAssociationRefV1{}
		changed.ProjectionDigest = ""
		changed, err := ports.SealPreparedDomainCommandAssociationCurrentProjectionV1(changed)
		if err != nil {
			t.Fatal(err)
		}
		gateway, err := kernel.NewControlledOperationPhysicalAuthorizationGatewayV3(&fixture.gateway, &preparedDomainCommandAssociationReaderStubV1{projection: changed})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = gateway.AuthorizeControlledOperationPhysicalV3(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("other command error=%v", err)
		}
	})

	t.Run("clock_rollback", func(t *testing.T) {
		values := []time.Time{fixture.now, fixture.now.Add(-time.Nanosecond)}
		index := 0
		base := fixture.gateway
		baseClock := func() time.Time {
			value := values[index]
			if index+1 < len(values) {
				index++
			}
			return value
		}
		base = kernel.NewControlledOperationProviderGatewayV2(base.Entries, base.Routes, fixture.readers, base.Bindings, base.Effects, base.Prepared, base.Policies, base.Enforcement, base.Handoff, base.Evidence, base.Boundary, base.ProviderInspect, fixture.transport, baseClock)
		gateway, err := kernel.NewControlledOperationPhysicalAuthorizationGatewayV3(&base, &preparedDomainCommandAssociationReaderStubV1{projection: association})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = gateway.AuthorizeControlledOperationPhysicalV3(context.Background(), request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback error=%v", err)
		}
	})

	t.Run("nil_context", func(t *testing.T) {
		gateway, err := kernel.NewControlledOperationPhysicalAuthorizationGatewayV3(&fixture.gateway, &preparedDomainCommandAssociationReaderStubV1{projection: association})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = gateway.AuthorizeControlledOperationPhysicalV3(nil, request); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})

	t.Run("typed_nil_reader", func(t *testing.T) {
		var reader *preparedDomainCommandAssociationReaderStubV1
		if _, err := kernel.NewControlledOperationPhysicalAuthorizationGatewayV3(&fixture.gateway, reader); !core.HasReason(err, core.ReasonComponentMissing) {
			t.Fatalf("typed nil reader error=%v", err)
		}
	})

	if fixture.transport.calls.Load() != 0 || fixture.providerInspect.calls.Load() != 0 {
		t.Fatal("fail-closed authorization reached Provider")
	}
}

func TestPreparedDomainCommandAssociationGatewayV1CreateOnceLostReplyAndConcurrency(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "association-gateway")
	_, association := controlledOperationPhysicalAuthorizationFixtureV3(t, fixture)
	request := ensureAssociationRequestV1(association)
	store := fakes.NewInMemoryPreparedDomainCommandAssociationStoreV1()
	var ticks atomic.Int64
	clock := func() time.Time { return fixture.now.Add(time.Duration(ticks.Add(1)) * time.Nanosecond) }
	gateway, err := kernel.NewPreparedDomainCommandAssociationGatewayV1(store, clock)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	results := make(chan ports.PreparedDomainCommandAssociationCurrentProjectionV1, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, callErr := gateway.EnsurePreparedDomainCommandAssociationV1(context.Background(), request)
			if callErr != nil {
				errors <- callErr
				return
			}
			results <- value
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for callErr := range errors {
		t.Fatal(callErr)
	}
	var winner ports.PreparedDomainCommandAssociationCurrentProjectionV1
	count := 0
	for result := range results {
		if count == 0 {
			winner = result
		} else if result.Ref != winner.Ref || result.CheckedUnixNano != winner.CheckedUnixNano || result.ExpiresUnixNano != winner.ExpiresUnixNano {
			t.Fatal("same canonical association did not return the persisted winner")
		}
		count++
	}
	if count != workers {
		t.Fatalf("association results=%d want=%d", count, workers)
	}
	inspected, err := gateway.InspectCurrentPreparedDomainCommandAssociationV1(context.Background(), winner.Ref)
	if err != nil || inspected.Ref != winner.Ref {
		t.Fatalf("exact inspect value=%v err=%v", inspected.Ref, err)
	}

	lostStore := &lostReplyPreparedDomainCommandAssociationStoreV1{delegate: fakes.NewInMemoryPreparedDomainCommandAssociationStoreV1()}
	lostGateway, err := kernel.NewPreparedDomainCommandAssociationGatewayV1(lostStore, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := lostGateway.EnsurePreparedDomainCommandAssociationV1(context.Background(), request)
	if err != nil || recovered.Ref.Validate() != nil || lostStore.creates.Load() != 1 {
		t.Fatalf("lost create recovery value=%v creates=%d err=%v", recovered.Ref, lostStore.creates.Load(), err)
	}
}

func TestPreparedDomainCommandAssociationGatewayV1FailClosed(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "association-gateway-negative")
	_, association := controlledOperationPhysicalAuthorizationFixtureV3(t, fixture)
	request := ensureAssociationRequestV1(association)
	store := fakes.NewInMemoryPreparedDomainCommandAssociationStoreV1()
	gateway, err := kernel.NewPreparedDomainCommandAssociationGatewayV1(store, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}

	changed := request
	changed.DomainCommand.Owner.ComponentID = "praxis.tool/other-provider"
	if _, err = gateway.EnsurePreparedDomainCommandAssociationV1(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("wrong command Owner error=%v", err)
	}
	expired := request
	expired.RequestedNotAfterNano = fixture.now.UnixNano()
	if _, err = gateway.EnsurePreparedDomainCommandAssociationV1(context.Background(), expired); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expired association error=%v", err)
	}
	if _, err = gateway.EnsurePreparedDomainCommandAssociationV1(nil, request); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	var typedNil *fakes.InMemoryPreparedDomainCommandAssociationStoreV1
	if _, err = kernel.NewPreparedDomainCommandAssociationGatewayV1(typedNil, time.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil store error=%v", err)
	}
	if _, err = store.InspectPreparedDomainCommandAssociationByIDV1(context.Background(), association.Ref.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("fail-closed paths wrote association: %v", err)
	}
}

func TestControlledOperationProviderV2InspectRejectsResealedClosureAndTimeDrift(t *testing.T) {
	fixture := newControlledOperationProviderFixtureV2(t, "malicious-closure")
	fixture.entries.LoseNextControlledOperationProviderCreateReplyV2()
	if _, err := fixture.gateway.EnterControlledOperationProviderV2(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	key := mustControlledOperationProviderEntryKeyV2(t, fixture.request)
	valid, err := fixture.entries.InspectControlledOperationProviderEntryV2(context.Background(), fixture.request.Operation, key.EntryID)
	if err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		name   string
		mutate func(*control.ControlledOperationProviderEntryFactV2) error
	}{
		{name: "expired_at_entry", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) error {
			fact.UnifiedNotAfterUnixNano = fact.EnteredUnixNano
			return nil
		}},
		{name: "future_effect_checked", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) error {
			value := fact.FreshEffect
			value.CheckedUnixNano = fact.EnteredUnixNano + 1
			sealed, err := ports.SealControlledOperationEffectCurrentProjectionV2(value)
			fact.FreshEffect = sealed
			return err
		}},
		{name: "future_route_checked", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) error {
			value := fact.FreshRoute
			value.CheckedUnixNano = fact.EnteredUnixNano + 1
			value.ProjectionDigest = ""
			sealed, err := ports.SealControlledOperationProviderRouteCurrentProjectionV2(value)
			fact.FreshRoute = sealed
			return err
		}},
		{name: "future_prepared_checked", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) error {
			value := fact.FreshPrepared
			value.CheckedUnixNano = fact.EnteredUnixNano + 1
			sealed, err := ports.SealControlledOperationPreparedCurrentProjectionV2(value)
			fact.FreshPrepared = sealed
			return err
		}},
		{name: "future_boundary_checked", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) error {
			value := fact.FreshBoundary
			value.CheckedUnixNano = fact.EnteredUnixNano + 1
			sealed, err := ports.SealOperationProviderBoundaryCurrentProjectionV1(value)
			fact.FreshBoundary = sealed
			return err
		}},
		{name: "binding_role_swap", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) error {
			fact.FreshBindings = append([]ports.ProviderBindingCurrentProjectionV2{}, fact.FreshBindings...)
			fact.FreshBindings[0], fact.FreshBindings[1] = fact.FreshBindings[1], fact.FreshBindings[0]
			return nil
		}},
		{name: "another_binding_set", mutate: func(fact *control.ControlledOperationProviderEntryFactV2) error {
			fact.FreshBindings = append([]ports.ProviderBindingCurrentProjectionV2{}, fact.FreshBindings...)
			for index := range fact.FreshBindings {
				value := fact.FreshBindings[index]
				value.BindingSetDigest = digestV3("another-binding-set")
				value.BindingSetSemanticDigest = digestV3("another-binding-semantics")
				sealed, err := ports.SealProviderBindingCurrentProjectionV2(value)
				if err != nil {
					return err
				}
				fact.FreshBindings[index] = sealed
			}
			return nil
		}},
	}
	for index := 0; index < 7; index++ {
		bindingIndex := index
		mutations = append(mutations, struct {
			name   string
			mutate func(*control.ControlledOperationProviderEntryFactV2) error
		}{name: "future_binding_issued_" + string(rune('a'+index)), mutate: func(fact *control.ControlledOperationProviderEntryFactV2) error {
			fact.FreshBindings = append([]ports.ProviderBindingCurrentProjectionV2{}, fact.FreshBindings...)
			value := fact.FreshBindings[bindingIndex]
			value.IssuedUnixNano = fact.EnteredUnixNano + 1
			sealed, err := ports.SealProviderBindingCurrentProjectionV2(value)
			fact.FreshBindings[bindingIndex] = sealed
			return err
		}})
	}

	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			forged := valid
			if err := testCase.mutate(&forged); err != nil {
				t.Fatal(err)
			}
			forged.Digest = ""
			forged.Digest, err = forged.DigestV2()
			if err != nil {
				t.Fatal(err)
			}
			inspect := &controlledOperationProviderInspectStubV2{}
			gateway := kernel.NewControlledOperationProviderGatewayV2(
				controlledOperationProviderMaliciousStoreV2{fact: forged},
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				inspect, nil, func() time.Time { return fixture.now },
			)
			if _, err := gateway.InspectControlledOperationProviderV2(context.Background(), ports.ControlledOperationProviderInspectRequestV2{Operation: fixture.request.Operation, Key: key}); err == nil {
				t.Fatal("re-sealed closure/time drift was accepted")
			}
			if inspect.calls.Load() != 0 {
				t.Fatal("re-sealed closure/time drift reached Provider Inspect")
			}
		})
	}
}

type controlledOperationProviderFixtureV2 struct {
	now             time.Time
	entries         *fakes.ControlledOperationProviderEntryStoreV2
	request         ports.ControlledOperationProviderRequestV2
	gateway         kernel.ControlledOperationProviderGatewayV2
	transport       *controlledOperationProviderTransportStubV2
	providerInspect *controlledOperationProviderInspectStubV2
	readers         *controlledOperationProviderReadersV2
}

func newControlledOperationProviderFixtureV2(t *testing.T, suffix string) *controlledOperationProviderFixtureV2 {
	t.Helper()
	enforcement := newOperationEnforcementFixtureForScopeV4(t, "cop2-"+suffix, "run-cop2", "tenant-cop2", ports.OperationScopeEvidenceActionEffectKindV3)
	now := enforcement.effect.now
	preparedPhase, err := enforcement.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), enforcement.prepare)
	if err != nil {
		t.Fatal(err)
	}
	preparedRef := preparedAttemptForEnforcementV4(t, enforcement, preparedPhase)
	executeRequest := enforcement.prepare
	executeRequest.Phase = ports.OperationDispatchEnforcementExecuteV4
	executeRequest.ExpectedJournalRevision = 1
	executeRequest.Prepare = &preparedPhase.Phase
	executeRequest.PreparedAttempt = preparedRef
	executed, err := enforcement.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), executeRequest)
	if err != nil {
		t.Fatal(err)
	}

	effectFact, err := enforcement.effect.store.InspectOperationEffectV3(context.Background(), enforcement.effect.intent.Operation, enforcement.effect.intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	intentDigest, err := effectFact.Intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: executed.Phase.OperationDigest,
		EffectID:        executed.Phase.EffectID,
		IntentRevision:  preparedRef.IntentRevision,
		IntentDigest:    preparedRef.IntentDigest,
		PermitID:        executed.Phase.PermitID,
		PermitRevision:  executed.Phase.PermitFactRevision,
		PermitDigest:    executed.Phase.PermitDigest,
		AttemptID:       executed.Phase.AttemptID,
	}
	preparedAttempt := attempt
	preparedAttempt.PermitRevision = preparedRef.PermitRevision
	preparedAttempt.PermitDigest = preparedRef.PermitDigest
	delegation := ports.ExecutionDelegationRefV2{ID: preparedRef.DeclaredDelegation.ID, Revision: preparedRef.DeclaredDelegation.Revision + 1, Digest: digestV3("prepared-delegation-" + suffix)}
	persisted := ports.PersistedOperationEnforcementRefV3{
		PermitID:         executed.Phase.PermitID,
		PermitRevision:   executed.Phase.PermitFactRevision,
		PermitDigest:     executed.Phase.PermitDigest,
		AttemptID:        attempt.AttemptID,
		OperationDigest:  attempt.OperationDigest,
		Provider:         preparedRef.Provider,
		ReceiptDigest:    digestV3("persisted-enforcement-" + suffix),
		RecordedRevision: 1,
	}
	preparedSemantics, err := ports.SealControlledOperationPreparedSemanticSnapshotV2(ports.ControlledOperationPreparedSemanticSnapshotV2{
		Prepared:             *preparedRef,
		Delegation:           delegation,
		PersistedEnforcement: persisted,
		OperationDigest:      attempt.OperationDigest,
		EffectID:             attempt.EffectID,
		IntentRevision:       attempt.IntentRevision,
		IntentDigest:         attempt.IntentDigest,
		Attempt:              preparedAttempt,
		ProviderBinding:      preparedRef.Provider,
		PayloadSchema:        preparedRef.PayloadSchema,
		PayloadDigest:        preparedRef.PayloadDigest,
		PayloadRevision:      preparedRef.PayloadRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	preparedCurrent, err := ports.SealControlledOperationPreparedCurrentProjectionV2(ports.ControlledOperationPreparedCurrentProjectionV2{
		Snapshot: preparedSemantics, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	applicability := controlledOperationProviderApplicabilityV2(suffix)
	appPolicy, err := ports.SealOperationScopeEvidenceApplicabilityPolicyFactV3(ports.OperationScopeEvidenceApplicabilityPolicyFactV3{
		ID: "cop2-app-policy-" + suffix, Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3,
		OperationKind: ports.OperationScopeRunV3, EffectKind: ports.OperationScopeEvidenceActionEffectKindV3,
		Profile: ports.OperationScopeEvidenceActionPolicyProfileV3, ExecutionScopeDigest: effectFact.Intent.Operation.ExecutionScopeDigest,
		Applicability: applicability, ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	schema := ports.SchemaRefV2{Namespace: "praxis.tool", Name: "provider-observation", Version: "1.0.0", MediaType: "application/json", ContentDigest: digestV3("cop2-schema")}
	evidencePolicy, err := ports.SealOperationScopeEvidencePolicyFactV3(ports.OperationScopeEvidencePolicyFactV3{
		ID: "cop2-evidence-policy-" + suffix, Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3,
		OperationKind: ports.OperationScopeRunV3, EffectKind: ports.OperationScopeEvidenceActionEffectKindV3,
		AllowedPhases:  []ports.OperationDispatchEnforcementPhaseV4{ports.OperationDispatchEnforcementExecuteV4},
		ExpectedSchema: schema, MaximumPayloadBytes: 1024, MaximumQualificationTTL: 5 * time.Second,
		MaximumIngestGrace: time.Second, ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	generation := ports.GenerationBindingAssociationRefV1{ID: "cop2-generation-" + suffix, Revision: 1, Digest: digestV3("cop2-generation-" + suffix)}
	scope := ports.OperationScopeEvidenceScopeV3{
		LedgerScope: ports.OperationScopeEvidenceLedgerScopeV3{TenantID: effectFact.Intent.Operation.ExecutionScope.Identity.TenantID, OperationDigest: attempt.OperationDigest, ChainID: "cop2-chain-" + suffix},
		Operation:   effectFact.Intent.Operation, OperationDigest: attempt.OperationDigest,
		EffectID: effectFact.Intent.ID, EffectRevision: effectFact.Revision, EffectDigest: intentDigest,
		EffectKind: effectFact.Intent.Kind, AttemptID: attempt.AttemptID, Phase: ports.OperationDispatchEnforcementExecuteV4,
		ApplicabilityPolicy: appPolicy.RefV3(), Applicability: applicability, Generation: generation,
	}
	runtimeCurrent, err := ports.SealOperationScopeEvidenceRuntimeCurrentProjectionV3(ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{
		Scope: scope, PermitID: executed.Phase.PermitID, PermitFactRevision: executed.Phase.PermitFactRevision, PermitDigest: executed.Phase.PermitDigest,
		AdmissionDigest: executed.Phase.AdmissionDigest, Authorization: executed.Phase.ReviewAuthorization, Phase: executed.Phase,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	registration := ports.OperationScopeEvidenceFactRefV3{ID: "cop2-registration-" + suffix, Revision: 1, Digest: digestV3("cop2-registration-" + suffix), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}
	qualification, err := ports.SealOperationScopeEvidenceQualificationFactV3(ports.OperationScopeEvidenceQualificationFactV3{
		ID: "cop2-qualification-" + suffix, Revision: 1, State: ports.OperationScopeEvidenceIssuedV3,
		Scope: scope, Runtime: runtimeCurrent, EvidencePolicy: evidencePolicy.RefV3(),
		Reservation: ports.OperationScopeEvidenceSourceReservationV3{
			Registration: registration,
			Source:       ports.OperationScopeEvidenceSourceKeyV3{RegistrationID: registration.ID, SourceEpoch: 1, SourceSequence: 1},
			EventID:      "cop2-event-" + suffix, Schema: schema,
		},
		RequestedTTL: 4 * time.Second, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano: now.Add(4 * time.Second).UnixNano(), IngestNotAfterUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := ports.SealOperationScopeEvidenceProviderHandoffFactV3(ports.OperationScopeEvidenceProviderHandoffFactV3{
		ID: "cop2-handoff-" + suffix, Revision: 1, Qualification: qualification.RefV3(), Phase: executed.Phase,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(4 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := ports.SealOperationProviderBoundaryCurrentProjectionV1(ports.OperationProviderBoundaryCurrentProjectionV1{
		Ref:       ports.OperationProviderBoundaryRefV1{ID: "cop2-boundary-" + suffix, Revision: 1, Digest: digestV3("cop2-boundary-" + suffix)},
		Operation: effectFact.Intent.Operation, OperationDigest: attempt.OperationDigest, OperationScopeDigest: effectFact.Intent.Operation.ExecutionScopeDigest,
		Attempt: attempt, ExecuteEnforcement: executed.Phase, ExecuteEvidenceHandoff: handoff.RefV3(),
		Stage: ports.OperationProviderBoundaryCrossedV1, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(4 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	bindings, route := controlledOperationProviderRouteV2(t, suffix, now, effectFact.Intent.Provider)
	effectCurrent, err := ports.SealControlledOperationEffectCurrentProjectionV2(ports.ControlledOperationEffectCurrentProjectionV2{
		Intent: effectFact.Intent, IntentDigest: intentDigest, FactRevision: effectFact.Revision,
		State: string(effectFact.State), CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := ports.SealControlledOperationProviderRequestV2(ports.ControlledOperationProviderRequestV2{
		RouteDeclarationRef: route.DeclarationRef, RouteConformanceRef: route.ConformanceRef, RouteCurrentRef: route.Ref,
		ToolAdapterBinding: route.ToolAdapterBinding, Operation: effectFact.Intent.Operation, OperationDigest: attempt.OperationDigest,
		OperationScopeDigest: effectFact.Intent.Operation.ExecutionScopeDigest, EffectID: effectFact.Intent.ID, EffectRevision: effectFact.Revision,
		EffectKind: effectFact.Intent.Kind, IntentDigest: intentDigest, Attempt: attempt, ProviderBinding: effectFact.Intent.Provider,
		Prepared: *preparedRef, PreparedSemantics: preparedSemantics, ExecuteEnforcement: executed.Phase,
		ExecuteEvidenceHandoff: handoff.RefV3(), Boundary: boundary.Ref, EvidencePolicy: evidencePolicy.RefV3(),
		ApplicabilityPolicy: appPolicy.RefV3(), CallerDeadlineUnixNano: now.Add(4 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	entries := fakes.NewControlledOperationProviderEntryStoreV2(func() time.Time { return now })
	transport := &controlledOperationProviderTransportStubV2{}
	providerInspect := &controlledOperationProviderInspectStubV2{}
	readers := &controlledOperationProviderReadersV2{
		route: route, bindings: bindings, effect: effectCurrent, prepared: preparedCurrent,
		evidencePolicy: evidencePolicy, applicabilityPolicy: appPolicy, enforcement: executed.Phase,
		handoff: handoff, qualification: qualification, boundary: boundary,
	}
	gateway := kernel.NewControlledOperationProviderGatewayV2(
		entries, readers, readers, readers, readers, readers, readers, readers, readers, readers, readers, providerInspect, transport, func() time.Time { return now },
	)
	return &controlledOperationProviderFixtureV2{now: now, entries: entries, request: request, gateway: gateway, transport: transport, providerInspect: providerInspect, readers: readers}
}

func controlledOperationProviderApplicabilityV2(suffix string) []ports.OperationScopeEvidenceApplicabilityV3 {
	values := make([]ports.OperationScopeEvidenceApplicabilityV3, 0, 5)
	for _, route := range ports.OperationScopeEvidenceActionRoutesV3() {
		ref := ports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: route.Kind, ID: "cop2-" + string(route.Dimension) + "-" + suffix, Revision: 1, Digest: digestV3("cop2-" + string(route.Dimension) + "-" + suffix)}
		values = append(values, ports.OperationScopeEvidenceApplicabilityV3{Dimension: route.Dimension, Mode: ports.OperationScopeEvidenceRequiredV3, Fact: &ref})
	}
	return ports.NormalizeOperationScopeEvidenceApplicabilityV3(values)
}

func controlledOperationProviderRouteV2(t *testing.T, suffix string, now time.Time, provider ports.ProviderBindingRefV2) (map[ports.ProviderBindingRefV2]ports.ProviderBindingCurrentProjectionV2, ports.ControlledOperationProviderRouteCurrentProjectionV2) {
	t.Helper()
	declaration := ports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "cop2-route-" + suffix, Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: digestV3("cop2-declaration-" + suffix)}
	conformance := ports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "cop2-conformance-" + suffix, Revision: 1, DeclarationRef: declaration, ConformanceDigest: digestV3("cop2-conformance-" + suffix)}
	binding := func(component ports.ComponentIDV2, capability ports.CapabilityNameV2) ports.ProviderBindingRefV2 {
		return ports.ProviderBindingRefV2{
			BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision,
			ComponentID: component, ManifestDigest: digestV3("manifest-" + string(component)), ArtifactDigest: digestV3("artifact-" + string(component)), Capability: capability,
		}
	}
	refs := []ports.ProviderBindingRefV2{
		binding("praxis.tool/adapter", ports.ControlledOperationToolAdapterCapabilityV2),
		binding("praxis.runtime/gateway", ports.ControlledOperationGatewayCapabilityV2),
		binding("praxis.tool/transport", ports.ControlledOperationProviderTransportCapabilityV2),
		binding("praxis.runtime/prepared-reader", ports.ControlledOperationPreparedReaderCapabilityV2),
		binding("praxis.runtime/boundary-reader", ports.ControlledOperationBoundaryReaderCapabilityV2),
		binding("praxis.runtime/provider-inspect", ports.ControlledOperationProviderInspectCapabilityV2),
		provider,
	}
	setDigest := digestV3("cop2-binding-set-" + suffix)
	semanticDigest := digestV3("cop2-binding-semantic-" + suffix)
	currents := make(map[ports.ProviderBindingRefV2]ports.ProviderBindingCurrentProjectionV2, len(refs))
	for index, ref := range refs {
		projection, err := ports.SealProviderBindingCurrentProjectionV2(ports.ProviderBindingCurrentProjectionV2{
			ContractVersion: ports.ProviderBindingCurrentnessContractVersionV2, Ref: ref, State: ports.ProviderBindingCurrentActiveV2,
			BindingSetDigest: setDigest, BindingSetSemanticDigest: semanticDigest,
			BindingID: "cop2-binding-" + string(rune('a'+index)) + "-" + suffix, BindingRevision: 1,
			GrantDigest:    digestV3("cop2-grant-" + string(rune('a'+index)) + "-" + suffix),
			IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(),
		})
		if err != nil {
			t.Fatal(err)
		}
		currents[ref] = projection
	}
	route, err := ports.SealControlledOperationProviderRouteCurrentProjectionV2(ports.ControlledOperationProviderRouteCurrentProjectionV2{
		Ref: ports.ControlledOperationProviderRouteCurrentRefV2{Revision: 1}, DeclarationRef: declaration, ConformanceRef: conformance,
		Generation: ports.GenerationArtifactRefV1{ID: "cop2-generation-artifact-" + suffix, Revision: 1, Digest: digestV3("generation-artifact-" + suffix), InputDigest: digestV3("generation-input-" + suffix), ManifestDigest: digestV3("generation-manifest-" + suffix), GraphDigest: digestV3("generation-graph-" + suffix), CatalogDigest: digestV3("generation-catalog-" + suffix)},
		HandoffID:  "cop2-assembly-handoff-" + suffix, HandoffRevision: 1, HandoffDigest: digestV3("assembly-handoff-" + suffix),
		BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, BindingSetDigest: setDigest,
		BindingSetSemanticDigest: semanticDigest, BindingSetCurrentnessDigest: digestV3("binding-currentness-" + suffix),
		ActiveRouteID: "cop2-active-route-" + suffix, ActiveRouteRevision: 1, ActiveRouteDigest: digestV3("active-route-" + suffix),
		ToolAdapterBinding: refs[0], GatewayBinding: refs[1], ProviderTransportBinding: refs[2], PreparedReaderBinding: refs[3],
		BoundaryReaderBinding: refs[4], ProviderInspectBinding: refs[5], ProviderBinding: refs[6],
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return currents, route
}

type controlledOperationProviderReadersV2 struct {
	route               ports.ControlledOperationProviderRouteCurrentProjectionV2
	bindings            map[ports.ProviderBindingRefV2]ports.ProviderBindingCurrentProjectionV2
	effect              ports.ControlledOperationEffectCurrentProjectionV2
	prepared            ports.ControlledOperationPreparedCurrentProjectionV2
	evidencePolicy      ports.OperationScopeEvidencePolicyFactV3
	applicabilityPolicy ports.OperationScopeEvidenceApplicabilityPolicyFactV3
	enforcement         ports.OperationDispatchEnforcementPhaseRefV4
	handoff             ports.OperationScopeEvidenceProviderHandoffFactV3
	qualification       ports.OperationScopeEvidenceQualificationFactV3
	boundary            ports.OperationProviderBoundaryCurrentProjectionV1
}

type preparedDomainCommandAssociationReaderStubV1 struct {
	projection ports.PreparedDomainCommandAssociationCurrentProjectionV1
}

type lostReplyPreparedDomainCommandAssociationStoreV1 struct {
	delegate *fakes.InMemoryPreparedDomainCommandAssociationStoreV1
	creates  atomic.Int64
}

func (s *lostReplyPreparedDomainCommandAssociationStoreV1) CreatePreparedDomainCommandAssociationV1(ctx context.Context, projection ports.PreparedDomainCommandAssociationCurrentProjectionV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	s.creates.Add(1)
	if _, err := s.delegate.CreatePreparedDomainCommandAssociationV1(ctx, projection); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "injected association create reply loss")
}

func (s *lostReplyPreparedDomainCommandAssociationStoreV1) InspectPreparedDomainCommandAssociationByIDV1(ctx context.Context, id string) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	return s.delegate.InspectPreparedDomainCommandAssociationByIDV1(ctx, id)
}

func (s *lostReplyPreparedDomainCommandAssociationStoreV1) InspectPreparedDomainCommandAssociationV1(ctx context.Context, exact ports.PreparedDomainCommandAssociationRefV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	return s.delegate.InspectPreparedDomainCommandAssociationV1(ctx, exact)
}

func ensureAssociationRequestV1(projection ports.PreparedDomainCommandAssociationCurrentProjectionV1) ports.EnsurePreparedDomainCommandAssociationRequestV1 {
	return ports.EnsurePreparedDomainCommandAssociationRequestV1{
		Operation: projection.Operation, OperationDigest: projection.OperationDigest,
		EffectID: projection.EffectID, IntentRevision: projection.EffectRevision, IntentDigest: projection.IntentDigest,
		Prepared: projection.Prepared, Attempt: projection.Attempt, Provider: projection.Provider,
		PayloadSchema: projection.PayloadSchema, PayloadDigest: projection.PayloadDigest, PayloadRevision: projection.PayloadRevision,
		DomainCommand: projection.DomainCommand, RequestedNotAfterNano: projection.ExpiresUnixNano,
	}
}

func (r *preparedDomainCommandAssociationReaderStubV1) InspectCurrentPreparedDomainCommandAssociationV1(context.Context, ports.PreparedDomainCommandAssociationRefV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	return r.projection, nil
}

func controlledOperationPhysicalAuthorizationFixtureV3(t *testing.T, fixture *controlledOperationProviderFixtureV2) (ports.ControlledOperationPhysicalAuthorizationRequestV3, ports.PreparedDomainCommandAssociationCurrentProjectionV1) {
	t.Helper()
	provider := fixture.request.ProviderBinding
	command := ports.OperationDomainCommandRefV1{
		Owner: ports.EffectOwnerRefV2{Role: ports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		Kind:  "praxis.mcp/execution-command", ID: "mcp-command-" + fixture.request.Attempt.AttemptID, Revision: 1, Digest: digestV3("mcp-command-" + fixture.request.Attempt.AttemptID),
	}
	association, err := ports.SealPreparedDomainCommandAssociationCurrentProjectionV1(ports.PreparedDomainCommandAssociationCurrentProjectionV1{
		Operation: fixture.request.Operation, OperationDigest: fixture.request.OperationDigest,
		EffectID: fixture.request.EffectID, EffectRevision: fixture.request.Attempt.IntentRevision, IntentDigest: fixture.request.IntentDigest,
		Prepared: fixture.request.Prepared, Attempt: fixture.request.Attempt, Provider: provider,
		PayloadSchema: fixture.request.Prepared.PayloadSchema, PayloadDigest: fixture.request.Prepared.PayloadDigest, PayloadRevision: fixture.request.Prepared.PayloadRevision,
		DomainCommand: command, CheckedUnixNano: fixture.now.Add(-time.Second).UnixNano(), ExpiresUnixNano: fixture.now.Add(3 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := ports.ControlledOperationPhysicalAuthorizationRequestV3{Provider: fixture.request, Association: association.Ref, DomainCommand: command}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return request, association
}

func (r *controlledOperationProviderReadersV2) InspectCurrentControlledOperationProviderRouteV2(context.Context, ports.ControlledOperationProviderRouteCurrentRefV2, ports.OperationScopeEvidenceApplicabilityMatrixKeyV3) (ports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	return r.route, nil
}

func (r *controlledOperationProviderReadersV2) InspectCurrentControlledOperationProviderRouteInputsV2(context.Context, ports.ControlledOperationProviderRouteCurrentProjectionV2) (ports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	return r.route, nil
}

func (r *controlledOperationProviderReadersV2) InspectProviderBindingCurrentV2(_ context.Context, ref ports.ProviderBindingRefV2) (ports.ProviderBindingCurrentProjectionV2, error) {
	projection, ok := r.bindings[ref]
	if !ok {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "binding current projection not found")
	}
	return projection, nil
}

func (r *controlledOperationProviderReadersV2) InspectCurrentControlledOperationEffectV2(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (ports.ControlledOperationEffectCurrentProjectionV2, error) {
	return r.effect, nil
}

func (r *controlledOperationProviderReadersV2) InspectCurrentControlledOperationPreparedV2(context.Context, ports.PreparedProviderAttemptRefV2) (ports.ControlledOperationPreparedCurrentProjectionV2, error) {
	return r.prepared, nil
}

func (r *controlledOperationProviderReadersV2) InspectCurrentControlledOperationEvidencePolicyV2(context.Context, ports.OperationScopeEvidencePolicyRefV3) (ports.OperationScopeEvidencePolicyFactV3, error) {
	return r.evidencePolicy, nil
}

func (r *controlledOperationProviderReadersV2) InspectCurrentControlledOperationApplicabilityPolicyV2(context.Context, ports.OperationScopeEvidenceApplicabilityPolicyRefV3) (ports.OperationScopeEvidenceApplicabilityPolicyFactV3, error) {
	return r.applicabilityPolicy, nil
}

func (r *controlledOperationProviderReadersV2) InspectCurrentOperationProviderExecuteEnforcementV1(context.Context, ports.OperationSubjectV3, ports.OperationDispatchEnforcementPhaseRefV4) (ports.OperationDispatchEnforcementPhaseRefV4, error) {
	return r.enforcement, nil
}

func (r *controlledOperationProviderReadersV2) InspectCurrentOperationProviderEvidenceHandoffV1(context.Context, ports.OperationScopeEvidenceProviderHandoffRefV3) (ports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	return r.handoff, nil
}

func (r *controlledOperationProviderReadersV2) InspectCurrentOperationProviderBoundaryV1(context.Context, ports.OperationProviderBoundaryRefV1) (ports.OperationProviderBoundaryCurrentProjectionV1, error) {
	return r.boundary, nil
}

func (r *controlledOperationProviderReadersV2) IssueOperationScopeEvidenceV3(context.Context, ports.IssueOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	return ports.OperationScopeEvidenceQualificationFactV3{}, core.NewError(core.ErrorForbidden, core.ReasonEvidenceConflict, "fixture cannot mutate Evidence")
}

func (r *controlledOperationProviderReadersV2) InspectOperationScopeEvidenceV3(context.Context, ports.InspectOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	return r.qualification, nil
}

func (r *controlledOperationProviderReadersV2) InspectCurrentOperationScopeEvidenceV3(context.Context, ports.InspectCurrentOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	return r.qualification, nil
}

func (r *controlledOperationProviderReadersV2) HandoffOperationScopeEvidenceV3(context.Context, ports.HandoffOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	return ports.OperationScopeEvidenceProviderHandoffFactV3{}, core.NewError(core.ErrorForbidden, core.ReasonEvidenceConflict, "fixture cannot mutate Evidence")
}

func (r *controlledOperationProviderReadersV2) ConsumeOperationScopeEvidenceV3(context.Context, ports.ConsumeOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceConsumeResultV3, error) {
	return ports.OperationScopeEvidenceConsumeResultV3{}, core.NewError(core.ErrorForbidden, core.ReasonEvidenceConflict, "fixture cannot consume Evidence")
}

type controlledOperationProviderTransportStubV2 struct {
	mu                sync.Mutex
	calls             atomic.Int64
	logicalAdmissions atomic.Int64
	loseReply         atomic.Bool
	receipts          map[core.Digest]ports.ControlledOperationProviderAdmissionReceiptRefV2
}

func (s *controlledOperationProviderTransportStubV2) AdmitControlledOperationProviderV2(_ context.Context, request kernel.ControlledOperationProviderTransportRequestV2) (kernel.ControlledOperationProviderTransportResultV2, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.receipts == nil {
		s.receipts = map[core.Digest]ports.ControlledOperationProviderAdmissionReceiptRefV2{}
	}
	receipt, ok := s.receipts[request.StableKeyDigest]
	if !ok {
		var err error
		receipt, err = ports.SealControlledOperationProviderAdmissionReceiptRefV2(ports.ControlledOperationProviderAdmissionReceiptRefV2{
			ID: "cop2-admission", Revision: 1, StableKeyDigest: request.StableKeyDigest, Admitted: true,
		})
		if err != nil {
			return kernel.ControlledOperationProviderTransportResultV2{}, err
		}
		s.receipts[request.StableKeyDigest] = receipt
		s.logicalAdmissions.Add(1)
	}
	result := kernel.ControlledOperationProviderTransportResultV2{AdmissionReceipt: &receipt}
	if s.loseReply.CompareAndSwap(true, false) {
		return result, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "injected Provider reply loss")
	}
	return result, nil
}

type controlledOperationProviderInspectStubV2 struct {
	calls atomic.Int64
}

func (s *controlledOperationProviderInspectStubV2) InspectOriginalControlledProviderAttemptV2(context.Context, ports.PreparedProviderAttemptRefV2, ports.OperationDispatchAttemptRefV3) (ports.ProviderAttemptObservationRefV2, error) {
	s.calls.Add(1)
	return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "Provider observation not found")
}

type controlledOperationProviderMaliciousStoreV2 struct {
	fact control.ControlledOperationProviderEntryFactV2
}

func (s controlledOperationProviderMaliciousStoreV2) CreateControlledOperationProviderEntryV2(context.Context, control.ControlledOperationProviderEntryFactV2) (control.CreateControlledOperationProviderEntryResultV2, error) {
	return control.CreateControlledOperationProviderEntryResultV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "malicious fixture is read-only")
}

func (s controlledOperationProviderMaliciousStoreV2) InspectControlledOperationProviderEntryV2(context.Context, ports.OperationSubjectV3, string) (control.ControlledOperationProviderEntryFactV2, error) {
	return s.fact, nil
}

func (s controlledOperationProviderMaliciousStoreV2) CompareAndSwapControlledOperationProviderEntryV2(context.Context, ports.OperationSubjectV3, control.ControlledOperationProviderEntryCASRequestV2) (control.ControlledOperationProviderEntryFactV2, error) {
	return control.ControlledOperationProviderEntryFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "malicious fixture is read-only")
}

type controlledOperationProviderProgressOnCreateStoreV2 struct {
	delegate *fakes.ControlledOperationProviderEntryStoreV2
	now      time.Time
}

func (s *controlledOperationProviderProgressOnCreateStoreV2) CreateControlledOperationProviderEntryV2(ctx context.Context, fact control.ControlledOperationProviderEntryFactV2) (control.CreateControlledOperationProviderEntryResultV2, error) {
	created, err := s.delegate.CreateControlledOperationProviderEntryV2(ctx, fact)
	if err != nil {
		return control.CreateControlledOperationProviderEntryResultV2{}, err
	}
	if created.Disposition == control.ControlledOperationProviderEntryCreatedV2 {
		next := created.Fact
		next.Revision++
		next.State = control.ControlledOperationProviderEntryUnknownV2
		next.UpdatedUnixNano = s.now.UnixNano()
		next, err = control.SealControlledOperationProviderEntryFactV2(next)
		if err != nil {
			return control.CreateControlledOperationProviderEntryResultV2{}, err
		}
		if _, err = s.delegate.CompareAndSwapControlledOperationProviderEntryV2(ctx, fact.Request.Operation, control.ControlledOperationProviderEntryCASRequestV2{ExpectedRevision: created.Fact.Revision, Next: next}); err != nil {
			return control.CreateControlledOperationProviderEntryResultV2{}, err
		}
	}
	return control.CreateControlledOperationProviderEntryResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected create reply loss after another reconciler progressed the Entry")
}

func (s *controlledOperationProviderProgressOnCreateStoreV2) InspectControlledOperationProviderEntryV2(ctx context.Context, operation ports.OperationSubjectV3, entryID string) (control.ControlledOperationProviderEntryFactV2, error) {
	return s.delegate.InspectControlledOperationProviderEntryV2(ctx, operation, entryID)
}

func (s *controlledOperationProviderProgressOnCreateStoreV2) CompareAndSwapControlledOperationProviderEntryV2(ctx context.Context, operation ports.OperationSubjectV3, request control.ControlledOperationProviderEntryCASRequestV2) (control.ControlledOperationProviderEntryFactV2, error) {
	return s.delegate.CompareAndSwapControlledOperationProviderEntryV2(ctx, operation, request)
}

type controlledOperationProviderProgressOnCASStoreV2 struct {
	delegate *fakes.ControlledOperationProviderEntryStoreV2
	now      time.Time
	once     atomic.Bool
}

func (s *controlledOperationProviderProgressOnCASStoreV2) CreateControlledOperationProviderEntryV2(ctx context.Context, fact control.ControlledOperationProviderEntryFactV2) (control.CreateControlledOperationProviderEntryResultV2, error) {
	return s.delegate.CreateControlledOperationProviderEntryV2(ctx, fact)
}

func (s *controlledOperationProviderProgressOnCASStoreV2) InspectControlledOperationProviderEntryV2(ctx context.Context, operation ports.OperationSubjectV3, entryID string) (control.ControlledOperationProviderEntryFactV2, error) {
	return s.delegate.InspectControlledOperationProviderEntryV2(ctx, operation, entryID)
}

func (s *controlledOperationProviderProgressOnCASStoreV2) CompareAndSwapControlledOperationProviderEntryV2(ctx context.Context, operation ports.OperationSubjectV3, request control.ControlledOperationProviderEntryCASRequestV2) (control.ControlledOperationProviderEntryFactV2, error) {
	stored, err := s.delegate.CompareAndSwapControlledOperationProviderEntryV2(ctx, operation, request)
	if err != nil || request.Next.State != control.ControlledOperationProviderEntryUnknownV2 || !s.once.CompareAndSwap(false, true) {
		return stored, err
	}
	observation := ports.ProviderAttemptObservationRefV2{
		Delegation: request.Next.Request.PreparedSemantics.Delegation, PreparedAttemptID: request.Next.Request.Prepared.ID,
		ProviderOperationRef: "cop2-provider-operation", Revision: 1, State: ports.ProviderAttemptObservedV2,
		Digest: digestV3("cop2-observation"), PayloadDigest: digestV3("cop2-observation-payload"), PayloadRevision: 1,
		SourceRegistrationID: "cop2-observation-source", SourceEpoch: 1, SourceSequence: 1,
		Evidence:         ports.EvidenceRecordRefV2{LedgerScopeDigest: digestV3("cop2-observation-ledger"), Sequence: 1, RecordDigest: digestV3("cop2-observation-record")},
		ObservedUnixNano: s.now.UnixNano(),
	}
	next := stored
	next.Revision++
	next.State = control.ControlledOperationProviderEntryObservedV2
	next.Observation = &observation
	next.UpdatedUnixNano = s.now.UnixNano()
	next, err = control.SealControlledOperationProviderEntryFactV2(next)
	if err != nil {
		return control.ControlledOperationProviderEntryFactV2{}, err
	}
	if _, err = s.delegate.CompareAndSwapControlledOperationProviderEntryV2(ctx, operation, control.ControlledOperationProviderEntryCASRequestV2{ExpectedRevision: stored.Revision, Next: next}); err != nil {
		return control.ControlledOperationProviderEntryFactV2{}, err
	}
	return control.ControlledOperationProviderEntryFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected CAS reply loss after another reconciler observed the Entry")
}

func mustControlledOperationProviderEntryKeyV2(t *testing.T, request ports.ControlledOperationProviderRequestV2) ports.ControlledOperationProviderInspectKeyV2 {
	t.Helper()
	key, err := ports.DeriveControlledOperationProviderEntryKeyV2(request)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
