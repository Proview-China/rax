package dataplaneadapter

import "testing"

func TestWorkspaceReadNoEffectRequiresCompleteActualPointEvidenceV1(t *testing.T) {
	notCrossed := false
	crossed := true
	zero := uint64(0)
	one := uint64(1)
	tests := []struct {
		name   string
		closed *ClosedError
		want   bool
	}{
		{"complete-no-effect", &ClosedError{EffectBoundary: "effect_not_started", CrossedActualPoint: &notCrossed, PhysicalReadCount: &zero}, true},
		{"missing-evidence", &ClosedError{EffectBoundary: "effect_not_started"}, false},
		{"crossed", &ClosedError{EffectBoundary: "effect_started_unknown", CrossedActualPoint: &crossed, PhysicalReadCount: &one}, false},
		{"contradictory-boundary", &ClosedError{EffectBoundary: "effect_not_started", CrossedActualPoint: &crossed, PhysicalReadCount: &one}, false},
		{"contradictory-count", &ClosedError{EffectBoundary: "effect_not_started", CrossedActualPoint: &notCrossed, PhysicalReadCount: &one}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := workspaceReadNoEffectEvidenceV1(test.closed); got != test.want {
				t.Fatalf("no-effect=%v, want %v", got, test.want)
			}
		})
	}
}
