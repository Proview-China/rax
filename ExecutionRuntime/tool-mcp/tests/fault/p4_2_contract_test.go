package fault_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestFaultP42HistoricalAndExactRefTamperFailClosed(t *testing.T) {
	projection := testkit.ModelProjection(1)
	historical, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	historical.CanonicalArgumentsDigest = testkit.Digest("argument-splice")
	if err = historical.Validate(); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("historical argument splice did not fail closed: %v", err)
	}
	ref := toolcontract.SingleCallToolActionBindingCurrentRefV2{ID: "single-call-tool-binding-v2-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: 1, Digest: testkit.Digest("binding-v2")}
	if err = ref.Validate(); err != nil {
		t.Fatal(err)
	}
	ref.Revision = 2
	if err = ref.Validate(); err == nil {
		t.Fatal("BindingV2 successor revision was accepted for create-once V2 root")
	}
}
