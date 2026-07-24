package domain

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestWhiteboxCheckpointManifestImmutableIdentity(t *testing.T) {
	current := testkit.ManifestV2(contract.ManifestCollecting, 1)
	next := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	if err := validateManifestIdentityV2(current, next); err != nil {
		t.Fatalf("valid identity advance rejected: %v", err)
	}
	mutations := map[string]func(*contract.CheckpointManifestFactV2){
		"attempt":    func(value *contract.CheckpointManifestFactV2) { value.CheckpointAttemptRef.ID = "other-attempt" },
		"barrier":    func(value *contract.CheckpointManifestFactV2) { value.BarrierRef.Revision++ },
		"effect cut": func(value *contract.CheckpointManifestFactV2) { value.EffectCutRef.Digest = "other-effect-cut-digest" },
		"scope":      func(value *contract.CheckpointManifestFactV2) { value.Scope.InstanceEpoch++ },
		"context frame": func(value *contract.CheckpointManifestFactV2) {
			value.ContextFrameRefs[0].Digest = "other-context-frame-digest"
		},
		"required set": func(value *contract.CheckpointManifestFactV2) {
			value.RequiredParticipantSetDigest = "other-required-set"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := next.Clone()
			mutate(&changed)
			if err := validateManifestIdentityV2(current, changed); !contract.HasCode(err, contract.ErrRevisionConflict) {
				t.Fatalf("immutable identity drift accepted: %v", err)
			}
		})
	}
}

func TestWhiteboxTypedNilDetection(t *testing.T) {
	var backend *memory.Backend
	if !nilInterfaceV2(backend) || !nilInterfaceV2(nil) || nilInterfaceV2(memory.New()) {
		t.Fatal("typed-nil detection is not fail closed")
	}
}
