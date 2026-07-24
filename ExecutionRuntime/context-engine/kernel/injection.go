package kernel

import (
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func CompareInjection(id string, expected contract.ExpectedInjectionManifest, actual contract.HarnessActualInjectionManifest, observations []contract.ProviderActualInjectionObservation, inspectedUnixNano int64) (contract.InjectionConformanceFact, error) {
	if expected.Validate() != nil || inspectedUnixNano <= 0 {
		return contract.InjectionConformanceFact{}, fmt.Errorf("%w: injection comparison", contract.ErrInvalid)
	}
	expectedDigest, _ := contract.DigestJSON(expected)
	actualDigest, _ := contract.DigestJSON(actual)
	fact := contract.InjectionConformanceFact{
		ContractVersion: contract.Version, ID: id, Revision: 1,
		ExpectedRef:       contract.FactRef{ID: expected.ID, Revision: expected.Revision, Digest: expectedDigest},
		ActualRef:         contract.FactRef{ID: actual.ID, Revision: actual.Revision, Digest: actualDigest},
		InspectedUnixNano: inspectedUnixNano,
	}
	finish := func(state contract.InjectionConformanceState, reason string) (contract.InjectionConformanceFact, error) {
		fact.State, fact.Reason = state, reason
		return fact, fact.Validate()
	}
	if inspectedUnixNano < expected.CreatedUnixNano || inspectedUnixNano >= expected.ExpiresUnixNano {
		return finish(contract.InjectionRejected, "expected_manifest_not_current")
	}
	if expected.Execution != actual.Execution || expected.FrameRef != actual.FrameRef {
		return finish(contract.InjectionRejected, "actual_manifest_binding_mismatch")
	}
	if len(actual.ObservationRefs) == 0 || len(observations) == 0 {
		return finish(contract.InjectionUnknown, "provider_observation_missing")
	}
	if err := actual.Validate(); err != nil {
		return finish(contract.InjectionRejected, "actual_manifest_invalid")
	}
	if len(observations) < len(actual.ObservationRefs) {
		return finish(contract.InjectionUnknown, "provider_observation_missing")
	}
	if len(observations) > len(actual.ObservationRefs) {
		return finish(contract.InjectionRejected, "unreferenced_provider_observation")
	}
	observationsByID := make(map[string]contract.ProviderActualInjectionObservation, len(observations))
	for _, observation := range observations {
		if err := observation.Validate(); err != nil {
			return finish(contract.InjectionRejected, "provider_observation_invalid")
		}
		if _, ok := observationsByID[observation.ID]; ok {
			return finish(contract.InjectionRejected, "duplicate_provider_observation")
		}
		observationsByID[observation.ID] = observation
	}
	observedFields := make(map[string]contract.InjectionField)
	for _, ref := range actual.ObservationRefs {
		observation, ok := observationsByID[ref.ID]
		if !ok {
			return finish(contract.InjectionUnknown, "provider_observation_missing")
		}
		if observation.Execution != ref.Execution || observation.Execution != actual.Execution {
			return finish(contract.InjectionRejected, "observation_execution_mismatch")
		}
		if observation.RouteID != ref.RouteID || observation.RouteID != actual.RouteID {
			return finish(contract.InjectionRejected, "observation_route_mismatch")
		}
		if observation.AttemptID != ref.AttemptID || observation.AttemptID != actual.AttemptID {
			return finish(contract.InjectionRejected, "observation_attempt_mismatch")
		}
		if observation.FrameRef != ref.FrameRef || observation.FrameRef != actual.FrameRef {
			return finish(contract.InjectionRejected, "observation_frame_mismatch")
		}
		if observation.SourceSequence != ref.SourceSequence {
			return finish(contract.InjectionRejected, "observation_source_sequence_mismatch")
		}
		if observation.Revision != ref.Revision {
			return finish(contract.InjectionRejected, "observation_revision_mismatch")
		}
		digest, err := contract.DigestJSON(observation)
		if err != nil || digest != ref.Digest {
			return finish(contract.InjectionRejected, "observation_digest_mismatch")
		}
		if ref.Fidelity != contract.ObservationFidelityComplete {
			return finish(contract.InjectionUnknown, "observation_fidelity_incomplete")
		}
		for _, field := range observation.Fields {
			if previous, exists := observedFields[field.Path]; exists && previous != field {
				return finish(contract.InjectionRejected, "observation_field_conflict")
			}
			observedFields[field.Path] = field
		}
	}
	for _, field := range actual.Fields {
		observed, ok := observedFields[field.Path]
		if !ok || observed != field {
			return finish(contract.InjectionRejected, "actual_field_not_observed")
		}
	}
	for path := range observedFields {
		found := false
		for _, field := range actual.Fields {
			if field.Path == path {
				found = true
				break
			}
		}
		if !found {
			return finish(contract.InjectionRejected, "observed_field_not_manifested")
		}
	}
	actualByPath := make(map[string]contract.InjectionField, len(actual.Fields))
	for _, field := range actual.Fields {
		actualByPath[field.Path] = field
	}
	expectedByPath := make(map[string]contract.InjectionField, len(expected.Fields))
	residual := false
	for _, wanted := range expected.Fields {
		expectedByPath[wanted.Path] = wanted
		got, ok := actualByPath[wanted.Path]
		if wanted.Opaque || (ok && got.Opaque) {
			fact.State, fact.Reason = contract.InjectionUnknown, "required_content_opaque"
			return finish(fact.State, fact.Reason)
		}
		if !ok || got.Digest != wanted.Digest {
			if wanted.Required {
				fact.State, fact.Reason = contract.InjectionRejected, "required_field_mismatch"
				return finish(fact.State, fact.Reason)
			}
			if !expected.AllowResidual {
				fact.State, fact.Reason = contract.InjectionRejected, "residual_not_allowed"
				return finish(fact.State, fact.Reason)
			}
			residual = true
		}
	}
	residualPaths := make(map[string]struct{}, len(actual.ResidualPaths))
	for _, path := range actual.ResidualPaths {
		residualPaths[path] = struct{}{}
	}
	for _, got := range actual.Fields {
		if _, ok := expectedByPath[got.Path]; ok {
			continue
		}
		if got.Opaque {
			fact.State, fact.Reason = contract.InjectionUnknown, "unexpected_content_opaque"
			return finish(fact.State, fact.Reason)
		}
		if _, declared := residualPaths[got.Path]; !declared {
			fact.State, fact.Reason = contract.InjectionRejected, "undeclared_actual_field"
			return finish(fact.State, fact.Reason)
		}
		residual = true
	}
	if len(actual.ResidualPaths) > 0 {
		if !expected.AllowResidual {
			fact.State, fact.Reason = contract.InjectionRejected, "residual_not_allowed"
		} else {
			fact.State, fact.Reason = contract.InjectionAllowedResidual, "declared_residual"
		}
	} else if residual {
		fact.State, fact.Reason = contract.InjectionAllowedResidual, "allowed_optional_drift"
	} else {
		fact.State, fact.Reason = contract.InjectionMatched, "exact_match"
	}
	return finish(fact.State, fact.Reason)
}
