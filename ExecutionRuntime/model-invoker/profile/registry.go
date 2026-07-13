package profile

import (
	"fmt"
	"sort"
	"time"
)

type ErrorCode string

const (
	ErrorInvalidProfile      ErrorCode = "invalid_profile"
	ErrorResolutionFailed    ErrorCode = "profile_resolution_failed"
	ErrorProfileIncompatible ErrorCode = "profile_incompatible"
	ErrorPolicyRejected      ErrorCode = "policy_rejected"
	ErrorManifestUnavailable ErrorCode = "manifest_unavailable"
	ErrorManifestDrift       ErrorCode = "manifest_drift"
	ErrorCapabilityRejected  ErrorCode = "capability_rejected"
)

type Error struct {
	Code       ErrorCode
	Operation  string
	Message    string
	Path       string
	Candidates []ProfileID
}

func (err *Error) Error() string {
	if err == nil {
		return "<nil>"
	}
	message := err.Message
	if message == "" {
		message = string(err.Code)
	}
	if err.Operation != "" {
		return fmt.Sprintf("profile %s: %s", err.Operation, message)
	}
	return message
}

type ProfileSelector struct {
	ID          ProfileID
	Constraints SelectionConstraints
}

type Registry struct {
	profiles map[ProfileID]SemanticRouteProfile
	ids      []ProfileID
}

func NewRegistry(now time.Time, profiles ...SemanticRouteProfile) (*Registry, error) {
	if now.IsZero() {
		return nil, &Error{Code: ErrorInvalidProfile, Operation: "new_registry", Message: "validation time is required"}
	}
	registry := &Registry{profiles: make(map[ProfileID]SemanticRouteProfile, len(profiles))}
	selectionDigests := make(map[string]ProfileID, len(profiles))
	for index, candidate := range profiles {
		if err := candidate.Validate(now); err != nil {
			return nil, &Error{Code: ErrorInvalidProfile, Operation: "new_registry", Message: fmt.Sprintf("profile %d: %s", index, err)}
		}
		if _, duplicate := registry.profiles[candidate.ID]; duplicate {
			return nil, &Error{Code: ErrorInvalidProfile, Operation: "new_registry", Message: fmt.Sprintf("duplicate profile ID %q", candidate.ID)}
		}
		digest, err := candidate.Selection.Digest()
		if err != nil {
			return nil, &Error{Code: ErrorInvalidProfile, Operation: "new_registry", Message: err.Error()}
		}
		if previous, duplicate := selectionDigests[digest]; duplicate {
			return nil, &Error{
				Code: ErrorInvalidProfile, Operation: "new_registry",
				Message: fmt.Sprintf("profiles %q and %q have the same exact selection key", previous, candidate.ID),
			}
		}
		selectionDigests[digest] = candidate.ID
		registry.profiles[candidate.ID] = candidate.Clone()
		registry.ids = append(registry.ids, candidate.ID)
	}
	sort.Slice(registry.ids, func(i, j int) bool { return registry.ids[i] < registry.ids[j] })
	return registry, nil
}

func (registry *Registry) IDs() []ProfileID {
	if registry == nil {
		return nil
	}
	return append([]ProfileID(nil), registry.ids...)
}

func (registry *Registry) Get(id ProfileID) (SemanticRouteProfile, bool) {
	if registry == nil {
		return SemanticRouteProfile{}, false
	}
	value, ok := registry.profiles[id]
	if !ok {
		return SemanticRouteProfile{}, false
	}
	return value.Clone(), true
}

func (registry *Registry) Resolve(selector ProfileSelector) (SemanticRouteProfile, error) {
	if registry == nil {
		return SemanticRouteProfile{}, &Error{Code: ErrorResolutionFailed, Operation: "resolve", Message: "profile registry is nil"}
	}
	if selector.ID != "" {
		if !constraintsEmpty(selector.Constraints) {
			return SemanticRouteProfile{}, &Error{Code: ErrorResolutionFailed, Operation: "resolve", Message: "profile ID and constraints are mutually exclusive"}
		}
		profile, ok := registry.Get(selector.ID)
		if !ok {
			return SemanticRouteProfile{}, &Error{Code: ErrorResolutionFailed, Operation: "resolve", Message: "profile ID was not found"}
		}
		return profile, nil
	}
	var matches []ProfileID
	for _, id := range registry.ids {
		if selector.Constraints.matches(registry.profiles[id].Selection) {
			matches = append(matches, id)
		}
	}
	if len(matches) != 1 {
		message := "profile selector did not resolve exactly one profile"
		if len(matches) > 1 {
			message = "profile selector is ambiguous"
		}
		return SemanticRouteProfile{}, &Error{
			Code: ErrorResolutionFailed, Operation: "resolve", Message: message,
			Candidates: append([]ProfileID(nil), matches...),
		}
	}
	return registry.profiles[matches[0]].Clone(), nil
}

func constraintsEmpty(value SelectionConstraints) bool {
	return value.BaseRouteID == "" && value.Provider == "" && value.ModelID == "" && value.ModelRevision == "" &&
		value.Deployment == "" && value.Region == "" && value.EndpointIdentity == "" && value.Protocol == "" &&
		value.ProtocolSchemaVersion == "" && value.Offering == "" && value.AuthRoute == "" &&
		value.ExecutionSurface == "" && !value.HarnessStackSpecified && len(value.HarnessStack) == 0 &&
		value.HarnessStackDigest == ""
}
