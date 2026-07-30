package conformance_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

func TestModelInputLineagePublicKindsAndReaderConformanceV1(t *testing.T) {
	if contract.ContextInvocationSourceFrameV1 != "praxis.context/frame" || contract.ContextInvocationSourceModelInputMaterialV1 != "praxis.context/model-input-material" {
		t.Fatalf("public Context invocation kinds drifted")
	}
	var _ contextports.ContextModelInputLineageCurrentReaderV1 = (*kernel.ContextModelInputLineageCurrentReaderV1)(nil)
	var _ contextports.ContextFrameExactCurrentReaderV1 = (*testfixture.ModelInputLineageFrameReaderV1)(nil)
}

func TestModelInputLineageProjectionContainsNoDownstreamAuthorityV1(t *testing.T) {
	typeOf := reflect.TypeOf(contract.ContextModelInputLineageCurrentProjectionV1{})
	for index := 0; index < typeOf.NumField(); index++ {
		name := strings.ToLower(typeOf.Field(index).Name)
		for _, forbidden := range []string{"permit", "provider", "dispatch", "authorization", "evidence", "settlement", "continuation", "outcome"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("lineage projection contains forbidden downstream authority field %q", typeOf.Field(index).Name)
			}
		}
	}
}

func TestModelInputLineageCanceledContextReturnsZeroProjectionV1(t *testing.T) {
	fixture, err := testfixture.NewModelInputLineageFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
	frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, fixture.Frame)
	reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, materials, materials, frames, func() time.Time { return fixture.Now }, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	projection, err := reader.InspectContextModelInputLineageCurrentV1(ctx, fixture.Request)
	if err != context.Canceled || projection != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
		t.Fatalf("cancel conformance: projection=%+v err=%v", projection, err)
	}
}
