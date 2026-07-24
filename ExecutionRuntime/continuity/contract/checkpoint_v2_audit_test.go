package contract_test

import (
	"strconv"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestAuditExactFactIdentityKeyIsDelimiterSafeAndBindsFullOwner(t *testing.T) {
	left := testkit.ExactRefV2("fact-1", "praxis/runtime", "checkpoint-fact")
	left.TenantID = "tenant|scope"
	left.ScopeDigest = "digest"
	right := left
	right.TenantID = "tenant"
	right.ScopeDigest = "scope|digest"
	legacyKey := func(ref contract.ExactFactRefV2) string {
		return ref.TenantID + "|" + ref.ScopeDigest + "|" + ref.ContractVersion + "|" + ref.SchemaRef + "|" + ref.Owner.ComponentID + "|" + ref.ID + "|" + strconv.FormatUint(ref.Revision, 10) + "|" + ref.Digest
	}
	if legacyKey(left) != legacyKey(right) {
		t.Fatal("delimiter collision fixture no longer demonstrates the legacy ambiguity")
	}
	if left.IdentityKey() == right.IdentityKey() {
		t.Fatal("structural identity key aliased delimiter-bearing tenant/scope values")
	}
	if _, err := contract.ExactRefSetDigestV2([]contract.ExactFactRefV2{left, right}); err != nil {
		t.Fatalf("delimiter-distinct refs were treated as duplicates: %v", err)
	}

	base := testkit.ExactRefV2("fact-owner", "praxis/runtime", "checkpoint-fact")
	mutations := map[string]func(*contract.OwnerBinding){
		"binding set":      func(owner *contract.OwnerBinding) { owner.BindingSetID += "|drift" },
		"binding revision": func(owner *contract.OwnerBinding) { owner.BindingRevision++ },
		"component":        func(owner *contract.OwnerBinding) { owner.ComponentID += "|drift" },
		"manifest digest":  func(owner *contract.OwnerBinding) { owner.ManifestDigest += "|drift" },
		"artifact digest":  func(owner *contract.OwnerBinding) { owner.ArtifactDigest += "|drift" },
		"capability":       func(owner *contract.OwnerBinding) { owner.Capability += "|drift" },
		"fact kind":        func(owner *contract.OwnerBinding) { owner.FactKind += "|drift" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			drifted := base
			mutate(&drifted.Owner)
			if base.IdentityKey() == drifted.IdentityKey() {
				t.Fatalf("OwnerBinding %s drift did not change exact identity", name)
			}
		})
	}
}

func TestAuditManifestV2RejectsRecursiveCrossTenantAndScopeSplice(t *testing.T) {
	base := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	diagnosticSubject := testkit.ExactRefV2("diagnostic-subject-1", "praxis/runtime", "checkpoint-subject")
	diagnosticInspection := testkit.ExactRefV2("diagnostic-inspection-1", "praxis/runtime", "checkpoint-inspection")
	base.Diagnostics = []contract.CheckpointManifestDiagnosticV2{{
		DiagnosticID: "diagnostic-info-1", DiagnosticRef: testkit.ExactRefV2("diagnostic-info-1", "praxis/continuity", "checkpoint-diagnostic"),
		Code: "policy_note", Severity: contract.ManifestDiagnosticInfoV2,
		SubjectRef: &diagnosticSubject, InspectionRef: &diagnosticInspection,
	}}
	testkit.RefreshManifestV2(&base)
	if err := base.Validate(); err != nil {
		t.Fatalf("valid nonblocking diagnostic manifest rejected: %v", err)
	}

	tests := map[string]func(*contract.CheckpointManifestFactV2){
		"context frame tenant": func(value *contract.CheckpointManifestFactV2) { value.ContextFrameRefs[0].TenantID = "tenant-2" },
		"memory scope":         func(value *contract.CheckpointManifestFactV2) { value.MemoryRefs[0].ScopeDigest = "scope-other" },
		"knowledge tenant":     func(value *contract.CheckpointManifestFactV2) { value.KnowledgeRefs[0].TenantID = "tenant-2" },
		"attempt scope": func(value *contract.CheckpointManifestFactV2) {
			value.AttemptSettlementClosures[0].AttemptRef.ScopeDigest = "scope-other"
		},
		"settlement tenant": func(value *contract.CheckpointManifestFactV2) {
			value.AttemptSettlementClosures[0].SettlementRef.TenantID = "tenant-2"
		},
		"participant fact": func(value *contract.CheckpointManifestFactV2) {
			value.ParticipantClosures[0].ParticipantFactRef.ScopeDigest = "scope-other"
		},
		"runtime closure tenant": func(value *contract.CheckpointManifestFactV2) {
			value.ParticipantClosures[0].RuntimeClosureRef.TenantID = "tenant-2"
		},
		"snapshot tenant": func(value *contract.CheckpointManifestFactV2) {
			value.ParticipantClosures[0].SnapshotRef.TenantID = "tenant-2"
		},
		"coverage scope": func(value *contract.CheckpointManifestFactV2) {
			value.ParticipantClosures[0].CoverageRef.ScopeDigest = "scope-other"
		},
		"evidence tenant": func(value *contract.CheckpointManifestFactV2) {
			value.ParticipantClosures[0].EvidenceRefs[0].TenantID = "tenant-2"
		},
		"diagnostic fact": func(value *contract.CheckpointManifestFactV2) {
			value.Diagnostics[0].DiagnosticRef.ScopeDigest = "scope-other"
		},
		"diagnostic subject": func(value *contract.CheckpointManifestFactV2) { value.Diagnostics[0].SubjectRef.TenantID = "tenant-2" },
		"diagnostic inspect": func(value *contract.CheckpointManifestFactV2) {
			value.Diagnostics[0].InspectionRef.ScopeDigest = "scope-other"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			spliced := base.Clone()
			mutate(&spliced)
			testkit.RefreshManifestV2(&spliced)
			if err := spliced.Validate(); !contract.HasCode(err, contract.ErrRevisionConflict) {
				t.Fatalf("cross-tenant/scope splice accepted: %v", err)
			}
		})
	}
}

func TestAuditVerifiedManifestAggregatesEveryNestedResidual(t *testing.T) {
	tests := map[string]func(*contract.CheckpointManifestFactV2){
		"settled attempt residual": func(value *contract.CheckpointManifestFactV2) {
			value.AttemptSettlementClosures[0].ResidualRefs = []contract.ExactFactRefV2{testkit.ResidualV2("residual-settled", "settled_attempt", "settlement")}
		},
		"participant residual": func(value *contract.CheckpointManifestFactV2) {
			value.ParticipantClosures[0].ResidualRefs = []contract.ExactFactRefV2{testkit.ResidualV2("residual-participant", "participant", "participant")}
		},
		"nonblocking diagnostic residual": func(value *contract.CheckpointManifestFactV2) {
			value.Diagnostics = []contract.CheckpointManifestDiagnosticV2{{
				DiagnosticID: "diagnostic-warning", DiagnosticRef: testkit.ExactRefV2("diagnostic-warning", "praxis/continuity", "checkpoint-diagnostic"),
				Code: "warning", Severity: contract.ManifestDiagnosticWarningV2,
				ResidualRefs: []contract.ExactFactRefV2{testkit.ResidualV2("residual-warning", "warning", "warning")},
			}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			manifest := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
			mutate(&manifest)
			testkit.RefreshManifestV2(&manifest)
			if err := manifest.Validate(); !contract.HasCode(err, contract.ErrCheckpointIndeterminate) {
				t.Fatalf("nested residual accepted by verified manifest: %v", err)
			}
		})
	}
}

func TestAuditSealRejectsRecursiveSpliceAndResidual(t *testing.T) {
	manifest := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	base := testkit.SealV2(manifest)
	tests := map[string]func(*contract.CheckpointManifestSealFactV2){
		"manifest tenant": func(value *contract.CheckpointManifestSealFactV2) {
			ref := value.ManifestRef.Exact()
			ref.TenantID = "tenant-2"
			value.ManifestRef = contract.CheckpointManifestRefV2(ref)
		},
		"attempt scope": func(value *contract.CheckpointManifestSealFactV2) {
			value.CheckpointAttemptRef.ScopeDigest = "scope-other"
		},
		"barrier tenant":   func(value *contract.CheckpointManifestSealFactV2) { value.BarrierRef.TenantID = "tenant-2" },
		"effect cut scope": func(value *contract.CheckpointManifestSealFactV2) { value.EffectCutRef.ScopeDigest = "scope-other" },
		"participant evidence": func(value *contract.CheckpointManifestSealFactV2) {
			value.ParticipantClosures[0].EvidenceRefs[0].TenantID = "tenant-2"
		},
		"runtime closure scope": func(value *contract.CheckpointManifestSealFactV2) {
			value.ParticipantClosures[0].RuntimeClosureRef.ScopeDigest = "scope-other"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			seal := base.Clone()
			mutate(&seal)
			testkit.RefreshSealV2(&seal)
			if err := seal.Validate(); !contract.HasCode(err, contract.ErrRevisionConflict) {
				t.Fatalf("seal splice accepted: %v", err)
			}
		})
	}

	withResidual := base.Clone()
	withResidual.ParticipantClosures[0].ResidualRefs = []contract.ExactFactRefV2{testkit.ResidualV2("seal-residual", "participant", "participant")}
	testkit.RefreshSealV2(&withResidual)
	if err := withResidual.Validate(); !contract.HasCode(err, contract.ErrCheckpointIndeterminate) {
		t.Fatalf("seal participant residual accepted: %v", err)
	}
}

func TestAuditSealBindingFreezesRuntimeSetContextAndArtifactClosures(t *testing.T) {
	manifest := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	base := testkit.SealV2(manifest)
	tests := map[string]func(*contract.CheckpointManifestSealFactV2){
		"runtime participant set": func(value *contract.CheckpointManifestSealFactV2) {
			value.RuntimeParticipantSetDigest += "-drift"
		},
		"context closure": func(value *contract.CheckpointManifestSealFactV2) {
			value.ContextClosureDigest += "-drift"
		},
		"artifact closure": func(value *contract.CheckpointManifestSealFactV2) {
			value.ArtifactClosureDigest += "-drift"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			seal := base.Clone()
			mutate(&seal)
			testkit.RefreshSealV2(&seal)
			if err := contract.ValidateCheckpointManifestSealBindingV2(manifest, seal); !contract.HasCode(err, contract.ErrRevisionConflict) {
				t.Fatalf("changed immutable Seal closure was accepted: %v", err)
			}
		})
	}
}
