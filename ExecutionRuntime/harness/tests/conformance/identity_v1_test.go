package conformance_test

import (
	"reflect"
	"testing"

	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSingleCallToolActionAssemblerV2ImplementsOnlyNarrowApplicationReadCapability(t *testing.T) {
	var inputCurrent applicationports.SingleCallToolActionInputCurrentReaderV2 = (*applicationadapter.SingleCallToolActionAssemblerV2)(nil)
	_ = inputCurrent
	constructor := reflect.TypeOf(applicationadapter.NewSingleCallToolActionAssemblerV2)
	want := []reflect.Type{
		reflect.TypeOf((*harnessports.SessionCurrentReaderV4)(nil)).Elem(),
		reflect.TypeOf((*harnessports.CommittedPendingActionReaderV3)(nil)).Elem(),
		reflect.TypeOf((*harnessports.SettledTurnDomainResultReaderV3)(nil)).Elem(),
		reflect.TypeOf((*modelinvoker.ToolCallCandidateObservationProjectionReaderV1)(nil)).Elem(),
		reflect.TypeOf((*runtimeports.AuthorityFactReaderV2)(nil)).Elem(),
	}
	if constructor.NumIn() != 6 {
		t.Fatalf("P3 assembler dependency count=%d", constructor.NumIn())
	}
	for index, expected := range want {
		if constructor.In(index) != expected {
			t.Fatalf("P3 assembler dependency %d=%v want=%v", index, constructor.In(index), expected)
		}
	}
	for _, forbidden := range []reflect.Type{
		reflect.TypeOf((*harnessports.SessionFactPortV4)(nil)).Elem(),
		reflect.TypeOf((*harnessports.SettledTurnDomainResultRepositoryV3)(nil)).Elem(),
		treflectTypeOfApplicationToolPortV2(),
	} {
		for index := 0; index < constructor.NumIn(); index++ {
			if constructor.In(index) == forbidden {
				t.Fatalf("P3 assembler received forbidden write capability %v", forbidden)
			}
		}
	}
}

func treflectTypeOfApplicationToolPortV2() reflect.Type {
	return reflect.TypeOf((*applicationports.SingleCallToolActionPortV2)(nil)).Elem()
}

func TestIdentityOwnerPortsExposeOneRepositoryAndNarrowReaders(t *testing.T) {
	var repository harnessports.SettledTurnDomainResultRepositoryV3 = fakes.NewSettledTurnDomainResultRepositoryV3()
	var reader harnessports.SettledTurnDomainResultReaderV3 = repository
	var sessions harnessports.SessionFactPortV3 = fakes.NewGovernedStoreV2()
	if repository == nil || reader == nil || sessions == nil {
		t.Fatal("public owner ports are unavailable")
	}
	readerType := reflect.TypeOf((*harnessports.CommittedPendingActionReaderV2)(nil)).Elem()
	if readerType.NumMethod() != 1 || readerType.Method(0).Name != "InspectCommittedPendingActionCurrentV2" {
		t.Fatalf("Current V2 reader leaked a write or weak lookup: %v", readerType)
	}
}

func TestCurrentV3UsesDistinctTypesAndRuntimeNarrowSettlementReader(t *testing.T) {
	constructor := reflect.TypeOf(kernel.NewCommittedPendingActionReaderV3)
	narrow := reflect.TypeOf((*runtimeports.OperationSettlementCurrentReaderV3)(nil)).Elem()
	governance := reflect.TypeOf((*runtimeports.OperationSettlementGovernancePortV3)(nil)).Elem()
	if constructor.In(4) != narrow || constructor.In(4) == governance {
		t.Fatalf("Current V3 constructor Settlement dependency = %v", constructor.In(4))
	}
	v2 := reflect.TypeOf((*harnessports.CommittedPendingActionReaderV2)(nil)).Elem().Method(0)
	v3 := reflect.TypeOf((*harnessports.CommittedPendingActionReaderV3)(nil)).Elem().Method(0)
	if v2.Type.Out(0) != reflect.TypeOf(contract.CommittedPendingActionCurrentV2{}) || v3.Type.Out(0) != reflect.TypeOf(contract.CommittedPendingActionCurrentV3{}) {
		t.Fatalf("Reader V2/V3 return types were aliased: %v / %v", v2.Type, v3.Type)
	}
	for _, field := range []reflect.Type{reflect.TypeOf(contract.CommittedPendingActionCurrentV2{}), reflect.TypeOf(contract.GovernedSessionV3{})} {
		for index := 0; index < field.NumField(); index++ {
			if field.Field(index).Type == reflect.TypeOf(contract.PendingActionApplicationBindingV2{}) || field.Field(index).Type == reflect.TypeOf((*contract.PendingActionApplicationBindingV2)(nil)) {
				t.Fatalf("old type %v carries Binding V2", field)
			}
		}
	}
}

func TestSessionCurrentReaderV4IsNarrowAndFactPortCompatible(t *testing.T) {
	reader := reflect.TypeOf((*harnessports.SessionCurrentReaderV4)(nil)).Elem()
	factPort := reflect.TypeOf((*harnessports.SessionFactPortV4)(nil)).Elem()
	if reader.NumMethod() != 1 || reader.Method(0).Name != "InspectSessionV4" {
		t.Fatalf("Session Current Reader V4 leaked write capability: %v", reader)
	}
	if !factPort.Implements(reader) {
		t.Fatalf("SessionFactPortV4 no longer satisfies SessionCurrentReaderV4: %v", factPort)
	}
	var current harnessports.SessionCurrentReaderV4 = fakes.NewGovernedStoreV2()
	var facts harnessports.SessionFactPortV4 = fakes.NewGovernedStoreV2()
	if current == nil || facts == nil {
		t.Fatal("Session V4 public capabilities are unavailable")
	}
	currentType := reflect.TypeOf((*harnessports.SessionCurrentReaderV4)(nil)).Elem()
	for _, write := range []string{"CreateSessionV4", "CompareAndSwapSessionV4"} {
		if _, ok := currentType.MethodByName(write); ok {
			t.Fatalf("P3 narrow Session capability can call %s", write)
		}
	}
}

func TestIdentityContractsDoNotExposeModelWriteCapabilities(t *testing.T) {
	for _, value := range []any{
		contract.ModelToolCallPendingActionIdentityV1{},
		contract.SettledTurnDomainResultFactV3{},
		contract.CommittedPendingActionCurrentV2{},
	} {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			field := typeOf.Field(index)
			if field.Type == reflect.TypeOf((*modelinvoker.ToolCallCandidateObservationProjectionPublisherV1)(nil)).Elem() || field.Type == reflect.TypeOf((*modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1)(nil)).Elem() {
				t.Fatalf("%s exposes Model write capability %s", typeOf, field.Name)
			}
		}
	}
}
