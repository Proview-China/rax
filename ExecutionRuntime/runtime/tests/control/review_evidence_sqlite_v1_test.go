package control_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	runtimecontrol "github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/storage/sqlite"
)

func TestReviewEvidenceSQLiteV1PublicGatewayRestartIntegration(t *testing.T) {
	fixture := newReviewEvidenceApplicabilityFixtureV1(t)
	path := filepath.Join(t.TempDir(), "runtime-state-plane.db")
	store, err := sqlite.Open(context.Background(), sqlite.Config{Path: path, BusyTimeout: time.Second, MaxOpenConns: 8, Clock: func() time.Time { return fixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	evidenceGateway, ok := fixture.gateway.Evidence.(kernel.EvidenceSubjectCurrentGatewayV1)
	if !ok {
		t.Fatal("fixture did not expose the Runtime EvidenceSubject public Gateway")
	}
	evidence := fixture.create.Projection.EvidenceSubjectSnapshot
	mutation, err := ports.SealEvidenceSubjectMutationRequestV1(ports.EvidenceSubjectMutationRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: evidence.Projection.Subject, Kind: ports.EvidenceSubjectMutationSourceRegistrationAdvanceV1, Registration: &evidence.Projection.Registration})
	if err != nil {
		t.Fatal(err)
	}
	commit, projection, index, err := runtimecontrol.NewEvidenceSubjectMutationBundleV1(mutation, evidence.Projection, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Ref != evidence.Projection.Ref || !reflect.DeepEqual(index, evidence.CurrentIndex) {
		t.Fatal("SQLite EvidenceSubject seed drifted from the public Gateway snapshot")
	}
	if _, err = store.PublishEvidenceSubjectMutationV1(context.Background(), commit, projection, index); err != nil {
		t.Fatal(err)
	}
	evidenceGateway.Facts = store
	fixture.gateway.Evidence = evidenceGateway
	fixture.gateway.Facts = store
	receipt, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	current, err := fixture.gateway.InspectCurrentReviewEvidenceApplicabilityV1(context.Background(), receipt.Projection)
	if err != nil || current.Projection.Ref != receipt.Projection {
		t.Fatalf("SQLite-backed public Gateway current drifted: %+v %v", current, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(context.Background(), sqlite.Config{Path: path, BusyTimeout: time.Second, MaxOpenConns: 8, Clock: func() time.Time { return fixture.now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	fixture.gateway.Facts = reopened
	evidenceGateway.Facts = reopened
	fixture.gateway.Evidence = evidenceGateway
	afterRestart, err := fixture.gateway.InspectCurrentReviewEvidenceApplicabilityV1(context.Background(), receipt.Projection)
	if err != nil || !reflect.DeepEqual(afterRestart, current) {
		t.Fatalf("SQLite restart changed the exact public Gateway snapshot: %+v %v", afterRestart, err)
	}
}
