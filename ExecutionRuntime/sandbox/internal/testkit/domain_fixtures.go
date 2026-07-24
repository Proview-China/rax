package testkit

import "github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"

func Requirement() contract.ExecutionRequirement {
	return contract.ExecutionRequirement{
		Meta:          Meta("requirement", 1),
		OSFamily:      "linux",
		Architecture:  "amd64",
		ReadScopes:    []string{"src"},
		WriteScopes:   []string{"src/generated"},
		Network:       contract.NetworkRequirement{Mode: contract.NetworkDenyAll},
		ProcessScopes: []string{"go"},
		Resources:     contract.ResourceBounds{CPUUnits: 1, MemoryBytes: 1024, StorageBytes: 1024, PIDLimit: 4, WallTimeSeconds: 30},
		RequiredCapabilities: []contract.BackendCapability{
			contract.CapabilityExecutionControlled,
			contract.CapabilityFilesView,
			contract.CapabilityFilesOverlay,
			contract.CapabilityNetworkDenyAll,
			contract.CapabilityProcessFence,
			contract.CapabilityInspectAttempt,
			contract.CapabilityCleanupCoverage,
		},
		Risk:              contract.RiskHigh,
		AllowedSurfaces:   []contract.ExecutionSurface{contract.SurfaceContainer},
		AllowedDowngrades: []contract.ExecutionSurface{contract.SurfaceMicroVM},
	}
}

func Policy() contract.PolicyProjection {
	return contract.PolicyProjection{
		Meta:                    Meta("policy", 1),
		RequirementRef:          Requirement().Meta.Ref(),
		SourcePolicyRef:         Ref("source-policy"),
		AuthorityRef:            Ref("authority"),
		ReviewPolicyRef:         Ref("review-policy"),
		BudgetPolicyRef:         Ref("budget-policy"),
		ScopeDigest:             Ref("scope").Digest,
		CapabilityGrantDigest:   Ref("capability-grant").Digest,
		ReadScopes:              []string{"src"},
		WriteScopes:             []string{"src/generated"},
		Network:                 contract.NetworkRequirement{Mode: contract.NetworkDenyAll},
		ProcessScopes:           []string{"go"},
		Resources:               contract.ResourceBounds{CPUUnits: 1, MemoryBytes: 1024, StorageBytes: 1024, PIDLimit: 4, WallTimeSeconds: 30},
		MinimumConformance:      contract.ConformanceRestrictedControlled,
		ExternalEffectsDisabled: true,
	}
}

func Backend() contract.BackendDescriptor {
	capabilities := make(map[contract.BackendCapability]contract.CapabilityLevel)
	for _, capability := range Requirement().RequiredCapabilities {
		capabilities[capability] = contract.CapabilityEnforced
	}
	return contract.BackendDescriptor{
		Meta:               Meta("backend", 1),
		Surface:            contract.SurfaceContainer,
		Locality:           contract.LocalityInstanceDataPlane,
		ArtifactRef:        Ref("backend-artifact"),
		BackendContractRef: Ref("backend-contract"),
		Capabilities:       capabilities,
		Conformance:        contract.ConformanceRestrictedControlled,
		ConformanceRef:     Ref("backend-conformance"),
	}
}

func Candidate() contract.PlacementCandidate {
	return contract.PlacementCandidate{
		Meta:              Meta("candidate", 1),
		RequirementRef:    Requirement().Meta.Ref(),
		PolicyRef:         Policy().Meta.Ref(),
		BackendRef:        Backend().Meta.Ref(),
		SlotCandidateRef:  Meta("slot-candidate", 1).Ref(),
		MatchEvidenceRefs: []contract.Ref{Ref("match-evidence")},
	}
}

func ProviderBinding() contract.ProviderBindingRef {
	return contract.ProviderBindingRef{
		BindingSetID: "binding-set-1", BindingSetRevision: 1,
		ComponentID: "praxis.sandbox/provider", ManifestDigest: Ref("provider-manifest").Digest,
		ArtifactDigest: Ref("provider-artifact").Digest, Capability: "praxis.sandbox/execute",
	}
}

func Slot() contract.SlotCandidate {
	return contract.SlotCandidate{Meta: Meta("slot-candidate", 1), State: contract.CurrentFactActive, PlacementRef: Candidate().Meta.Ref(), BackendRef: Backend().Meta.Ref(), ProviderBinding: ProviderBinding()}
}

func WorkspaceView() contract.WorkspaceView {
	return contract.WorkspaceView{
		Meta:            Meta("workspace-view", 1),
		BaseArtifactRef: Ref("workspace-artifact"),
		BaseRevision:    "revision-a",
		OverlayRef:      Ref("overlay"),
		PolicyRef:       Policy().Meta.Ref(),
		Lease:           Lease(),
		ReadScopes:      []string{"src"},
		WriteScopes:     []string{"src/generated"},
		HiddenScopes:    []string{"src/generated/private"},
		FileScopeDigest: Ref("file-scope").Digest,
	}
}

func ConformanceReport() contract.BackendConformanceReport {
	backend := Backend()
	requirement := Requirement()
	return contract.BackendConformanceReport{
		Meta:              Meta("conformance-report", 1),
		BackendRef:        backend.Meta.Ref(),
		RequirementRef:    requirement.Meta.Ref(),
		Disposition:       contract.ConformanceAdmitted,
		CapabilityResults: backend.Capabilities,
		EvidenceRefs:      []contract.Ref{Ref("conformance-evidence")},
		ProductionProof:   false,
	}
}
