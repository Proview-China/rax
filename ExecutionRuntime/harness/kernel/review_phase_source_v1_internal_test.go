package kernel

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

type reviewPhaseNilStubV1 struct{}

func (*reviewPhaseNilStubV1) InspectCommittedPendingActionCurrentV3(context.Context, contract.CommittedPendingActionCurrentRequestV3) (contract.CommittedPendingActionCurrentV3, error) {
	return contract.CommittedPendingActionCurrentV3{}, nil
}
func (*reviewPhaseNilStubV1) InspectSessionV4(context.Context, contract.RunRef, string) (contract.GovernedSessionV4, error) {
	return contract.GovernedSessionV4{}, nil
}

func TestNewReviewPhaseSourceCurrentReaderV1RejectsTypedNilDependencies(t *testing.T) {
	valid := &reviewPhaseNilStubV1{}
	var typedNil *reviewPhaseNilStubV1
	clock := func() time.Time { return time.Unix(1_760_000_000, 0) }
	for name, call := range map[string]func() error{
		"actions":  func() error { _, err := NewReviewPhaseSourceCurrentReaderV1(typedNil, valid, clock); return err },
		"sessions": func() error { _, err := NewReviewPhaseSourceCurrentReaderV1(valid, typedNil, clock); return err },
		"clock":    func() error { _, err := NewReviewPhaseSourceCurrentReaderV1(valid, valid, nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("constructor accepted missing dependency")
			}
		})
	}
}
