package owneradapter

import (
	"context"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	definitionconformance "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/conformance"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

type inputsReaderStubV1 struct {
	values []hostcontract.ResolutionInputsCurrentV1
	index  int
}

func (s *inputsReaderStubV1) InspectResolutionInputsCurrentV1(context.Context, string, string) (hostcontract.ResolutionInputsCurrentV1, error) {
	value := s.values[s.index]
	if s.index+1 < len(s.values) {
		s.index++
	}
	return value, nil
}

type assemblerStubV1 struct {
	calls  int
	result assemblercontract.ResolveResultV1
	err    error
}

func (s *assemblerStubV1) Resolve(context.Context, assemblercontract.ResolveRequestV1) (assemblercontract.ResolveResultV1, error) {
	s.calls++
	return s.result, s.err
}

func TestAssemblerAdapterRejectsResolutionInputsS2ABA(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	definition, current, sourceCurrent := definitionFixtureV1(t, now, definitioncontract.DefinitionCurrentActiveV1)
	inputs := resolutionInputsFixtureV1(t, now)
	drift := inputs
	drift.Revision = 2
	drift, _ = hostcontract.SealResolutionInputsCurrentV1(drift)
	ownerAssembler := &assemblerStubV1{}
	adapter := NewAssemblerAdapterV1(&definitionReaderStubV1{definition: definition, currents: []definitioncontract.DefinitionCurrentV1{current, current}}, &sourceReaderStubV1{values: []hostcontract.DefinitionSourceCurrentV1{sourceCurrent, sourceCurrent}}, &inputsReaderStubV1{values: []hostcontract.ResolutionInputsCurrentV1{inputs, drift}}, ownerAssembler, definitionconformance.CatalogV1(), sequenceClockV1(now, now.Add(time.Millisecond), now.Add(2*time.Millisecond)))
	_, err := adapter.ResolveAgentV1(context.Background(), hostConfigV1(), hostcontract.DecodedDefinitionV1{Ref: definitionRefV1(definition.RefV1())})
	if !hostcontract.HasCode(err, hostcontract.ErrorConflict) || ownerAssembler.calls != 1 {
		t.Fatalf("error=%v calls=%d", err, ownerAssembler.calls)
	}
}

func TestAssemblerAdapterRejectsAliasAndFinalTTL(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	definition, current, sourceCurrent := definitionFixtureV1(t, now, definitioncontract.DefinitionCurrentActiveV1)
	inputs := resolutionInputsFixtureV1(t, now)
	alias := inputs
	alias.CatalogStableID = "other-catalog"
	alias, _ = hostcontract.SealResolutionInputsCurrentV1(alias)
	adapter := NewAssemblerAdapterV1(&definitionReaderStubV1{definition: definition, currents: []definitioncontract.DefinitionCurrentV1{current}}, &sourceReaderStubV1{values: []hostcontract.DefinitionSourceCurrentV1{sourceCurrent}}, &inputsReaderStubV1{values: []hostcontract.ResolutionInputsCurrentV1{alias}}, &assemblerStubV1{}, definitionconformance.CatalogV1(), sequenceClockV1(now))
	if _, err := adapter.ResolveAgentV1(context.Background(), hostConfigV1(), hostcontract.DecodedDefinitionV1{Ref: definitionRefV1(definition.RefV1())}); !hostcontract.HasCode(err, hostcontract.ErrorConflict) {
		t.Fatalf("alias error=%v", err)
	}

	short := inputs
	short.ExpiresUnixNano = now.Add(3 * time.Millisecond).UnixNano()
	short, _ = hostcontract.SealResolutionInputsCurrentV1(short)
	adapter = NewAssemblerAdapterV1(&definitionReaderStubV1{definition: definition, currents: []definitioncontract.DefinitionCurrentV1{current, current}}, &sourceReaderStubV1{values: []hostcontract.DefinitionSourceCurrentV1{sourceCurrent, sourceCurrent}}, &inputsReaderStubV1{values: []hostcontract.ResolutionInputsCurrentV1{short, short}}, &assemblerStubV1{}, definitionconformance.CatalogV1(), sequenceClockV1(now, now.Add(time.Millisecond), now.Add(3*time.Millisecond)))
	if _, err := adapter.ResolveAgentV1(context.Background(), hostConfigV1(), hostcontract.DecodedDefinitionV1{Ref: definitionRefV1(definition.RefV1())}); !hostcontract.HasCode(err, hostcontract.ErrorPrecondition) {
		t.Fatalf("TTL error=%v", err)
	}
}

func TestAssemblerAdapterRejectsTypedNilAndNilContext(t *testing.T) {
	var inputs *inputsReaderStubV1
	adapter := NewAssemblerAdapterV1(&definitionReaderStubV1{}, &sourceReaderStubV1{}, inputs, &assemblerStubV1{}, definitionconformance.CatalogV1(), time.Now)
	decoded := hostcontract.DecodedDefinitionV1{Ref: hostcontract.ExactRefV1{Kind: DefinitionKindV1, ID: "definition", Revision: 1, Digest: hostcontract.DigestV1(digestCoreV1("definition"))}}
	if _, err := adapter.ResolveAgentV1(context.Background(), hostConfigV1(), decoded); !hostcontract.HasCode(err, hostcontract.ErrorUnavailable) {
		t.Fatalf("typed nil error=%v", err)
	}
	if _, err := adapter.ResolveAgentV1(nil, hostConfigV1(), decoded); !hostcontract.HasCode(err, hostcontract.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
}

func resolutionInputsFixtureV1(t *testing.T, now time.Time) hostcontract.ResolutionInputsCurrentV1 {
	t.Helper()
	value, err := hostcontract.SealResolutionInputsCurrentV1(hostcontract.ResolutionInputsCurrentV1{ContractVersion: hostcontract.ContractVersionV1, ObjectKind: hostcontract.ResolutionInputsCurrentKindV1, CatalogStableID: hostConfigV1().CatalogRef, ResolutionFactsStableID: hostConfigV1().ResolutionFactsRef, CatalogExactRef: hostcontract.ExactRefV1{Kind: CatalogKindV1, ID: "catalog-exact", Revision: 1, Digest: hostcontract.DigestV1(digestCoreV1("catalog"))}, ResolutionFactsExactRef: hostcontract.ExactRefV1{Kind: FactsKindV1, ID: "facts-exact", Revision: 1, Digest: hostcontract.DigestV1(digestCoreV1("facts"))}, Revision: 1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
