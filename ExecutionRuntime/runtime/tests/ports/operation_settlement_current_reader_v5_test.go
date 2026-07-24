package ports_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationSettlementCurrentReaderV5IsCapabilityNarrowed(t *testing.T) {
	readerType := reflect.TypeOf((*ports.OperationSettlementCurrentReaderV5)(nil)).Elem()
	if readerType.NumMethod() != 1 {
		t.Fatalf("current Reader exposes %d methods, want exactly one", readerType.NumMethod())
	}
	method, ok := readerType.MethodByName("InspectCheckpointPhaseSettlementCurrentV5")
	if !ok || method.Type.NumIn() != 2 || method.Type.NumOut() != 2 {
		t.Fatalf("current Reader signature drifted: %+v", method)
	}
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	requestType := reflect.TypeOf(ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{})
	inspectionType := reflect.TypeOf(ports.OperationCheckpointRestoreSettlementInspectionV5{})
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if method.Type.In(0) != contextType || method.Type.In(1) != requestType || method.Type.Out(0) != inspectionType || method.Type.Out(1) != errorType {
		t.Fatalf("current Reader parameter/result types drifted: %v", method.Type)
	}
	if _, exposed := readerType.MethodByName("SettleCheckpointPhaseV5"); exposed {
		t.Fatal("capability-narrowed Reader exposes Settle authority")
	}
	providerType := reflect.TypeOf((*ports.OperationSettlementCurrentReaderProviderV5)(nil)).Elem()
	if providerType.NumMethod() != 2 {
		t.Fatalf("current Reader provider exposes %d methods, want Reader plus marker", providerType.NumMethod())
	}
	if _, ok := providerType.MethodByName("GatewayBackedOperationSettlementCurrentReaderV5"); !ok {
		t.Fatal("current Reader provider lost its Gateway-backed wiring marker")
	}
	rawFactType := reflect.TypeOf((*ports.OperationCheckpointRestoreSettlementFactPortV5)(nil)).Elem()
	if rawFactType.Implements(providerType) {
		t.Fatal("raw Settlement Fact Port structurally satisfies Gateway-backed provider")
	}

	governanceType := reflect.TypeOf((*ports.OperationCheckpointRestoreSettlementGovernancePortV5)(nil)).Elem()
	if governanceType.NumMethod() != 6 {
		t.Fatalf("Governance method set changed: %d", governanceType.NumMethod())
	}
	for _, name := range []string{
		"SettleCheckpointPhaseV5",
		"InspectCheckpointPhaseSettlementHistoricalV5",
		"InspectCheckpointPhaseSettlementCurrentV5",
		"InspectCheckpointPhaseSettlementAssociationV5",
		"InspectCheckpointPhaseTerminalGuardV5",
		"InspectCheckpointPhaseTerminalProjectionV5",
	} {
		if _, ok := governanceType.MethodByName(name); !ok {
			t.Fatalf("Governance Port lost %s", name)
		}
	}
}

func TestOperationSettlementCurrentReaderV5ImportBoundary(t *testing.T) {
	allowed := []string{
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/core",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports",
	}
	if err := conformance.CheckAdapterRuntimeImportsV2(allowed); err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckAdapterRuntimeImportsV2([]string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"}); err == nil {
		t.Fatal("Settlement Reader consumer was allowed to import a raw Fact Owner")
	}
}

type operationSettlementCurrentReaderOnlyV5 struct{}

func (operationSettlementCurrentReaderOnlyV5) InspectCheckpointPhaseSettlementCurrentV5(context.Context, ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5) (ports.OperationCheckpointRestoreSettlementInspectionV5, error) {
	return ports.OperationCheckpointRestoreSettlementInspectionV5{}, nil
}

var _ ports.OperationSettlementCurrentReaderV5 = operationSettlementCurrentReaderOnlyV5{}

func TestOperationSettlementCurrentReaderV5PlainReaderCannotSatisfyWiringProvider(t *testing.T) {
	providerType := reflect.TypeOf((*ports.OperationSettlementCurrentReaderProviderV5)(nil)).Elem()
	if reflect.TypeOf(operationSettlementCurrentReaderOnlyV5{}).Implements(providerType) {
		t.Fatal("plain one-method Reader accidentally satisfies wiring provider")
	}
}
