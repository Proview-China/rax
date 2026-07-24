package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ControlledProviderFixtureV2 struct {
	Request     runtimeports.ControlledOperationProviderRequestV2
	Route       runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
	Observation runtimeports.ProviderAttemptObservationRefV2
}

func ControlledProviderV2(now time.Time) ControlledProviderFixtureV2 {
	boundary := BoundaryFixture(now)
	provider := ProviderBinding()
	prepared := PreparedAttempt(now, boundary, provider)
	persisted := runtimeports.PersistedOperationEnforcementRefV3{
		PermitID: boundary.Attempt.PermitID, PermitRevision: boundary.Attempt.PermitRevision,
		PermitDigest: boundary.Attempt.PermitDigest, AttemptID: boundary.Attempt.AttemptID,
		OperationDigest: boundary.Attempt.OperationDigest, Provider: provider,
		ReceiptDigest: Digest("controlled-provider-persisted"), RecordedRevision: 1,
	}
	semantics, err := runtimeports.SealControlledOperationPreparedSemanticSnapshotV2(runtimeports.ControlledOperationPreparedSemanticSnapshotV2{
		Prepared: prepared, Delegation: *boundary.Attempt.Delegation, PersistedEnforcement: persisted,
		OperationDigest: boundary.Attempt.OperationDigest, EffectID: boundary.Attempt.EffectID,
		IntentRevision: boundary.Attempt.IntentRevision, IntentDigest: boundary.Attempt.IntentDigest,
		Attempt: boundary.Attempt, ProviderBinding: provider, PayloadSchema: prepared.PayloadSchema,
		PayloadDigest: prepared.PayloadDigest, PayloadRevision: prepared.PayloadRevision,
	})
	if err != nil {
		panic(err)
	}
	route := ControlledProviderRouteV2(now, provider)
	evidencePolicy := runtimeports.OperationScopeEvidencePolicyRefV3(runtimeports.OperationScopeEvidenceFactRefV3{ID: "tool-evidence-policy-v2", Revision: 1, Digest: Digest("tool-evidence-policy-v2"), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	applicabilityPolicy := runtimeports.OperationScopeEvidenceApplicabilityPolicyRefV3(runtimeports.OperationScopeEvidenceFactRefV3{ID: "tool-applicability-policy-v2", Revision: 1, Digest: Digest("tool-applicability-policy-v2"), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	request, err := runtimeports.SealControlledOperationProviderRequestV2(runtimeports.ControlledOperationProviderRequestV2{
		RouteDeclarationRef: route.DeclarationRef, RouteConformanceRef: route.ConformanceRef,
		RouteCurrentRef: route.Ref, ToolAdapterBinding: route.ToolAdapterBinding,
		Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest,
		OperationScopeDigest: boundary.Operation.ExecutionScopeDigest, EffectID: boundary.Attempt.EffectID,
		EffectRevision: 3, EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3,
		IntentDigest: boundary.Attempt.IntentDigest, Attempt: boundary.Attempt, ProviderBinding: provider,
		Prepared: prepared, PreparedSemantics: semantics, ExecuteEnforcement: boundary.Enforcement,
		ExecuteEvidenceHandoff: boundary.Handoff.RefV3(), Boundary: runtimeports.OperationProviderBoundaryRefV1{ID: "tool-boundary-v2", Revision: 1, Digest: Digest("tool-boundary-v2")},
		EvidencePolicy: evidencePolicy, ApplicabilityPolicy: applicabilityPolicy,
		CallerDeadlineUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	observation := runtimeports.ProviderAttemptObservationRefV2{
		Delegation: semantics.Delegation, PreparedAttemptID: prepared.ID,
		ProviderOperationRef: "tool-provider-operation-v2", Revision: 1,
		State: runtimeports.ProviderAttemptObservedV2, Digest: Digest("tool-provider-observation-v2"),
		PayloadDigest: Digest("tool-provider-output-v2"), PayloadRevision: 1,
		SourceRegistrationID: "tool-provider-source-v2", SourceEpoch: 1, SourceSequence: 1,
		Evidence:         runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: Digest("tool-provider-ledger-v2"), Sequence: 1, RecordDigest: Digest("tool-provider-record-v2")},
		ObservedUnixNano: now.UnixNano(),
	}
	if err := observation.Validate(); err != nil {
		panic(err)
	}
	return ControlledProviderFixtureV2{Request: request, Route: route, Observation: observation}
}

func ControlledProviderRouteV2(now time.Time, provider runtimeports.ProviderBindingRefV2) runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 {
	declaration := runtimeports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "tool-route-v2", Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: Digest("tool-route-declaration-v2")}
	conformance := runtimeports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "tool-route-conformance-v2", Revision: 1, DeclarationRef: declaration, ConformanceDigest: Digest("tool-route-conformance-v2")}
	binding := func(component runtimeports.ComponentIDV2, capability runtimeports.CapabilityNameV2) runtimeports.ProviderBindingRefV2 {
		return runtimeports.ProviderBindingRefV2{BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, ComponentID: component, ManifestDigest: Digest("manifest-" + string(component)), ArtifactDigest: Digest("artifact-" + string(component)), Capability: capability}
	}
	route, err := runtimeports.SealControlledOperationProviderRouteCurrentProjectionV2(runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{
		Ref: runtimeports.ControlledOperationProviderRouteCurrentRefV2{Revision: 1}, DeclarationRef: declaration, ConformanceRef: conformance,
		Generation: runtimeports.GenerationArtifactRefV1{ID: "tool-generation-v2", Revision: 1, Digest: Digest("tool-generation-v2"), InputDigest: Digest("tool-generation-input-v2"), ManifestDigest: Digest("tool-generation-manifest-v2"), GraphDigest: Digest("tool-generation-graph-v2"), CatalogDigest: Digest("tool-generation-catalog-v2")},
		HandoffID:  "tool-route-handoff-v2", HandoffRevision: 1, HandoffDigest: Digest("tool-route-handoff-v2"),
		BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision,
		BindingSetDigest: Digest("tool-binding-set-v2"), BindingSetSemanticDigest: Digest("tool-binding-semantic-v2"), BindingSetCurrentnessDigest: Digest("tool-binding-current-v2"),
		ActiveRouteID: "tool-active-route-v2", ActiveRouteRevision: 1, ActiveRouteDigest: Digest("tool-active-route-v2"),
		ToolAdapterBinding:       binding("praxis.tool/adapter", runtimeports.ControlledOperationToolAdapterCapabilityV2),
		GatewayBinding:           binding("praxis.runtime/gateway", runtimeports.ControlledOperationGatewayCapabilityV2),
		ProviderTransportBinding: binding("praxis.tool/transport", runtimeports.ControlledOperationProviderTransportCapabilityV2),
		PreparedReaderBinding:    binding("praxis.runtime/prepared-reader", runtimeports.ControlledOperationPreparedReaderCapabilityV2),
		BoundaryReaderBinding:    binding("praxis.runtime/boundary-reader", runtimeports.ControlledOperationBoundaryReaderCapabilityV2),
		ProviderInspectBinding:   binding("praxis.runtime/provider-inspect", runtimeports.ControlledOperationProviderInspectCapabilityV2),
		ProviderBinding:          provider, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return route
}

func ControlledProviderResultV2(request runtimeports.ControlledOperationProviderRequestV2, status runtimeports.ControlledOperationProviderResultStatusV2, observation *runtimeports.ProviderAttemptObservationRefV2, now time.Time) runtimeports.ControlledOperationProviderResultV2 {
	key, err := runtimeports.DeriveControlledOperationProviderEntryKeyV2(request)
	if err != nil {
		panic(err)
	}
	entry := runtimeports.ControlledOperationProviderEntryRefV2{EntryID: key.EntryID, Revision: 1, StableKeyDigest: key.StableKeyDigest, Digest: Digest("tool-provider-entry-v2")}
	resultError := runtimeports.ControlledOperationProviderErrorNoneV2
	var receipt *runtimeports.ControlledOperationProviderAdmissionReceiptRefV2
	switch status {
	case runtimeports.ControlledOperationProviderEnteredV2:
		resultError = runtimeports.ControlledOperationProviderInspectionRequiredV2
	case runtimeports.ControlledOperationProviderUnknownV2:
		resultError = runtimeports.ControlledOperationProviderOutcomeUnknownV2
	case runtimeports.ControlledOperationProviderRejectedNoEffectV2:
		r, sealErr := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{ID: "tool-provider-no-effect-v2", Revision: 1, StableKeyDigest: key.StableKeyDigest, NoEffect: true})
		if sealErr != nil {
			panic(sealErr)
		}
		receipt = &r
	}
	result, err := runtimeports.SealControlledOperationProviderResultV2(runtimeports.ControlledOperationProviderResultV2{EntryRef: entry, Status: status, Error: resultError, Prepared: request.Prepared, Attempt: request.Attempt, AdmissionReceipt: receipt, Observation: observation, InspectedUnixNano: now.UnixNano()})
	if err != nil {
		panic(err)
	}
	return result
}

func DriftDigest(label string) core.Digest { return Digest(label) }
