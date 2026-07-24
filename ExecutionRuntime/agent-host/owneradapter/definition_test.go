package owneradapter

import (
	"context"
	"testing"
	"time"

	definitionconformance "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/conformance"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	definitionports "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

type definitionReaderStubV1 struct {
	definition definitioncontract.AgentDefinitionV1
	currents   []definitioncontract.DefinitionCurrentV1
	index      int
}

func (s *definitionReaderStubV1) InspectExactDefinitionV1(context.Context, definitioncontract.AgentDefinitionRefV1) (definitioncontract.AgentDefinitionV1, error) {
	return s.definition, nil
}
func (s *definitionReaderStubV1) InspectCurrentDefinitionV1(context.Context, string, int64) (definitioncontract.DefinitionCurrentV1, error) {
	value := s.currents[s.index]
	if s.index+1 < len(s.currents) {
		s.index++
	}
	return value, nil
}

type sourceReaderStubV1 struct {
	values []hostcontract.DefinitionSourceCurrentV1
	index  int
}

func (s *sourceReaderStubV1) InspectDefinitionSourceCurrentV1(context.Context, string) (hostcontract.DefinitionSourceCurrentV1, error) {
	value := s.values[s.index]
	if s.index+1 < len(s.values) {
		s.index++
	}
	return value, nil
}

func TestDefinitionAdapterUsesDualCurrentChainsWithoutIDTypePun(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	definition, current, sourceCurrent := definitionFixtureV1(t, now, definitioncontract.DefinitionCurrentActiveV1)
	reader := &definitionReaderStubV1{definition: definition, currents: []definitioncontract.DefinitionCurrentV1{current, current}}
	sources := &sourceReaderStubV1{values: []hostcontract.DefinitionSourceCurrentV1{sourceCurrent, sourceCurrent}}
	adapter := NewDefinitionAdapterV1(reader, sources, definitionconformance.CatalogV1(), sequenceClockV1(now, now.Add(time.Millisecond), now.Add(2*time.Millisecond)))
	result, err := adapter.DecodeDefinitionV1(context.Background(), hostConfigV1())
	if err != nil {
		t.Fatal(err)
	}
	if result.Ref.ID != definition.DefinitionID || hostConfigV1().DefinitionSourceRef == definition.DefinitionID {
		t.Fatalf("result=%+v source=%s", result, hostConfigV1().DefinitionSourceRef)
	}
}

func TestDefinitionAdapterRejectsRevokedOwnerCurrent(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	definition, current, sourceCurrent := definitionFixtureV1(t, now, definitioncontract.DefinitionCurrentRevokedV1)
	adapter := NewDefinitionAdapterV1(&definitionReaderStubV1{definition: definition, currents: []definitioncontract.DefinitionCurrentV1{current}}, &sourceReaderStubV1{values: []hostcontract.DefinitionSourceCurrentV1{sourceCurrent}}, definitionconformance.CatalogV1(), sequenceClockV1(now))
	_, err := adapter.DecodeDefinitionV1(context.Background(), hostConfigV1())
	if !hostcontract.HasCode(err, hostcontract.ErrorPrecondition) {
		t.Fatalf("error=%v", err)
	}
}

func TestDefinitionAdapterRejectsSourceS2ABAAndFinalTTL(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	definition, current, sourceCurrent := definitionFixtureV1(t, now, definitioncontract.DefinitionCurrentActiveV1)
	drift := sourceCurrent
	drift.Revision = 2
	drift, _ = hostcontract.SealDefinitionSourceCurrentV1(drift)
	adapter := NewDefinitionAdapterV1(&definitionReaderStubV1{definition: definition, currents: []definitioncontract.DefinitionCurrentV1{current, current}}, &sourceReaderStubV1{values: []hostcontract.DefinitionSourceCurrentV1{sourceCurrent, drift}}, definitionconformance.CatalogV1(), sequenceClockV1(now, now.Add(time.Millisecond)))
	if _, err := adapter.DecodeDefinitionV1(context.Background(), hostConfigV1()); !hostcontract.HasCode(err, hostcontract.ErrorConflict) {
		t.Fatalf("ABA error=%v", err)
	}

	short := sourceCurrent
	short.ExpiresUnixNano = now.Add(3 * time.Millisecond).UnixNano()
	short, _ = hostcontract.SealDefinitionSourceCurrentV1(short)
	adapter = NewDefinitionAdapterV1(&definitionReaderStubV1{definition: definition, currents: []definitioncontract.DefinitionCurrentV1{current, current}}, &sourceReaderStubV1{values: []hostcontract.DefinitionSourceCurrentV1{short, short}}, definitionconformance.CatalogV1(), sequenceClockV1(now, now.Add(time.Millisecond), now.Add(3*time.Millisecond)))
	if _, err := adapter.DecodeDefinitionV1(context.Background(), hostConfigV1()); !hostcontract.HasCode(err, hostcontract.ErrorPrecondition) {
		t.Fatalf("TTL error=%v", err)
	}
}

func TestDefinitionAdapterRejectsClockRollbackTypedNilAndNilContext(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	definition, current, sourceCurrent := definitionFixtureV1(t, now, definitioncontract.DefinitionCurrentActiveV1)
	adapter := NewDefinitionAdapterV1(&definitionReaderStubV1{definition: definition, currents: []definitioncontract.DefinitionCurrentV1{current, current}}, &sourceReaderStubV1{values: []hostcontract.DefinitionSourceCurrentV1{sourceCurrent, sourceCurrent}}, definitionconformance.CatalogV1(), sequenceClockV1(now, now.Add(-time.Millisecond)))
	if _, err := adapter.DecodeDefinitionV1(context.Background(), hostConfigV1()); !hostcontract.HasCode(err, hostcontract.ErrorPrecondition) {
		t.Fatalf("rollback error=%v", err)
	}
	var nilSource *sourceReaderStubV1
	adapter = NewDefinitionAdapterV1(&definitionReaderStubV1{}, nilSource, definitionconformance.CatalogV1(), sequenceClockV1(now))
	if _, err := adapter.DecodeDefinitionV1(context.Background(), hostConfigV1()); !hostcontract.HasCode(err, hostcontract.ErrorUnavailable) {
		t.Fatalf("typed nil error=%v", err)
	}
	if _, err := adapter.DecodeDefinitionV1(nil, hostConfigV1()); !hostcontract.HasCode(err, hostcontract.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
}

func definitionFixtureV1(t *testing.T, now time.Time, state definitioncontract.DefinitionCurrentStateV1) (definitioncontract.AgentDefinitionV1, definitioncontract.DefinitionCurrentV1, hostcontract.DefinitionSourceCurrentV1) {
	t.Helper()
	source := definitionconformance.SourceV1(now)
	definition, err := definitioncontract.SealDefinitionV1(source, definitionconformance.CatalogV1(), now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	current, err := definitioncontract.SealDefinitionCurrentV1(definitioncontract.DefinitionCurrentV1{Definition: definition.RefV1(), State: state, Revision: 1, UpdatedUnixNano: now.UnixNano(), CheckedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	sourceCurrent, err := hostcontract.SealDefinitionSourceCurrentV1(hostcontract.DefinitionSourceCurrentV1{ContractVersion: hostcontract.ContractVersionV1, ObjectKind: hostcontract.DefinitionSourceCurrentKindV1, SourceStableID: hostConfigV1().DefinitionSourceRef, DefinitionExactRef: definitionRefV1(definition.RefV1()), Revision: 1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return definition, current, sourceCurrent
}

func sequenceClockV1(values ...time.Time) func() time.Time {
	index := 0
	return func() time.Time {
		value := values[index]
		if index+1 < len(values) {
			index++
		}
		return value
	}
}

func hostConfigV1() hostcontract.HostConfigV1 {
	return hostcontract.HostConfigV1{ContractVersion: hostcontract.ContractVersionV1, HostID: "host-1", DefinitionSourceRef: "definition-source-stable", StatePlaneBindings: []string{"state-1"}, ProviderEndpointRefs: []string{"provider-1"}, SecretBrokerRef: "secret-1", CatalogRef: "catalog-current", ResolutionFactsRef: "facts-current", RuntimeServiceRefs: []string{"runtime-1"}, ListenRef: "listen-1", DiagnosticsPolicyRef: "diagnostics-1"}
}

var _ definitionports.DefinitionCurrentReaderV1 = (*definitionReaderStubV1)(nil)
