package ports

import (
	"context"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

type inspectOnlySessionV4Reader struct{}

func (inspectOnlySessionV4Reader) InspectSessionV4(context.Context, contract.RunRef, string) (contract.GovernedSessionV4, error) {
	return contract.GovernedSessionV4{}, nil
}

var _ SessionCurrentReaderV4 = inspectOnlySessionV4Reader{}

func TestSessionCurrentReaderV4ExposesOnlyInspect(t *testing.T) {
	t.Parallel()

	reader := reflect.TypeOf((*SessionCurrentReaderV4)(nil)).Elem()
	if reader.NumMethod() != 1 || reader.Method(0).Name != "InspectSessionV4" {
		t.Fatalf("SessionCurrentReaderV4 method set = %v", reader)
	}
	if _, ok := reader.MethodByName("CreateSessionV4"); ok {
		t.Fatal("SessionCurrentReaderV4 exposes CreateSessionV4")
	}
	if _, ok := reader.MethodByName("CompareAndSwapSessionV4"); ok {
		t.Fatal("SessionCurrentReaderV4 exposes CompareAndSwapSessionV4")
	}
}
