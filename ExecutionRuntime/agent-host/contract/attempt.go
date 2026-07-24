package contract

type AttemptStateV1 string

const (
	AttemptPlannedV1     AttemptStateV1 = "planned"
	AttemptUnknownV1     AttemptStateV1 = "unknown"
	AttemptConstructedV1 AttemptStateV1 = "constructed"
	AttemptBoundV1       AttemptStateV1 = "bound"
)

type ConstructionAttemptV1 struct {
	ContractVersion string                   `json:"contract_version"`
	AttemptID       string                   `json:"attempt_id"`
	HostID          string                   `json:"host_id"`
	StartID         string                   `json:"start_id"`
	GraphRef        ExactRefV1               `json:"graph_ref"`
	NodeID          string                   `json:"node_id"`
	Factory         FactoryKeyV1             `json:"factory"`
	Node            ComponentNodeV1          `json:"node"`
	Dependencies    []ConstructedComponentV1 `json:"dependencies"`
	RequestDigest   DigestV1                 `json:"request_digest"`
	State           AttemptStateV1           `json:"state"`
	ComponentRef    *ExactRefV1              `json:"component_ref,omitempty"`
	Reason          string                   `json:"reason,omitempty"`
	Digest          DigestV1                 `json:"digest"`
}

func NewConstructionAttemptV1(hostID, startID string, graph ExactRefV1, node ComponentNodeV1, dependencies []ConstructedComponentV1) (ConstructionAttemptV1, error) {
	if err := ValidateIdentifierV1("host id", hostID); err != nil {
		return ConstructionAttemptV1{}, err
	}
	if err := ValidateIdentifierV1("start id", startID); err != nil {
		return ConstructionAttemptV1{}, err
	}
	if err := graph.Validate(); err != nil {
		return ConstructionAttemptV1{}, err
	}
	if err := node.Validate(); err != nil {
		return ConstructionAttemptV1{}, err
	}
	for _, dependency := range dependencies {
		if err := dependency.Validate(); err != nil {
			return ConstructionAttemptV1{}, err
		}
	}
	cloned := append([]ConstructedComponentV1(nil), dependencies...)
	requestDigest, err := DigestJSONV1(struct {
		HostID, StartID string
		Graph           ExactRefV1
		Node            ComponentNodeV1
		Dependencies    []ConstructedComponentV1
	}{hostID, startID, graph, node, cloned})
	if err != nil {
		return ConstructionAttemptV1{}, err
	}
	attempt := ConstructionAttemptV1{ContractVersion: ContractVersionV1, AttemptID: "construction/" + string(requestDigest), HostID: hostID, StartID: startID, GraphRef: graph, NodeID: node.NodeID, Factory: node.Factory, Node: node, Dependencies: cloned, RequestDigest: requestDigest, State: AttemptPlannedV1}
	return SealConstructionAttemptV1(attempt)
}

func (a ConstructionAttemptV1) Validate() error {
	if a.ContractVersion != ContractVersionV1 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "construction attempt contract version is unsupported")
	}
	if err := ValidateIdentifierV1("attempt id", a.AttemptID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("host id", a.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", a.StartID); err != nil {
		return err
	}
	if err := a.GraphRef.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("node id", a.NodeID); err != nil {
		return err
	}
	if err := a.Factory.Validate(); err != nil {
		return err
	}
	if err := a.Node.Validate(); err != nil {
		return err
	}
	if a.Node.NodeID != a.NodeID || a.Node.Factory != a.Factory {
		return NewError(ErrorConflict, "attempt_node_drift", "construction attempt node identity drifted")
	}
	for _, dependency := range a.Dependencies {
		if err := dependency.Validate(); err != nil {
			return err
		}
	}
	if err := a.RequestDigest.Validate(); err != nil {
		return err
	}
	expectedRequest, err := DigestJSONV1(struct {
		HostID, StartID string
		Graph           ExactRefV1
		Node            ComponentNodeV1
		Dependencies    []ConstructedComponentV1
	}{a.HostID, a.StartID, a.GraphRef, a.Node, a.Dependencies})
	if err != nil {
		return err
	}
	if expectedRequest != a.RequestDigest || a.AttemptID != "construction/"+string(expectedRequest) {
		return NewError(ErrorPrecondition, "attempt_request_drift", "construction attempt no longer matches its exact request")
	}
	switch a.State {
	case AttemptPlannedV1:
		if a.ComponentRef != nil || a.Reason != "" {
			return NewError(ErrorInvalidArgument, "attempt_state_invalid", "planned construction carries outcome")
		}
	case AttemptUnknownV1:
		if a.ComponentRef != nil || a.Reason == "" {
			return NewError(ErrorInvalidArgument, "attempt_state_invalid", "unknown construction requires reason only")
		}
	case AttemptConstructedV1:
		if a.ComponentRef == nil || a.Reason != "" {
			return NewError(ErrorInvalidArgument, "attempt_state_invalid", "constructed attempt requires component ref only")
		}
		if err := a.ComponentRef.Validate(); err != nil {
			return err
		}
	default:
		return NewError(ErrorInvalidArgument, "attempt_state_invalid", "construction attempt state is unsupported")
	}
	expected, err := a.digestV1()
	if err != nil {
		return err
	}
	if expected != a.Digest {
		return NewError(ErrorPrecondition, "attempt_digest_drift", "construction attempt digest drifted")
	}
	return nil
}
func (a ConstructionAttemptV1) digestV1() (DigestV1, error) {
	clone := a
	clone.Digest = ""
	return DigestJSONV1(clone)
}
func SealConstructionAttemptV1(a ConstructionAttemptV1) (ConstructionAttemptV1, error) {
	digest, err := a.digestV1()
	if err != nil {
		return ConstructionAttemptV1{}, err
	}
	a.Digest = digest
	return a, nil
}

type BindingAttemptV1 struct {
	ContractVersion string         `json:"contract_version"`
	AttemptID       string         `json:"attempt_id"`
	RequestDigest   DigestV1       `json:"request_digest"`
	State           AttemptStateV1 `json:"state"`
	BindingRef      *ExactRefV1    `json:"binding_ref,omitempty"`
	Reason          string         `json:"reason,omitempty"`
	Digest          DigestV1       `json:"digest"`
}

func NewBindingAttemptV1(hostID, startID string, configDigest DigestV1, definition DecodedDefinitionV1, resolved ResolvedAssemblyV1, compiled CompiledAssemblyV1) (BindingAttemptV1, error) {
	if err := ValidateIdentifierV1("host id", hostID); err != nil {
		return BindingAttemptV1{}, err
	}
	if err := ValidateIdentifierV1("start id", startID); err != nil {
		return BindingAttemptV1{}, err
	}
	if err := configDigest.Validate(); err != nil {
		return BindingAttemptV1{}, err
	}
	if err := definition.Validate(); err != nil {
		return BindingAttemptV1{}, err
	}
	if err := resolved.Validate(); err != nil {
		return BindingAttemptV1{}, err
	}
	if err := compiled.Validate(); err != nil {
		return BindingAttemptV1{}, err
	}
	requestDigest, err := DigestJSONV1(struct {
		HostID, StartID string
		ConfigDigest    DigestV1
		Definition      DecodedDefinitionV1
		Resolved        ResolvedAssemblyV1
		Compiled        CompiledAssemblyV1
	}{hostID, startID, configDigest, definition, resolved, compiled})
	if err != nil {
		return BindingAttemptV1{}, err
	}
	attempt := BindingAttemptV1{ContractVersion: ContractVersionV1, AttemptID: "binding/" + string(requestDigest), RequestDigest: requestDigest, State: AttemptPlannedV1}
	return SealBindingAttemptV1(attempt)
}
func (a BindingAttemptV1) Validate() error {
	if a.ContractVersion != ContractVersionV1 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "binding attempt contract version is unsupported")
	}
	if err := ValidateIdentifierV1("attempt id", a.AttemptID); err != nil {
		return err
	}
	if err := a.RequestDigest.Validate(); err != nil {
		return err
	}
	if a.AttemptID != "binding/"+string(a.RequestDigest) {
		return NewError(ErrorPrecondition, "attempt_request_drift", "binding attempt id does not bind its request digest")
	}
	switch a.State {
	case AttemptPlannedV1:
		if a.BindingRef != nil || a.Reason != "" {
			return NewError(ErrorInvalidArgument, "attempt_state_invalid", "planned binding carries outcome")
		}
	case AttemptUnknownV1:
		if a.BindingRef != nil || a.Reason == "" {
			return NewError(ErrorInvalidArgument, "attempt_state_invalid", "unknown binding requires reason only")
		}
	case AttemptBoundV1:
		if a.BindingRef == nil || a.Reason != "" {
			return NewError(ErrorInvalidArgument, "attempt_state_invalid", "bound attempt requires binding ref only")
		}
		if err := a.BindingRef.Validate(); err != nil {
			return err
		}
	default:
		return NewError(ErrorInvalidArgument, "attempt_state_invalid", "binding attempt state is unsupported")
	}
	expected, err := a.digestV1()
	if err != nil {
		return err
	}
	if expected != a.Digest {
		return NewError(ErrorPrecondition, "attempt_digest_drift", "binding attempt digest drifted")
	}
	return nil
}
func (a BindingAttemptV1) digestV1() (DigestV1, error) {
	clone := a
	clone.Digest = ""
	return DigestJSONV1(clone)
}
func SealBindingAttemptV1(a BindingAttemptV1) (BindingAttemptV1, error) {
	digest, err := a.digestV1()
	if err != nil {
		return BindingAttemptV1{}, err
	}
	a.Digest = digest
	return a, nil
}
