package control_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestOperationSettlementV4BuilderDerivesExactDeterministicClosure(t *testing.T) {
	submission := testsupport.OperationSettlementSubmissionV4()
	left, err := control.BuildOperationSettlementCommitBundleV4(submission)
	if err != nil {
		t.Fatal(err)
	}
	right, err := control.BuildOperationSettlementCommitBundleV4(submission)
	if err != nil {
		t.Fatal(err)
	}
	leftDigest, err := control.OperationSettlementCommitBundleDigestV4(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := control.OperationSettlementCommitBundleDigestV4(right)
	if err != nil || leftDigest != rightDigest || left.Validate() != nil {
		t.Fatalf("V4 four-object builder was not deterministic and exact: left=%s right=%s err=%v", leftDigest, rightDigest, err)
	}
}

func TestOperationSettlementV4BuilderClosureRejectsTypedRefDrift(t *testing.T) {
	bundle, err := control.BuildOperationSettlementCommitBundleV4(testsupport.OperationSettlementSubmissionV4())
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(){
		"association": func() { bundle.Projection.Association.ID = "another-association" },
		"guard":       func() { bundle.Projection.Guard.Digest = core.DigestBytes([]byte("another-guard")) },
		"projection":  func() { bundle.Projection.Settlement.Digest = core.DigestBytes([]byte("another-settlement")) },
	} {
		t.Run(name, func(t *testing.T) {
			value, err := control.BuildOperationSettlementCommitBundleV4(testsupport.OperationSettlementSubmissionV4())
			if err != nil {
				t.Fatal(err)
			}
			bundle = value
			mutate()
			if err := bundle.Validate(); err == nil {
				t.Fatal("typed ref drift remained a valid four-object closure")
			}
		})
	}
}
