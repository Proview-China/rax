package assemblyadapter

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestRecoverableCreateReplyV1IsBoundedToUnknownOrRacingOutcomes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		category core.ErrorCategory
		want     bool
	}{
		{core.ErrorUnavailable, true},
		{core.ErrorIndeterminate, true},
		{core.ErrorConflict, true},
		{core.ErrorNotFound, false},
		{core.ErrorInvalidArgument, false},
		{core.ErrorForbidden, false},
	}
	for _, testCase := range cases {
		err := core.NewError(testCase.category, core.ReasonInvalidState, "test")
		if got := recoverableCreateReply(err); got != testCase.want {
			t.Fatalf("category %s: got %v want %v", testCase.category, got, testCase.want)
		}
	}
}
