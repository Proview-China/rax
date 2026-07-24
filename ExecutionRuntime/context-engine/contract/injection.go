package contract

import (
	"fmt"
	"sort"
)

type InjectionField struct {
	Path     string `json:"path"`
	Digest   Digest `json:"digest"`
	Required bool   `json:"required"`
	Opaque   bool   `json:"opaque"`
}

func (f InjectionField) Validate() error {
	if validateID(f.Path) != nil || f.Digest.Validate() != nil {
		return fmt.Errorf("%w: injection field", ErrInvalid)
	}
	return nil
}

type ExpectedInjectionManifest struct {
	ContractVersion string           `json:"contract_version"`
	ID              string           `json:"manifest_id"`
	Revision        uint64           `json:"revision"`
	Execution       ExecutionBinding `json:"execution"`
	FrameRef        FactRef          `json:"frame_ref"`
	Fields          []InjectionField `json:"fields"`
	AllowResidual   bool             `json:"allow_residual"`
	CapabilityRef   FactRef          `json:"capability_ref"`
	CreatedUnixNano int64            `json:"created_unix_nano"`
	ExpiresUnixNano int64            `json:"expires_unix_nano"`
}

func (m ExpectedInjectionManifest) Validate() error {
	if ValidateContract(m.ContractVersion) != nil || validateID(m.ID) != nil || m.Revision != 1 || m.Execution.Validate() != nil || m.FrameRef.Validate() != nil || m.CapabilityRef.Validate() != nil || validateTimes(m.CreatedUnixNano, m.ExpiresUnixNano) != nil || len(m.Fields) == 0 {
		return fmt.Errorf("%w: expected injection manifest", ErrInvalid)
	}
	seen := make(map[string]struct{}, len(m.Fields))
	for _, field := range m.Fields {
		if err := field.Validate(); err != nil {
			return err
		}
		if _, ok := seen[field.Path]; ok {
			return fmt.Errorf("%w: duplicate injection path", ErrConflict)
		}
		seen[field.Path] = struct{}{}
	}
	return nil
}

type ProviderActualInjectionObservation struct {
	ContractVersion  string           `json:"contract_version"`
	ID               string           `json:"observation_id"`
	Revision         uint64           `json:"revision"`
	Execution        ExecutionBinding `json:"execution"`
	FrameRef         FactRef          `json:"frame_ref"`
	RouteID          string           `json:"route_id"`
	AttemptID        string           `json:"attempt_id"`
	SourceSequence   uint64           `json:"source_sequence"`
	Fields           []InjectionField `json:"fields"`
	ObservedUnixNano int64            `json:"observed_unix_nano"`
}

func (o ProviderActualInjectionObservation) Validate() error {
	if ValidateContract(o.ContractVersion) != nil || validateID(o.ID) != nil || o.Revision != 1 || o.Execution.Validate() != nil || o.FrameRef.Validate() != nil || validateID(o.RouteID) != nil || validateID(o.AttemptID) != nil || o.SourceSequence == 0 || o.ObservedUnixNano <= 0 {
		return fmt.Errorf("%w: provider injection observation", ErrInvalid)
	}
	seen := make(map[string]struct{}, len(o.Fields))
	for _, field := range o.Fields {
		if err := field.Validate(); err != nil {
			return err
		}
		if _, ok := seen[field.Path]; ok {
			return fmt.Errorf("%w: duplicate provider observation path", ErrConflict)
		}
		seen[field.Path] = struct{}{}
	}
	return nil
}

type ObservationFidelity string

const (
	ObservationFidelityComplete    ObservationFidelity = "complete"
	ObservationFidelityPartial     ObservationFidelity = "partial"
	ObservationFidelityUnavailable ObservationFidelity = "unavailable"
)

type ActualInjectionObservationRef struct {
	ID             string              `json:"id"`
	Revision       uint64              `json:"revision"`
	Digest         Digest              `json:"digest"`
	Execution      ExecutionBinding    `json:"execution"`
	RouteID        string              `json:"route_id"`
	AttemptID      string              `json:"attempt_id"`
	FrameRef       FactRef             `json:"frame_ref"`
	SourceSequence uint64              `json:"source_sequence"`
	Fidelity       ObservationFidelity `json:"fidelity"`
}

func (r ActualInjectionObservationRef) Validate() error {
	if validateID(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil || r.Execution.Validate() != nil || validateID(r.RouteID) != nil || validateID(r.AttemptID) != nil || r.FrameRef.Validate() != nil || r.SourceSequence == 0 || !validObservationFidelity(r.Fidelity) {
		return fmt.Errorf("%w: actual injection observation reference", ErrInvalid)
	}
	return nil
}

func (o ProviderActualInjectionObservation) Ref(fidelity ObservationFidelity) (ActualInjectionObservationRef, error) {
	if err := o.Validate(); err != nil || !validObservationFidelity(fidelity) {
		return ActualInjectionObservationRef{}, fmt.Errorf("%w: provider observation reference", ErrInvalid)
	}
	digest, err := DigestJSON(o)
	if err != nil {
		return ActualInjectionObservationRef{}, err
	}
	return ActualInjectionObservationRef{
		ID: o.ID, Revision: o.Revision, Digest: digest, Execution: o.Execution,
		RouteID: o.RouteID, AttemptID: o.AttemptID, FrameRef: o.FrameRef,
		SourceSequence: o.SourceSequence, Fidelity: fidelity,
	}, nil
}

func SortActualInjectionObservationRefs(refs []ActualInjectionObservationRef) []ActualInjectionObservationRef {
	result := append([]ActualInjectionObservationRef(nil), refs...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].SourceSequence != result[j].SourceSequence {
			return result[i].SourceSequence < result[j].SourceSequence
		}
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].Revision < result[j].Revision
	})
	return result
}

type HarnessActualInjectionManifest struct {
	ContractVersion string                          `json:"contract_version"`
	ID              string                          `json:"manifest_id"`
	Revision        uint64                          `json:"revision"`
	Execution       ExecutionBinding                `json:"execution"`
	FrameRef        FactRef                         `json:"frame_ref"`
	RouteID         string                          `json:"route_id"`
	AttemptID       string                          `json:"attempt_id"`
	Fields          []InjectionField                `json:"fields"`
	ResidualPaths   []string                        `json:"residual_paths"`
	ObservationRefs []ActualInjectionObservationRef `json:"observation_refs"`
	CreatedUnixNano int64                           `json:"created_unix_nano"`
}

func (m HarnessActualInjectionManifest) Validate() error {
	if ValidateContract(m.ContractVersion) != nil || validateID(m.ID) != nil || m.Revision != 1 || m.Execution.Validate() != nil || m.FrameRef.Validate() != nil || validateID(m.RouteID) != nil || validateID(m.AttemptID) != nil || m.CreatedUnixNano <= 0 || len(m.ObservationRefs) == 0 {
		return fmt.Errorf("%w: actual injection manifest", ErrInvalid)
	}
	seen := make(map[string]struct{}, len(m.Fields))
	for _, field := range m.Fields {
		if err := field.Validate(); err != nil {
			return err
		}
		if _, ok := seen[field.Path]; ok {
			return fmt.Errorf("%w: duplicate actual path", ErrConflict)
		}
		seen[field.Path] = struct{}{}
	}
	for _, path := range m.ResidualPaths {
		if validateID(path) != nil {
			return fmt.Errorf("%w: residual path", ErrInvalid)
		}
	}
	seenObservationIDs := make(map[string]struct{}, len(m.ObservationRefs))
	seenSequences := make(map[uint64]struct{}, len(m.ObservationRefs))
	for index, ref := range m.ObservationRefs {
		if ref.Validate() != nil {
			return fmt.Errorf("%w: observation ref", ErrInvalid)
		}
		if ref.Execution != m.Execution || ref.RouteID != m.RouteID || ref.AttemptID != m.AttemptID || ref.FrameRef != m.FrameRef {
			return fmt.Errorf("%w: observation ref binding", ErrConflict)
		}
		if _, ok := seenObservationIDs[ref.ID]; ok {
			return fmt.Errorf("%w: duplicate observation id", ErrConflict)
		}
		if _, ok := seenSequences[ref.SourceSequence]; ok {
			return fmt.Errorf("%w: duplicate observation source sequence", ErrConflict)
		}
		if index > 0 && !observationRefLess(m.ObservationRefs[index-1], ref) {
			return fmt.Errorf("%w: observation refs not canonical", ErrConflict)
		}
		seenObservationIDs[ref.ID] = struct{}{}
		seenSequences[ref.SourceSequence] = struct{}{}
	}
	return nil
}

func validObservationFidelity(v ObservationFidelity) bool {
	return v == ObservationFidelityComplete || v == ObservationFidelityPartial || v == ObservationFidelityUnavailable
}

func observationRefLess(left, right ActualInjectionObservationRef) bool {
	if left.SourceSequence != right.SourceSequence {
		return left.SourceSequence < right.SourceSequence
	}
	if left.ID != right.ID {
		return left.ID < right.ID
	}
	return left.Revision < right.Revision
}

type InjectionConformanceState string

const (
	InjectionMatched         InjectionConformanceState = "matched"
	InjectionAllowedResidual InjectionConformanceState = "allowed_residual"
	InjectionRejected        InjectionConformanceState = "rejected"
	InjectionUnknown         InjectionConformanceState = "unknown"
)

type InjectionConformanceFact struct {
	ContractVersion   string                    `json:"contract_version"`
	ID                string                    `json:"fact_id"`
	Revision          uint64                    `json:"revision"`
	ExpectedRef       FactRef                   `json:"expected_ref"`
	ActualRef         FactRef                   `json:"actual_ref"`
	State             InjectionConformanceState `json:"state"`
	Reason            string                    `json:"reason"`
	InspectedUnixNano int64                     `json:"inspected_unix_nano"`
}

func (f InjectionConformanceFact) Validate() error {
	if ValidateContract(f.ContractVersion) != nil || validateID(f.ID) != nil || f.Revision != 1 || f.ExpectedRef.Validate() != nil || f.ActualRef.Validate() != nil || validateID(f.Reason) != nil || f.InspectedUnixNano <= 0 {
		return fmt.Errorf("%w: injection conformance fact", ErrInvalid)
	}
	if f.State != InjectionMatched && f.State != InjectionAllowedResidual && f.State != InjectionRejected && f.State != InjectionUnknown {
		return fmt.Errorf("%w: injection conformance state", ErrInvalid)
	}
	return nil
}
