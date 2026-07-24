package application

import (
	"context"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreStageEvidenceAdapterV1LostReplyInspectsOriginalSource(t *testing.T) {
	now := time.Unix(2_020_000_000, 0)
	governance, domain, record, _, err := runtimefakes.BuildRestoreStageSettlementFixtureV1("application-evidence", now)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &restoreStageEvidenceRuntimeFakeV1{record: record, loseReply: true}
	adapter, err := NewRestoreStageEvidenceAdapterV1(runtime, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := applicationcontract.RestoreStageEvidenceRequestV1{RequestDigest: core.DigestBytes([]byte("application-restore-stage-request")), Governance: governance, DomainResult: domain.Fact, SourceRegistration: restoreStageEvidenceSourceRefV1("adapter")}
	ref, err := adapter.PublishRestoreStageEvidenceV1(context.Background(), request)
	if err != nil || ref != record.Ref || runtime.publishCalls != 1 || runtime.inspectCalls != 1 {
		t.Fatalf("lost reply recovery failed: ref=%+v err=%v publish=%d inspect=%d", ref, err, runtime.publishCalls, runtime.inspectCalls)
	}

	var typedNil *restoreStageEvidenceRuntimeFakeV1
	if _, err := NewRestoreStageEvidenceAdapterV1(typedNil, func() time.Time { return now }); err == nil {
		t.Fatal("typed-nil Runtime Restore Evidence port was accepted")
	}
}

type restoreStageEvidenceRuntimeFakeV1 struct {
	record                     runtimeports.EvidenceLedgerRecordV2
	loseReply                  bool
	publishCalls, inspectCalls int
}

func (f *restoreStageEvidenceRuntimeFakeV1) PublishRestoreStageEvidenceV1(_ context.Context, _ runtimeports.PublishRestoreStageEvidenceRequestV1) (runtimeports.EvidenceRecordRefV2, error) {
	f.publishCalls++
	if f.loseReply {
		f.loseReply = false
		return runtimeports.EvidenceRecordRefV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost Runtime Evidence reply")
	}
	return f.record.Ref, nil
}

func (f *restoreStageEvidenceRuntimeFakeV1) InspectRestoreStageEvidenceV1(_ context.Context, _ runtimeports.PublishRestoreStageEvidenceRequestV1) (runtimeports.EvidenceLedgerRecordV2, error) {
	f.inspectCalls++
	return f.record, nil
}
