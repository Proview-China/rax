package agentdefinition_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	agentdefinition "github.com/Proview-China/rax/ExecutionRuntime/agent-definition"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/store"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"gopkg.in/yaml.v3"
)

type approvalReaderV1 struct {
	mu     sync.Mutex
	value  ports.ApprovalCurrentV1
	values []ports.ApprovalCurrentV1
	calls  int
	err    error
	after  func(int)
	refs   []contract.ObjectRefV1
}

func (a *approvalReaderV1) InspectApprovalCurrentV1(_ context.Context, ref contract.ObjectRefV1) (ports.ApprovalCurrentV1, error) {
	a.mu.Lock()
	a.calls++
	a.refs = append(a.refs, ref)
	call := a.calls
	value := a.value
	if call <= len(a.values) {
		value = a.values[call-1]
	}
	err := a.err
	after := a.after
	a.mu.Unlock()
	if after != nil {
		after(call)
	}
	return value, err
}

func TestServiceYAMLBlackBoxLostReplyAndIdempotentLaterClockV1(t *testing.T) {
	now := time.Unix(1_800_030_000, 0)
	source := conformance.SourceV1(now)
	catalog := conformance.CatalogV1()
	approvalValue, err := ports.SealApprovalCurrentV1(ports.ApprovalCurrentV1{Ref: source.ApprovalRef, Approved: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	approval := &approvalReaderV1{value: approvalValue}
	repository := store.NewMemoryRepositoryV1(catalog)
	repository.LoseNextCreateReply()
	clockNow := now
	service, err := agentdefinition.NewServiceV1(repository, approval, catalog, func() time.Time { return clockNow })
	if err != nil {
		t.Fatal(err)
	}
	payload, err := yaml.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.CreateYAMLV1(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	clockNow = now.Add(time.Minute)
	second, err := service.CreateYAMLV1(context.Background(), payload)
	if err != nil || first.Definition.Digest != second.Definition.Digest || first.Definition.CreatedUnixNano != second.Definition.CreatedUnixNano {
		t.Fatalf("idempotent replay = %#v %#v %v", first, second, err)
	}
	if approval.calls != 2 {
		t.Fatalf("approval calls=%d, replay must inspect owner fact", approval.calls)
	}
}

func TestServiceJSONAndYAMLConvergeOnSameImmutableDefinitionV1(t *testing.T) {
	now := time.Unix(1_800_030_050, 0)
	source := conformance.SourceV1(now)
	catalog := conformance.CatalogV1()
	approvalValue, err := ports.SealApprovalCurrentV1(ports.ApprovalCurrentV1{Ref: source.ApprovalRef, Approved: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	repository := store.NewMemoryRepositoryV1(catalog)
	service, err := agentdefinition.NewServiceV1(repository, &approvalReaderV1{value: approvalValue}, catalog, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	jsonPayload, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	yamlPayload, err := yaml.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := service.CreateJSONV1(context.Background(), jsonPayload)
	if err != nil {
		t.Fatal(err)
	}
	fromYAML, err := service.CreateYAMLV1(context.Background(), yamlPayload)
	if err != nil {
		t.Fatal(err)
	}
	if fromJSON.Definition.RefV1() != fromYAML.Definition.RefV1() || fromJSON.Definition.SourceDigest != fromYAML.Definition.SourceDigest {
		t.Fatalf("JSON/YAML publication drifted: json=%#v yaml=%#v", fromJSON.Definition.RefV1(), fromYAML.Definition.RefV1())
	}
}

func TestServiceApprovalS2TOCTOUFailClosedZeroWritesV1(t *testing.T) {
	base := time.Unix(1_800_030_150, 0)
	for _, test := range []struct {
		name       string
		configure  func(*approvalReaderV1, *time.Time, ports.ApprovalCurrentV1)
		wantReason core.ReasonCode
	}{
		{
			name: "slow-reader-crosses-ttl",
			configure: func(reader *approvalReaderV1, clockNow *time.Time, _ ports.ApprovalCurrentV1) {
				reader.after = func(call int) {
					if call == 2 {
						*clockNow = base.Add(2 * time.Minute)
					}
				}
			},
			wantReason: core.ReasonReviewVerdictStale,
		},
		{
			name: "s1-s2-drift",
			configure: func(reader *approvalReaderV1, _ *time.Time, first ports.ApprovalCurrentV1) {
				second := first
				second.CheckedUnixNano++
				second, _ = ports.SealApprovalCurrentV1(second)
				reader.values = []ports.ApprovalCurrentV1{first, second}
			},
			wantReason: core.ReasonEvidenceConflict,
		},
		{
			name: "clock-rollback",
			configure: func(reader *approvalReaderV1, clockNow *time.Time, _ ports.ApprovalCurrentV1) {
				reader.after = func(call int) {
					if call == 2 {
						*clockNow = base.Add(-time.Nanosecond)
					}
				}
			},
			wantReason: core.ReasonClockRegression,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := conformance.SourceV1(base)
			catalog := conformance.CatalogV1()
			approvalValue, err := ports.SealApprovalCurrentV1(ports.ApprovalCurrentV1{Ref: source.ApprovalRef, Approved: true, CheckedUnixNano: base.Add(-time.Second).UnixNano(), ExpiresUnixNano: base.Add(time.Minute).UnixNano()})
			if err != nil {
				t.Fatal(err)
			}
			clockNow := base
			reader := &approvalReaderV1{value: approvalValue}
			test.configure(reader, &clockNow, approvalValue)
			repository := store.NewMemoryRepositoryV1(catalog)
			service, err := agentdefinition.NewServiceV1(repository, reader, catalog, func() time.Time { return clockNow })
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.CreateSourceV1(context.Background(), source); !core.HasReason(err, test.wantReason) {
				t.Fatalf("error=%v want=%s", err, test.wantReason)
			}
			if _, err := repository.InspectDefinitionRevisionV1(context.Background(), source.DefinitionID, source.Revision); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("TOCTOU failure wrote a definition: %v", err)
			}
		})
	}
}

func TestServiceDefinitionCreatedAtFreshS2ClockV1(t *testing.T) {
	now1 := time.Unix(1_800_030_175, 0)
	now2 := now1.Add(time.Second)
	clockNow := now1
	source := conformance.SourceV1(now1)
	catalog := conformance.CatalogV1()
	approvalValue, _ := ports.SealApprovalCurrentV1(ports.ApprovalCurrentV1{Ref: source.ApprovalRef, Approved: true, CheckedUnixNano: now1.Add(-time.Second).UnixNano(), ExpiresUnixNano: now1.Add(time.Hour).UnixNano()})
	reader := &approvalReaderV1{value: approvalValue, after: func(call int) {
		if call == 2 {
			clockNow = now2
		}
	}}
	repository := store.NewMemoryRepositoryV1(catalog)
	service, _ := agentdefinition.NewServiceV1(repository, reader, catalog, func() time.Time { return clockNow })
	created, err := service.CreateSourceV1(context.Background(), source)
	if err != nil || created.Definition.CreatedUnixNano != now2.UnixNano() {
		t.Fatalf("definition was not sealed at S2 clock: %#v %v", created.Definition, err)
	}
	if len(reader.refs) != 2 || reader.refs[0] != source.ApprovalRef || reader.refs[1] != source.ApprovalRef {
		t.Fatalf("approval S1/S2 did not inspect the same exact ref: %#v", reader.refs)
	}
}

func TestServiceClockCursorRejectsIntermediateRollbackZeroWritesV1(t *testing.T) {
	base := time.Unix(1_800_030_180, 0)
	source := conformance.SourceV1(base)
	catalog := conformance.CatalogV1()
	approvalValue, err := ports.SealApprovalCurrentV1(ports.ApprovalCurrentV1{Ref: source.ApprovalRef, Approved: true, CheckedUnixNano: base.Add(-time.Second).UnixNano(), ExpiresUnixNano: base.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	times := []time.Time{base, base.Add(5 * time.Second), base.Add(3 * time.Second)}
	var clockMu sync.Mutex
	clockCall := 0
	clock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		observed := times[clockCall]
		clockCall++
		return observed
	}
	repository := store.NewMemoryRepositoryV1(catalog)
	service, err := agentdefinition.NewServiceV1(repository, &approvalReaderV1{value: approvalValue}, catalog, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateSourceV1(context.Background(), source); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("intermediate clock rollback was accepted: %v", err)
	}
	if _, err := repository.InspectDefinitionRevisionV1(context.Background(), source.DefinitionID, source.Revision); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("intermediate clock rollback wrote a definition: %v", err)
	}
}

func TestServiceClockCursorRejectsLostReplyRecoveryRollbackV1(t *testing.T) {
	base := time.Unix(1_800_030_185, 0)
	source := conformance.SourceV1(base)
	catalog := conformance.CatalogV1()
	approvalValue, err := ports.SealApprovalCurrentV1(ports.ApprovalCurrentV1{Ref: source.ApprovalRef, Approved: true, CheckedUnixNano: base.Add(-time.Second).UnixNano(), ExpiresUnixNano: base.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	times := []time.Time{base, base.Add(5 * time.Second), base.Add(6 * time.Second), base.Add(3 * time.Second)}
	clockCall := 0
	clock := func() time.Time {
		observed := times[clockCall]
		clockCall++
		return observed
	}
	repository := store.NewMemoryRepositoryV1(catalog)
	repository.LoseNextCreateReply()
	service, err := agentdefinition.NewServiceV1(repository, &approvalReaderV1{value: approvalValue}, catalog, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateSourceV1(context.Background(), source); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("lost-reply recovery clock rollback was accepted: %v", err)
	}
	if _, err := repository.InspectDefinitionRevisionV1(context.Background(), source.DefinitionID, source.Revision); err != nil {
		t.Fatalf("lost reply did not preserve its already committed immutable definition: %v", err)
	}
}

type driftingCurrentRepositoryV1 struct {
	ports.DefinitionRepositoryV1
}

func (r *driftingCurrentRepositoryV1) InspectCurrentDefinitionV1(ctx context.Context, definitionID string, checked int64) (contract.DefinitionCurrentV1, error) {
	current, err := r.DefinitionRepositoryV1.InspectCurrentDefinitionV1(ctx, definitionID, checked)
	if err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	current.CheckedUnixNano++
	return current, nil
}

func TestServiceRevalidatesCurrentProjectionOnIdempotentReplayV1(t *testing.T) {
	base := time.Unix(1_800_030_190, 0)
	source := conformance.SourceV1(base)
	catalog := conformance.CatalogV1()
	approvalValue, err := ports.SealApprovalCurrentV1(ports.ApprovalCurrentV1{Ref: source.ApprovalRef, Approved: true, CheckedUnixNano: base.Add(-time.Second).UnixNano(), ExpiresUnixNano: base.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	repository := store.NewMemoryRepositoryV1(catalog)
	service, err := agentdefinition.NewServiceV1(repository, &approvalReaderV1{value: approvalValue}, catalog, func() time.Time { return base })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateSourceV1(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	drifting := &driftingCurrentRepositoryV1{DefinitionRepositoryV1: repository}
	service, err = agentdefinition.NewServiceV1(drifting, &approvalReaderV1{value: approvalValue}, catalog, func() time.Time { return base.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateSourceV1(context.Background(), source); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("drifted current projection was accepted: %v", err)
	}
}

func TestServiceExpiredApprovalAndWindowFailClosedZeroWritesV1(t *testing.T) {
	now := time.Unix(1_800_030_100, 0)
	source := conformance.SourceV1(now)
	catalog := conformance.CatalogV1()
	repository := store.NewMemoryRepositoryV1(catalog)
	approvalValue, err := ports.SealApprovalCurrentV1(ports.ApprovalCurrentV1{Ref: source.ApprovalRef, Approved: true, CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	approval := &approvalReaderV1{value: approvalValue}
	service, _ := agentdefinition.NewServiceV1(repository, approval, catalog, func() time.Time { return now })
	if _, err := service.CreateSourceV1(context.Background(), source); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("expired approval = %v", err)
	}
	if _, err := repository.InspectDefinitionRevisionV1(context.Background(), source.DefinitionID, 1); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("expired approval wrote definition: %v", err)
	}
	approval.value.ExpiresUnixNano = now.Add(time.Hour).UnixNano()
	approval.value, _ = ports.SealApprovalCurrentV1(approval.value)
	source.EffectiveWindow.NotAfterUnixNano = now.UnixNano()
	if _, err := service.CreateSourceV1(context.Background(), source); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired window = %v", err)
	}
}

func TestServiceInvalidDefinitionFailsBeforeApprovalAndRepositoryV1(t *testing.T) {
	now := time.Unix(1_800_030_200, 0)
	source := conformance.SourceV1(now)
	source.Components = source.Components[1:]
	catalog := conformance.CatalogV1()
	repository := store.NewMemoryRepositoryV1(catalog)
	approval := &approvalReaderV1{err: core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "must not be called")}
	service, _ := agentdefinition.NewServiceV1(repository, approval, catalog, func() time.Time { return now })
	if _, err := service.CreateSourceV1(context.Background(), source); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("invalid definition = %v", err)
	}
	if approval.calls != 0 {
		t.Fatalf("approval side effect calls=%d", approval.calls)
	}
	if _, err := repository.InspectDefinitionRevisionV1(context.Background(), source.DefinitionID, 1); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("invalid definition wrote repository: %v", err)
	}
}

func TestServiceRejectsTypedNilDependenciesAndFreezesCatalogV1(t *testing.T) {
	now := time.Unix(1_800_030_300, 0)
	catalog := conformance.CatalogV1()
	source := conformance.SourceV1(now)
	approvalValue, err := ports.SealApprovalCurrentV1(ports.ApprovalCurrentV1{Ref: source.ApprovalRef, Approved: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	approval := &approvalReaderV1{value: approvalValue}
	repository := store.NewMemoryRepositoryV1(catalog)
	service, err := agentdefinition.NewServiceV1(repository, approval, catalog, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	catalog.Kinds[0] = "attacker/mutated"
	catalog.Capabilities[0] = "attacker/mutated"
	catalog.RegisteredExtensionKeys[0] = "attacker/mutated"
	if _, err := service.CreateSourceV1(context.Background(), source); err != nil {
		t.Fatalf("service retained caller-owned catalog slices: %v", err)
	}

	var nilRepository *store.MemoryRepositoryV1
	if _, err := agentdefinition.NewServiceV1(nilRepository, approval, conformance.CatalogV1(), func() time.Time { return now }); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil repository accepted: %v", err)
	}
	var nilApproval *approvalReaderV1
	if _, err := agentdefinition.NewServiceV1(repository, nilApproval, conformance.CatalogV1(), func() time.Time { return now }); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil approval reader accepted: %v", err)
	}
	var nilService *agentdefinition.ServiceV1
	if _, err := nilService.CreateSourceV1(context.Background(), source); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("nil service receiver did not fail closed: %v", err)
	}
}
