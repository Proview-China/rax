package application_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestWorkflowPlanV2AcceptsCustomEighthComponentWithoutEnumChange(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bundle, _ := applicationFixtureV2(t, now, "custom.eighth/process", true)
	if err := bundle.Validate(now); err != nil {
		t.Fatalf("custom workflow rejected: %v", err)
	}
	digest1, err := bundle.Plan.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	digest2, err := bundle.Plan.DigestV2()
	if err != nil || digest1 != digest2 {
		t.Fatal("workflow digest is nondeterministic")
	}
}

func TestWorkflowPlanV2RejectsCycleUnknownNamespaceAndProviderSmuggling(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bundle, descriptor := applicationFixtureV2(t, now, "custom.eighth/process", true)
	second := bundle.Plan.Steps[0]
	second.ID = "step-b"
	second.Dependencies = []string{"step-a"}
	bundle.Plan.Steps[0].Dependencies = []string{"step-b"}
	bundle.Plan.Steps = []contract.WorkflowStepV2{bundle.Plan.Steps[0], second}
	if err := bundle.Plan.Validate(now); err == nil {
		t.Fatal("cyclic workflow accepted")
	}
	bundle, _ = applicationFixtureV2(t, now, "custom.eighth/process", true)
	bundle.Plan.Steps[0].Kind = "not-namespaced"
	if err := bundle.Plan.Validate(now); err == nil {
		t.Fatal("unnamespaced custom kind accepted")
	}
	bundle, _ = applicationFixtureV2(t, now, "custom.eighth/process", true)
	bundle.Plan.Steps[0].ExecutionClass = contract.StepCoordinationV2
	if err := bundle.Plan.Validate(now); err == nil {
		t.Fatal("coordination step smuggled provider binding")
	}
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("fixture descriptor invalid: %v", err)
	}
}

func TestWorkflowJournalV2EnforcesDependencyAndSingleStepCAS(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bundle, _ := applicationFixtureV2(t, now, "custom.eighth/process", true)
	second := bundle.Plan.Steps[0]
	second.ID, second.Dependencies = "step-b", []string{"step-a"}
	bundle.Plan.Steps = []contract.WorkflowStepV2{bundle.Plan.Steps[0], second}
	journal, err := contract.NewWorkflowJournalV2("journal-deps", bundle.Plan, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	early := journal
	early.Steps = append([]contract.WorkflowStepProgressV2(nil), journal.Steps...)
	early.Revision = 2
	early.Steps[1].State, early.Steps[1].UpdatedUnixNano = contract.StepReadyV2, now.Add(time.Second).UnixNano()
	early.UpdatedUnixNano = early.Steps[1].UpdatedUnixNano
	if err := contract.ValidateWorkflowJournalTransitionV2(bundle.Plan, journal, early); err == nil {
		t.Fatal("dependency became ready before prerequisite completed")
	}
	effect := &contract.ApplicationFactRefV2{Ref: "effect-a", Revision: 2, Digest: core.DigestBytes([]byte("effect-a"))}
	dispatch := journal
	dispatch.Steps = append([]contract.WorkflowStepProgressV2(nil), journal.Steps...)
	dispatch.Revision = 2
	dispatch.Steps[0].State, dispatch.Steps[0].Attempt, dispatch.Steps[0].Effect, dispatch.Steps[0].UpdatedUnixNano = contract.StepDispatchIntentV2, 1, effect, now.Add(time.Second).UnixNano()
	dispatch.UpdatedUnixNano = dispatch.Steps[0].UpdatedUnixNano
	dispatch.Status = contract.DeriveWorkflowStatusV2(dispatch.Steps)
	if err := contract.ValidateWorkflowJournalTransitionV2(bundle.Plan, journal, dispatch); err != nil {
		t.Fatalf("valid dispatch intent rejected: %v", err)
	}
	waiting := dispatch
	waiting.Steps = append([]contract.WorkflowStepProgressV2(nil), dispatch.Steps...)
	waiting.Revision = 3
	waiting.Steps[0].State, waiting.Steps[0].UpdatedUnixNano = contract.StepWaitingInspectV2, now.Add(2*time.Second).UnixNano()
	waiting.UpdatedUnixNano = waiting.Steps[0].UpdatedUnixNano
	waiting.Status = contract.DeriveWorkflowStatusV2(waiting.Steps)
	if err := contract.ValidateWorkflowJournalTransitionV2(bundle.Plan, dispatch, waiting); err != nil {
		t.Fatalf("valid inspect wait rejected: %v", err)
	}
	settlement := &contract.ApplicationFactRefV2{Ref: "settlement-a", Revision: 1, Digest: core.DigestBytes([]byte("settlement-a"))}
	completed := waiting
	completed.Steps = append([]contract.WorkflowStepProgressV2(nil), waiting.Steps...)
	completed.Revision = 4
	completed.Steps[0].State, completed.Steps[0].Settlement, completed.Steps[0].UpdatedUnixNano = contract.StepCompletedV2, settlement, now.Add(3*time.Second).UnixNano()
	completed.UpdatedUnixNano = completed.Steps[0].UpdatedUnixNano
	completed.Status = contract.DeriveWorkflowStatusV2(completed.Steps)
	if err := contract.ValidateWorkflowJournalTransitionV2(bundle.Plan, waiting, completed); err != nil {
		t.Fatalf("valid completion rejected: %v", err)
	}
	forgedDrop := completed
	forgedDrop.Steps = append([]contract.WorkflowStepProgressV2(nil), completed.Steps...)
	forgedDrop.Steps[0].Effect = nil
	if err := contract.ValidateWorkflowJournalTransitionV2(bundle.Plan, waiting, forgedDrop); !core.HasReason(err, core.ReasonEffectStateConflict) {
		t.Fatalf("governed completion discarded its write-ahead Effect: %v", err)
	}
	ready := completed
	ready.Steps = append([]contract.WorkflowStepProgressV2(nil), completed.Steps...)
	ready.Revision = 5
	ready.Steps[1].State, ready.Steps[1].UpdatedUnixNano = contract.StepReadyV2, now.Add(4*time.Second).UnixNano()
	ready.UpdatedUnixNano = ready.Steps[1].UpdatedUnixNano
	if err := contract.ValidateWorkflowJournalTransitionV2(bundle.Plan, completed, ready); err != nil {
		t.Fatalf("dependent step did not become ready: %v", err)
	}
	two := completed
	two.Steps = append([]contract.WorkflowStepProgressV2(nil), completed.Steps...)
	two.Revision = 5
	two.Steps[0].LastError = "changed"
	two.Steps[1].State, two.Steps[1].UpdatedUnixNano = contract.StepReadyV2, now.Add(4*time.Second).UnixNano()
	two.UpdatedUnixNano = two.Steps[1].UpdatedUnixNano
	if err := contract.ValidateWorkflowJournalTransitionV2(bundle.Plan, completed, two); err == nil {
		t.Fatal("one CAS changed two steps")
	}
}

func TestOutboxDispatcherV2PersistsJournalBeforeMarkAndRecoversLostReplies(t *testing.T) {
	for _, fault := range []string{"none", "journal_create", "mark_outbox"} {
		t.Run(fault, func(t *testing.T) {
			now := time.Unix(1_800_000_000, 0)
			bundle, descriptor := applicationFixtureV2(t, now, "custom.eighth/process", true)
			runtimeStore, appStore := acceptedSubmissionFixtureV2(t, now, bundle)
			if err := appStore.RegisterStepDescriptorV2(descriptor); err != nil {
				t.Fatal(err)
			}
			if fault == "journal_create" {
				appStore.LoseNextJournalCreateReply = true
			}
			var commands runtimeports.ApplicationCommandFactPortV2 = runtimeStore
			if fault == "mark_outbox" {
				commands = &lostMarkOutboxV2{ApplicationCommandFactPortV2: runtimeStore}
			}
			dispatcher, err := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: commands, Submissions: appStore, Journals: appStore, StepCatalog: appStore, Clock: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			result, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID)
			if err != nil {
				t.Fatalf("dispatch handoff failed at %s: %v", fault, err)
			}
			if !result.Outbox.Dispatched || result.Journal.Revision != 1 || result.Journal.Status != contract.WorkflowAcceptedV2 {
				t.Fatalf("unexpected handoff: %#v", result)
			}
			replayed, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID)
			if err != nil || replayed.Journal.ID != result.Journal.ID || replayed.Journal.Revision != result.Journal.Revision {
				t.Fatalf("replay changed handoff: %#v err=%v", replayed, err)
			}
		})
	}
}

func TestFacadeV2RecoversSubmissionAndCommandAcceptanceReplyLoss(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bundle, _ := applicationFixtureV2(t, now, "custom.eighth/process", true)
	runtimeStore := runtimefakes.NewFactStore(func() time.Time { return now })
	if _, err := runtimeStore.CreateDesiredState(context.Background(), runtimeports.DesiredStateSnapshotV2{Scope: bundle.Command.Target, Desired: runtimeports.DesiredRunningV2, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	appStore := fakes.NewFactStoreV2()
	appStore.Clock = func() time.Time { return now }
	appStore.LoseNextSubmissionReply = true
	commands := &lostAcceptCommandV2{ApplicationCommandFactPortV2: runtimeStore}
	facade, err := application.NewFacadeV2(application.FacadeConfigV2{Commands: commands, Submissions: appStore, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.SubmitWorkflowV2(context.Background(), application.SubmitWorkflowRequestV2{Bundle: bundle, Mutation: runtimeports.DesiredStateMutationV2{Desired: runtimeports.DesiredRunningV2}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Command.Envelope.ID != bundle.Command.ID || result.Outbox.CommandID != bundle.Command.ID || result.Outbox.Dispatched {
		t.Fatalf("unexpected facade recovery: %#v", result)
	}
	replayed, err := facade.SubmitWorkflowV2(context.Background(), application.SubmitWorkflowRequestV2{Bundle: bundle, Mutation: runtimeports.DesiredStateMutationV2{Desired: runtimeports.DesiredRunningV2}})
	if err != nil || replayed.Command.Revision != result.Command.Revision {
		t.Fatalf("facade replay changed accepted facts: %#v err=%v", replayed, err)
	}
}

func TestJournalCoordinatorV2RecoversCASAndReadiesDependenciesWithoutRedispatch(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	clock := now
	bundle, _ := applicationFixtureV2(t, now, "custom.eighth/process", true)
	second := bundle.Plan.Steps[0]
	second.ID, second.Dependencies = "step-b", []string{"step-a"}
	bundle.Plan.Steps = []contract.WorkflowStepV2{bundle.Plan.Steps[0], second}
	journal, err := contract.NewWorkflowJournalV2("journal-coordinator", bundle.Plan, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewFactStoreV2()
	store.Clock = func() time.Time { return clock }
	if _, err := store.CreateWorkflowJournalV2(context.Background(), bundle.Plan, journal); err != nil {
		t.Fatal(err)
	}
	coordinator, _ := application.NewJournalCoordinatorV2(application.JournalCoordinatorConfigV2{Facts: store, Clock: func() time.Time { return clock }})
	effect := &contract.ApplicationFactRefV2{Ref: "effect-a", Revision: 1, Digest: core.DigestBytes([]byte("effect-a"))}
	store.LoseNextJournalCASReply = true
	clock = now.Add(time.Second)
	dispatching, err := coordinator.AdvanceStepV2(context.Background(), application.AdvanceStepRequestV2{Plan: bundle.Plan, JournalID: journal.ID, StepID: "step-a", Target: contract.StepDispatchIntentV2, Effect: effect})
	if err != nil || dispatching.Revision != 2 {
		t.Fatalf("dispatch intent CAS recovery failed: %#v err=%v", dispatching, err)
	}
	replayed, err := coordinator.AdvanceStepV2(context.Background(), application.AdvanceStepRequestV2{Plan: bundle.Plan, JournalID: journal.ID, StepID: "step-a", Target: contract.StepDispatchIntentV2, Effect: effect})
	if err != nil || replayed.Revision != 2 || replayed.Steps[0].Attempt != 1 {
		t.Fatalf("dispatch replay created another attempt: %#v err=%v", replayed, err)
	}
	clock = now.Add(2 * time.Second)
	waiting, err := coordinator.AdvanceStepV2(context.Background(), application.AdvanceStepRequestV2{Plan: bundle.Plan, JournalID: journal.ID, StepID: "step-a", Target: contract.StepWaitingInspectV2, Effect: effect})
	if err != nil || waiting.Status != contract.WorkflowWaitingInspectV2 {
		t.Fatalf("waiting inspect failed: %#v err=%v", waiting, err)
	}
	settlement := &contract.ApplicationFactRefV2{Ref: "settlement-a", Revision: 1, Digest: core.DigestBytes([]byte("settlement-a"))}
	clock = now.Add(3 * time.Second)
	completed, err := coordinator.AdvanceStepV2(context.Background(), application.AdvanceStepRequestV2{Plan: bundle.Plan, JournalID: journal.ID, StepID: "step-a", Target: contract.StepCompletedV2, Settlement: settlement})
	if !core.HasCategory(err, core.ErrorForbidden) || completed.Revision != 0 {
		t.Fatalf("legacy raw completion did not fail closed: %#v err=%v", completed, err)
	}
}

func TestJournalCoordinatorV2RejectsNewWorkAfterPlanExpiryAndRawGovernedSettlement(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bundle, _ := applicationFixtureV2(t, now, "custom.eighth/process", true)
	store := fakes.NewFactStoreV2()
	journal, err := contract.NewWorkflowJournalV2("journal-expiry", bundle.Plan, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkflowJournalV2(context.Background(), bundle.Plan, journal); err != nil {
		t.Fatal(err)
	}
	effect := &contract.ApplicationFactRefV2{Ref: "effect-expiry", Revision: 2, Digest: core.DigestBytes([]byte("effect-expiry"))}
	beforeExpiry, _ := application.NewJournalCoordinatorV2(application.JournalCoordinatorConfigV2{Facts: store, Clock: func() time.Time { return now.Add(time.Minute) }})
	dispatched, err := beforeExpiry.AdvanceStepV2(context.Background(), application.AdvanceStepRequestV2{Plan: bundle.Plan, JournalID: journal.ID, StepID: "step-a", Target: contract.StepDispatchIntentV2, Effect: effect})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := beforeExpiry.AdvanceStepV2(context.Background(), application.AdvanceStepRequestV2{Plan: bundle.Plan, JournalID: journal.ID, StepID: "step-a", Target: contract.StepWaitingInspectV2, Effect: effect})
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Revision != dispatched.Revision+1 {
		t.Fatal("inspect wait was not persisted")
	}
	afterExpiry, _ := application.NewJournalCoordinatorV2(application.JournalCoordinatorConfigV2{Facts: store, Clock: func() time.Time { return now.Add(2 * time.Hour) }})
	settlement := &contract.ApplicationFactRefV2{Ref: "settlement-expiry", Revision: 1, Digest: core.DigestBytes([]byte("settlement-expiry"))}
	completed, err := afterExpiry.AdvanceStepV2(context.Background(), application.AdvanceStepRequestV2{Plan: bundle.Plan, JournalID: journal.ID, StepID: "step-a", Target: contract.StepCompletedV2, Settlement: settlement})
	if !core.HasCategory(err, core.ErrorForbidden) || completed.Revision != 0 {
		t.Fatalf("raw governed settlement did not fail closed: %#v err=%v", completed, err)
	}

	fresh, _ := applicationFixtureV2(t, now, "custom.eighth/process", true)
	freshJournal, _ := contract.NewWorkflowJournalV2("journal-expired-new", fresh.Plan, now.UnixNano())
	freshStore := fakes.NewFactStoreV2()
	if _, err := freshStore.CreateWorkflowJournalV2(context.Background(), fresh.Plan, freshJournal); err != nil {
		t.Fatal(err)
	}
	expiredCoordinator, _ := application.NewJournalCoordinatorV2(application.JournalCoordinatorConfigV2{Facts: freshStore, Clock: func() time.Time { return now.Add(2 * time.Hour) }})
	if _, err := expiredCoordinator.AdvanceStepV2(context.Background(), application.AdvanceStepRequestV2{Plan: fresh.Plan, JournalID: freshJournal.ID, StepID: "step-a", Target: contract.StepDispatchIntentV2, Effect: effect}); !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("expired plan started new provider work: %v", err)
	}
}

func TestOutboxDispatcherV2UnknownRequiredFailsClosedOptionalIsPreservedAndSkipped(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	required, _ := applicationFixtureV2(t, now, "custom.unknown/process", true)
	runtimeStore, appStore := acceptedSubmissionFixtureV2(t, now, required)
	dispatcher, _ := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore, Submissions: appStore, Journals: appStore, StepCatalog: appStore, Clock: func() time.Time { return now }})
	if _, err := dispatcher.DispatchCommandV2(context.Background(), required.Command.Target, required.Command.ID); !core.HasReason(err, core.ReasonUnknownCapability) {
		t.Fatalf("unknown required step did not fail closed: %v", err)
	}
	outbox, _ := runtimeStore.ListOutbox(context.Background(), required.Command.Target)
	if outbox[0].Dispatched {
		t.Fatal("required unknown step marked outbox dispatched")
	}

	optional, _ := applicationFixtureV2(t, now, "custom.unknown/process", false)
	optional.Command.ID, optional.Command.IdempotencyKey, optional.Payload.CommandID, optional.Plan.CommandID = "command-optional", "idem-optional", "command-optional", "command-optional"
	optional.Payload.Payload.ContentDigest = optional.Command.CanonicalPayloadDigest
	payloadDigest, _ := optional.Payload.DigestV2()
	optional.Plan.CommandPayloadDigest = payloadDigest
	runtimeStore2, appStore2 := acceptedSubmissionFixtureV2(t, now, optional)
	dispatcher2, _ := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore2, Submissions: appStore2, Journals: appStore2, StepCatalog: appStore2, Clock: func() time.Time { return now }})
	result, err := dispatcher2.DispatchCommandV2(context.Background(), optional.Command.Target, optional.Command.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Journal.Status != contract.WorkflowCompletedV2 || result.Journal.Steps[0].State != contract.StepSkippedV2 || result.Journal.Steps[0].LastError == "" {
		t.Fatalf("optional unknown was not preserved as explicit skip: %#v", result.Journal)
	}
}

func TestGovernedWorkflowWithoutDomainAdapterCannotCreateJournalOrDispatchOutbox(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	valid, _ := applicationFixtureV2(t, now, "custom.domain-required/process", true)
	runtimeStore, appStore := acceptedSubmissionFixtureV2(t, now, valid)
	invalid := valid
	invalid.Plan.Steps = append([]contract.WorkflowStepV2(nil), valid.Plan.Steps...)
	invalid.Plan.Steps[0].DomainAdapter = nil
	if err := invalid.Plan.Validate(now); !core.HasReason(err, core.ReasonProviderBindingStale) {
		t.Fatalf("governed step without DomainAdapter was valid: %v", err)
	}
	dispatcher, err := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{
		Commands:    runtimeStore,
		Submissions: fixedSubmissionV2{bundle: invalid},
		Journals:    appStore,
		StepCatalog: appStore,
		Clock:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dispatcher.DispatchCommandV2(context.Background(), invalid.Command.Target, invalid.Command.ID); !core.HasReason(err, core.ReasonProviderBindingStale) {
		t.Fatalf("invalid governed plan reached dispatch path: %v", err)
	}
	journals, err := appStore.ListWorkflowJournalsV2(context.Background(), invalid.Command.Target, "", 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(journals) != 0 {
		t.Fatalf("invalid governed plan created a Journal: %#v", journals)
	}
	outbox, err := runtimeStore.ListOutbox(context.Background(), invalid.Command.Target)
	if err != nil || len(outbox) != 1 || outbox[0].Dispatched {
		t.Fatalf("invalid governed plan changed Outbox dispatch state: %#v err=%v", outbox, err)
	}
}

func TestSkippedOptionalStepSatisfiesDependencyWithoutInstallingItsModule(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	optionalKind := runtimeports.NamespacedNameV2("custom.optional/missing")
	requiredKind := runtimeports.NamespacedNameV2("custom.required/available")
	bundle, descriptor := applicationFixtureV2(t, now, optionalKind, false)
	second := bundle.Plan.Steps[0]
	second.ID = "step-b"
	second.Kind = requiredKind
	second.Required = true
	second.Dependencies = []string{"step-a"}
	bundle.Plan.Steps = []contract.WorkflowStepV2{bundle.Plan.Steps[0], second}
	payloadDigest, err := bundle.Payload.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	bundle.Plan.CommandPayloadDigest = payloadDigest

	runtimeStore := runtimefakes.NewFactStore(func() time.Time { return now })
	if _, err := runtimeStore.CreateDesiredState(context.Background(), runtimeports.DesiredStateSnapshotV2{Scope: bundle.Command.Target, Desired: runtimeports.DesiredRunningV2, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	appStore := fakes.NewFactStoreV2()
	appStore.Clock = func() time.Time { return now }
	available := descriptor
	available.Kind = requiredKind
	availableRef, err := available.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	bundle.Plan.Steps[1].Descriptor = availableRef
	if err := appStore.RegisterStepDescriptorV2(available); err != nil {
		t.Fatal(err)
	}
	facade, err := application.NewFacadeV2(application.FacadeConfigV2{Commands: runtimeStore, Submissions: appStore, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.SubmitWorkflowV2(context.Background(), application.SubmitWorkflowRequestV2{Bundle: bundle, Mutation: runtimeports.DesiredStateMutationV2{Desired: runtimeports.DesiredRunningV2}}); err != nil {
		t.Fatal(err)
	}
	dispatcher, _ := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore, Submissions: appStore, Journals: appStore, StepCatalog: appStore, Clock: func() time.Time { return now }})
	result, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Journal.Steps[0].State != contract.StepSkippedV2 || result.Journal.Steps[1].State != contract.StepPendingV2 {
		t.Fatalf("unexpected post-dispatch states: %#v", result.Journal.Steps)
	}
	coordinator, _ := application.NewJournalCoordinatorV2(application.JournalCoordinatorConfigV2{Facts: appStore, Clock: func() time.Time { return now.Add(time.Second) }})
	refreshed, err := coordinator.RefreshReadyStepsV2(context.Background(), bundle.Plan, result.Journal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Steps[1].State != contract.StepReadyV2 {
		t.Fatalf("optional no-op must satisfy dependency, got %s", refreshed.Steps[1].State)
	}
}

func TestOptionalStepCatalogOutageDoesNotBecomeAFalseSkip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	bundle, _ := applicationFixtureV2(t, now, "custom.optional/process", false)
	runtimeStore := runtimefakes.NewFactStore(func() time.Time { return now })
	if _, err := runtimeStore.CreateDesiredState(context.Background(), runtimeports.DesiredStateSnapshotV2{Scope: bundle.Command.Target, Desired: runtimeports.DesiredRunningV2, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	appStore := fakes.NewFactStoreV2()
	appStore.Clock = func() time.Time { return now }
	facade, _ := application.NewFacadeV2(application.FacadeConfigV2{Commands: runtimeStore, Submissions: appStore, Clock: func() time.Time { return now }})
	if _, err := facade.SubmitWorkflowV2(context.Background(), application.SubmitWorkflowRequestV2{Bundle: bundle, Mutation: runtimeports.DesiredStateMutationV2{Desired: runtimeports.DesiredRunningV2}}); err != nil {
		t.Fatal(err)
	}
	dispatcher, _ := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore, Submissions: appStore, Journals: appStore, StepCatalog: unavailableStepCatalogV2{}, Clock: func() time.Time { return now }})
	if _, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("catalog outage must remain retryable, got %v", err)
	}
	outbox, err := runtimeStore.ListOutbox(context.Background(), bundle.Command.Target)
	if err != nil || len(outbox) != 1 || outbox[0].Dispatched {
		t.Fatalf("catalog outage must not mark the handoff dispatched: %#v err=%v", outbox, err)
	}
}

func TestStepKindCannotBeReboundToAnotherProviderCapability(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	bundle, descriptor := applicationFixtureV2(t, now, "custom.eighth/process", true)
	descriptor.RequiredCapability = "custom.eighth/read-only"
	runtimeStore, appStore := acceptedSubmissionFixtureV2(t, now, bundle)
	if err := appStore.RegisterStepDescriptorV2(descriptor); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore, Submissions: appStore, Journals: appStore, StepCatalog: appStore, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID); !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("registered step kind must not authorize a different provider capability: %v", err)
	}
	outbox, err := runtimeStore.ListOutbox(context.Background(), bundle.Command.Target)
	if err != nil || len(outbox) != 1 || outbox[0].Dispatched {
		t.Fatalf("capability drift must leave the handoff undispatched: %#v err=%v", outbox, err)
	}
}

func TestStepDescriptorDigestAndTTLAreRevalidatedAtJournalHandoff(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	bundle, descriptor := applicationFixtureV2(t, now, "custom.eighth/process", true)
	descriptor.ExpiresUnixNano = now.Add(30 * time.Minute).UnixNano()
	ref, err := descriptor.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	bundle.Plan.Steps[0].Descriptor = ref
	runtimeStore, appStore := acceptedSubmissionFixtureV2(t, now, bundle)
	if err := appStore.RegisterStepDescriptorV2(descriptor); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore, Submissions: appStore, Journals: appStore, StepCatalog: appStore, Clock: func() time.Time { return now.Add(45 * time.Minute) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID); !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("expired step descriptor started new work: %v", err)
	}

	bundle, descriptor = applicationFixtureV2(t, now, "custom.eighth/process", true)
	runtimeStore, appStore = acceptedSubmissionFixtureV2(t, now, bundle)
	descriptor.Revision++
	if err := appStore.RegisterStepDescriptorV2(descriptor); err != nil {
		t.Fatal(err)
	}
	dispatcher, _ = application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore, Submissions: appStore, Journals: appStore, StepCatalog: appStore, Clock: func() time.Time { return now }})
	if _, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID); !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("descriptor revision/digest drift reached Journal handoff: %v", err)
	}
}

func TestOptionalStepSkipFailsClosedOnClockRegression(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	bundle, _ := applicationFixtureV2(t, now, "custom.optional/process", false)
	runtimeStore := runtimefakes.NewFactStore(func() time.Time { return now })
	if _, err := runtimeStore.CreateDesiredState(context.Background(), runtimeports.DesiredStateSnapshotV2{Scope: bundle.Command.Target, Desired: runtimeports.DesiredRunningV2, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	appStore := fakes.NewFactStoreV2()
	appStore.Clock = func() time.Time { return now }
	facade, _ := application.NewFacadeV2(application.FacadeConfigV2{Commands: runtimeStore, Submissions: appStore, Clock: func() time.Time { return now }})
	if _, err := facade.SubmitWorkflowV2(context.Background(), application.SubmitWorkflowRequestV2{Bundle: bundle, Mutation: runtimeports.DesiredStateMutationV2{Desired: runtimeports.DesiredRunningV2}}); err != nil {
		t.Fatal(err)
	}
	dispatcher, _ := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore, Submissions: appStore, Journals: appStore, StepCatalog: appStore, Clock: func() time.Time { return now.Add(-time.Nanosecond) }})
	if _, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("regressed clock must not be hidden by a fake timestamp: %v", err)
	}
}

type unavailableStepCatalogV2 struct{}

func (unavailableStepCatalogV2) ResolveStepKindV2(context.Context, runtimeports.NamespacedNameV2) (applicationports.StepKindDescriptorV2, error) {
	return applicationports.StepKindDescriptorV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "catalog projection unavailable")
}

func TestOutboxDispatcherV2ConcurrentReplayCreatesOneJournal(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bundle, descriptor := applicationFixtureV2(t, now, "custom.eighth/process", true)
	runtimeStore, appStore := acceptedSubmissionFixtureV2(t, now, bundle)
	_ = appStore.RegisterStepDescriptorV2(descriptor)
	dispatcher, _ := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore, Submissions: appStore, Journals: appStore, StepCatalog: appStore, Clock: func() time.Time { return now }})
	var successes atomic.Int32
	var wait sync.WaitGroup
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID); err == nil {
				successes.Add(1)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 100 {
		t.Fatalf("only %d replay callers recovered the one handoff", successes.Load())
	}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(bundle.Command.Target)
	journalID := "workflow-journal:" + string(scopeDigest) + ":" + bundle.Command.ID
	journal, err := appStore.InspectWorkflowJournalV2(context.Background(), bundle.Command.Target, journalID)
	if err != nil || journal.Revision != 1 {
		t.Fatalf("concurrent replay created extra journal revisions: %#v err=%v", journal, err)
	}
}

func TestWorkflowRecoveryClaimIsPartitionedCASLeasedAndLostReplyInspectable(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bundle, descriptor := applicationFixtureV2(t, now, "custom.eighth/process", true)
	runtimeStore, appStore := acceptedSubmissionFixtureV2(t, now, bundle)
	appStore.Clock = func() time.Time { return now }
	if err := appStore.RegisterStepDescriptorV2(descriptor); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := application.NewOutboxDispatcherV2(application.OutboxDispatcherConfigV2{Commands: runtimeStore, Submissions: appStore, Journals: appStore, StepCatalog: appStore, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	dispatched, err := dispatcher.DispatchCommandV2(context.Background(), bundle.Command.Target, bundle.Command.ID)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := application.NewRecoveryCoordinatorV2(application.RecoveryCoordinatorConfigV2{Recovery: appStore, Facts: appStore, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := recovery.ListRecoverableV2(context.Background(), applicationports.WorkflowJournalListRequestV2{Scope: bundle.Command.Target, Limit: 1})
	if err != nil || len(listed) != 1 || listed[0].ID != dispatched.Journal.ID {
		t.Fatalf("recoverable journal listing mismatch: %#v err=%v", listed, err)
	}
	policy := core.DigestBytes([]byte("application-worker-policy"))
	request := applicationports.WorkflowJournalClaimRequestV2{Scope: bundle.Command.Target, JournalID: dispatched.Journal.ID, OwnerID: "worker-a", PolicyDigest: policy, LeaseNanos: int64(time.Minute)}
	appStore.LoseNextClaimReply = true
	first, err := recovery.AcquireV2(context.Background(), request)
	if err != nil || first.OwnerID != request.OwnerID || first.Epoch != 1 || first.Revision != 1 || first.State != applicationports.WorkflowJournalClaimActiveV2 {
		t.Fatalf("lost claim reply was not inspectable: %#v err=%v", first, err)
	}
	contender := request
	contender.OwnerID = "worker-b"
	contender.ExpectedRevision = first.Revision
	if _, err := appStore.ClaimWorkflowJournalV2(context.Background(), contender); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("active claim must block another worker: %v", err)
	}
	now = now.Add(time.Minute)
	second, err := recovery.AcquireV2(context.Background(), contender)
	if err != nil {
		t.Fatal(err)
	}
	if second.OwnerID != "worker-b" || second.Epoch != first.Epoch+1 || second.Revision != first.Revision+1 {
		t.Fatalf("expired claim takeover did not fence the old worker: %#v", second)
	}
	if _, err := appStore.ReleaseWorkflowJournalClaimV2(context.Background(), applicationports.WorkflowJournalReleaseRequestV2{Scope: bundle.Command.Target, JournalID: second.JournalID, OwnerID: first.OwnerID, Epoch: first.Epoch, ExpectedRevision: second.Revision}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale worker released a newer claim: %v", err)
	}
	appStore.LoseNextClaimReleaseReply = true
	released, err := recovery.ReleaseV2(context.Background(), applicationports.WorkflowJournalReleaseRequestV2{Scope: bundle.Command.Target, JournalID: second.JournalID, OwnerID: second.OwnerID, Epoch: second.Epoch, ExpectedRevision: second.Revision})
	if err != nil || released.State != applicationports.WorkflowJournalClaimReleasedV2 || released.Revision != second.Revision+1 {
		t.Fatalf("lost release reply was not inspectable: %#v err=%v", released, err)
	}
}

func TestApplicationFactStoreV2ClonesOpaqueAndScopeValues(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bundle, _ := applicationFixtureV2(t, now, "custom.eighth/process", true)
	store := fakes.NewFactStoreV2()
	store.Clock = func() time.Time { return now }
	originalByte := bundle.Payload.Payload.Inline[0]
	originalEpoch := bundle.Command.Target.SandboxLease.Epoch
	originalScope := bundle.Command.Target
	originalLease := *bundle.Command.Target.SandboxLease
	originalScope.SandboxLease = &originalLease
	if _, err := store.CreateSubmissionBundleV2(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	bundle.Payload.Payload.Inline[0] ^= 0xff
	bundle.Command.Target.SandboxLease.Epoch++
	read, err := store.InspectSubmissionBundleV2(context.Background(), originalScope, bundle.Command.ID)
	if err != nil || read.Payload.Payload.Inline[0] != originalByte || read.Command.Target.SandboxLease.Epoch != originalEpoch {
		t.Fatalf("caller mutation escaped into store: %#v err=%v", read, err)
	}
	read.Payload.Payload.Inline[0] ^= 0xff
	read.Command.Target.SandboxLease.Epoch++
	again, err := store.InspectSubmissionBundleV2(context.Background(), originalScope, bundle.Command.ID)
	if err != nil || again.Payload.Payload.Inline[0] != originalByte || again.Command.Target.SandboxLease.Epoch != originalEpoch {
		t.Fatalf("reader mutation escaped into store: %#v err=%v", again, err)
	}
}

func TestApplicationFactsPartitionSameIDsByFullExecutionScope(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	first, _ := applicationFixtureV2(t, now, "custom.eighth/process", true)
	second, _ := applicationFixtureV2(t, now, "custom.eighth/process", true)
	second.Command.Target.Identity.TenantID = "tenant-2"
	second.Command.Target.Identity.ID = "agent-2"
	second.Command.Target.Lineage.ID = "lineage-2"
	second.Command.Target.Instance.ID = "instance-2"
	second.Command.Target.SandboxLease = &core.SandboxLeaseRef{ID: "sandbox-2", Epoch: 1}
	second.Plan.Target = second.Command.Target

	store := fakes.NewFactStoreV2()
	store.Clock = func() time.Time { return now }
	for _, bundle := range []contract.SubmissionBundleV2{first, second} {
		if _, err := store.CreateSubmissionBundleV2(context.Background(), bundle); err != nil {
			t.Fatalf("same command id in an isolated scope conflicted: %v", err)
		}
	}
	for _, bundle := range []contract.SubmissionBundleV2{first, second} {
		read, err := store.InspectSubmissionBundleV2(context.Background(), bundle.Command.Target, bundle.Command.ID)
		if err != nil || !runtimeports.SameExecutionScopeV2(read.Command.Target, bundle.Command.Target) {
			t.Fatalf("scope partition returned another tenant: %#v err=%v", read.Command.Target, err)
		}
		digest, _ := runtimeports.ExecutionScopeDigestV2(bundle.Command.Target)
		journalID := "workflow-journal:" + string(digest) + ":" + bundle.Command.ID
		journal, err := contract.NewWorkflowJournalV2(journalID, bundle.Plan, now.UnixNano())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateWorkflowJournalV2(context.Background(), bundle.Plan, journal); err != nil {
			t.Fatalf("same journal command id in isolated scope conflicted: %v", err)
		}
	}
	if _, err := store.InspectSubmissionBundleV2(context.Background(), first.Command.Target, "missing-command"); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("wrong scoped id did not fail closed: %v", err)
	}
}

func FuzzWorkflowPlanV2CustomKindCanonical(f *testing.F) {
	f.Add("custom.eighth/process")
	f.Add("vendor.example/step")
	f.Fuzz(func(t *testing.T, kind string) {
		now := time.Unix(1_800_000_000, 0)
		if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(kind)); err != nil {
			return
		}
		bundle, _ := applicationFixtureV2(t, now, runtimeports.NamespacedNameV2(kind), true)
		if err := bundle.Plan.Validate(now); err != nil {
			return
		}
		first, err := bundle.Plan.DigestV2()
		if err != nil {
			t.Fatal(err)
		}
		second, err := bundle.Plan.DigestV2()
		if err != nil || first != second {
			t.Fatal("custom workflow digest is nondeterministic")
		}
	})
}

type lostMarkOutboxV2 struct {
	runtimeports.ApplicationCommandFactPortV2
	lost atomic.Bool
}

type fixedSubmissionV2 struct{ bundle contract.SubmissionBundleV2 }

func (p fixedSubmissionV2) CreateSubmissionBundleV2(context.Context, contract.SubmissionBundleV2) (contract.SubmissionBundleV2, error) {
	return contract.SubmissionBundleV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidState, "fixed test submission is read-only")
}

func (p fixedSubmissionV2) InspectSubmissionBundleV2(context.Context, core.ExecutionScope, string) (contract.SubmissionBundleV2, error) {
	return p.bundle, nil
}

type lostAcceptCommandV2 struct {
	runtimeports.ApplicationCommandFactPortV2
	lost atomic.Bool
}

func (p *lostAcceptCommandV2) AcceptCommand(ctx context.Context, intent runtimeports.ApplicationCommandIntentV2) (runtimeports.ApplicationCommandAcceptanceV2, error) {
	result, err := p.ApplicationCommandFactPortV2.AcceptCommand(ctx, intent)
	if err != nil {
		return result, err
	}
	if !p.lost.Swap(true) {
		return runtimeports.ApplicationCommandAcceptanceV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected command acceptance reply loss")
	}
	return result, nil
}

func (p *lostMarkOutboxV2) MarkOutboxDispatched(ctx context.Context, scope core.ExecutionScope, id string, revision core.Revision) (runtimeports.ApplicationOutboxRecordV2, error) {
	result, err := p.ApplicationCommandFactPortV2.MarkOutboxDispatched(ctx, scope, id, revision)
	if err != nil {
		return result, err
	}
	if !p.lost.Swap(true) {
		return runtimeports.ApplicationOutboxRecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected outbox reply loss")
	}
	return result, nil
}

func acceptedSubmissionFixtureV2(t *testing.T, now time.Time, bundle contract.SubmissionBundleV2) (*runtimefakes.FactStore, *fakes.FactStoreV2) {
	t.Helper()
	runtimeStore := runtimefakes.NewFactStore(func() time.Time { return now })
	appStore := fakes.NewFactStoreV2()
	appStore.Clock = func() time.Time { return now }
	if _, err := appStore.CreateSubmissionBundleV2(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.CreateDesiredState(context.Background(), runtimeports.DesiredStateSnapshotV2{Scope: bundle.Command.Target, Desired: runtimeports.DesiredRunningV2, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.AcceptCommand(context.Background(), runtimeports.ApplicationCommandIntentV2{Envelope: bundle.Command, Mutation: runtimeports.DesiredStateMutationV2{Desired: runtimeports.DesiredRunningV2}}); err != nil {
		t.Fatal(err)
	}
	return runtimeStore, appStore
}

func applicationFixtureV2(t *testing.T, now time.Time, kind runtimeports.NamespacedNameV2, required bool) (contract.SubmissionBundleV2, applicationports.StepKindDescriptorV2) {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: digest("plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 1}, AuthorityEpoch: 1}
	payloadBytes := []byte(`{"workflow":"custom"}`)
	schema := runtimeports.SchemaRefV2{Namespace: "custom.eighth", Name: "workflow", Version: "1.0.0", MediaType: "application/json", ContentDigest: digest("schema")}
	payload := runtimeports.OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(payloadBytes), Length: uint64(len(payloadBytes)), Inline: payloadBytes, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis/default-limit", Digest: digest("limit")}}
	leaseEpoch := scope.SandboxLease.Epoch
	command := runtimeports.ApplicationCommandEnvelopeV2{ID: "command-1", Kind: runtimeports.ApplicationCommandProvideInputV2, Target: scope, Actor: "user-1", AuthorityRef: "authority-1", CanonicalPayloadDigest: payload.ContentDigest, Preconditions: core.ExecutionPreconditions{IdentityEpoch: 1, InstanceEpoch: 1, LeaseEpoch: &leaseEpoch, AuthorityEpoch: 1, Revision: 1}, IdempotencyKey: "idem-1", SubmittedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}
	payloadFact := contract.CommandPayloadFactV2{ContractVersion: contract.WorkflowContractVersionV2, CommandID: command.ID, Revision: 1, Payload: payload, CreatedUnixNano: now.UnixNano()}
	payloadFactDigest, err := payloadFact.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-1", BindingSetRevision: 1, ComponentID: "custom.eighth/provider", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "custom.eighth/process"}
	domainAdapter := runtimeports.ProviderBindingRefV2{BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, ComponentID: "custom.eighth/domain-adapter", ManifestDigest: digest("domain-adapter-manifest"), ArtifactDigest: digest("domain-adapter-artifact"), Capability: "custom.eighth/domain-state"}
	descriptor := applicationports.StepKindDescriptorV2{Kind: kind, Revision: 1, ExecutionClass: contract.StepGovernedEffectV2, RequiredCapability: provider.Capability, Contract: runtimeports.ContractBindingV2{Name: "custom.eighth/workflow", Version: "1.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: []runtimeports.SchemaRefV2{schema}, IssuedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(2 * time.Hour).UnixNano()}
	descriptorRef, err := descriptor.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	plan := contract.WorkflowPlanV2{ContractVersion: contract.WorkflowContractVersionV2, ID: "workflow-1", Revision: 1, CommandID: command.ID, CommandPayloadDigest: payloadFactDigest, Target: scope, Authority: runtimeports.AuthorityBindingRefV2{Ref: command.AuthorityRef, Digest: digest("authority"), Revision: 1, Epoch: 1}, Steps: []contract.WorkflowStepV2{{ID: "step-a", Kind: kind, Descriptor: descriptorRef, ExecutionClass: contract.StepGovernedEffectV2, Required: required, Dependencies: []string{}, Payload: payload, Provider: &provider, DomainAdapter: &domainAdapter}}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	bundle := contract.SubmissionBundleV2{Command: command, Payload: payloadFact, Plan: plan}
	return bundle, descriptor
}
