package kernel

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
)

func TestWorkspaceReadNoEffectRequiresCompleteActualPointEvidenceV1(t *testing.T) {
	notCrossed := false
	crossed := true
	zero := uint64(0)
	one := uint64(1)
	tests := []struct {
		name   string
		closed *dataplaneadapter.ClosedError
		want   bool
	}{
		{"complete-no-effect", &dataplaneadapter.ClosedError{EffectBoundary: "effect_not_started", CrossedActualPoint: &notCrossed, PhysicalReadCount: &zero}, true},
		{"missing-evidence", &dataplaneadapter.ClosedError{EffectBoundary: "effect_not_started"}, false},
		{"crossed", &dataplaneadapter.ClosedError{EffectBoundary: "effect_started_unknown", CrossedActualPoint: &crossed, PhysicalReadCount: &one}, false},
		{"contradictory-boundary", &dataplaneadapter.ClosedError{EffectBoundary: "effect_not_started", CrossedActualPoint: &crossed, PhysicalReadCount: &one}, false},
		{"contradictory-count", &dataplaneadapter.ClosedError{EffectBoundary: "effect_not_started", CrossedActualPoint: &notCrossed, PhysicalReadCount: &one}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := workspaceReadNoEffectEvidenceV1(test.closed); got != test.want {
				t.Fatalf("no-effect=%v, want %v", got, test.want)
			}
		})
	}
}
