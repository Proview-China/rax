package upstream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RouteIdentity is the auditable identity projection of an UpstreamRoute. The
// pipe delimiter is excluded by every component's identifier grammar, making
// CanonicalKey deterministic and collision-free for validated identities.
type RouteIdentity struct {
	RouteID     RouteID             `json:"route_id"`
	ModelFamily string              `json:"model_family"`
	Provider    ProviderID          `json:"provider"`
	Offering    OfferingID          `json:"offering"`
	Deployment  DeploymentID        `json:"deployment"`
	Protocol    ProtocolID          `json:"protocol"`
	Endpoint    EndpointID          `json:"endpoint"`
	Credential  CredentialProfileID `json:"credential"`
}

func (r UpstreamRoute) Identity() RouteIdentity {
	return RouteIdentity{
		RouteID:     r.ID,
		ModelFamily: r.Model.CanonicalFamily,
		Provider:    r.Provider,
		Offering:    r.Offering.ID,
		Deployment:  r.Deployment.ID,
		Protocol:    r.Protocol.ID,
		Endpoint:    r.Endpoint.ID,
		Credential:  r.Credential.ID,
	}
}

type IdentityValidationError struct {
	Fields []FieldError
}

func (e *IdentityValidationError) Error() string {
	return formatFieldErrors("invalid route identity", e.Fields)
}

func (e *IdentityValidationError) HasField(field string) bool {
	return hasField(e.Fields, field)
}

func (identity RouteIdentity) Validate() error {
	var fields []FieldError
	add := func(field, problem string) { fields = append(fields, FieldError{Field: field, Problem: problem}) }
	validateID(add, "route_id", string(identity.RouteID))
	validateID(add, "model_family", identity.ModelFamily)
	validateID(add, "provider", string(identity.Provider))
	validateID(add, "offering", string(identity.Offering))
	validateID(add, "deployment", string(identity.Deployment))
	if !protocolPattern.MatchString(string(identity.Protocol)) {
		add("protocol", "must be a stable lowercase protocol identifier")
	}
	validateID(add, "endpoint", string(identity.Endpoint))
	validateID(add, "credential", string(identity.Credential))
	if len(fields) == 0 {
		return nil
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
	return &IdentityValidationError{Fields: fields}
}

func (identity RouteIdentity) CanonicalKey() (string, error) {
	if err := identity.Validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		string(identity.RouteID),
		identity.ModelFamily,
		string(identity.Provider),
		string(identity.Offering),
		string(identity.Deployment),
		string(identity.Protocol),
		string(identity.Endpoint),
		string(identity.Credential),
	}, "|"), nil
}

func (identity RouteIdentity) Digest() (string, error) {
	key, err := identity.CanonicalKey()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

type MappingValidationError struct {
	Fields []FieldError
}

func (e *MappingValidationError) Error() string {
	return formatFieldErrors("invalid mapping report", e.Fields)
}

func (e *MappingValidationError) HasField(field string) bool {
	return hasField(e.Fields, field)
}

// NewMappingReport constructs and validates a complete auditable report.
func NewMappingReport(route UpstreamRoute, evidenceDigest string, decisions []CapabilityDecision, reasons ...MappingReason) (MappingReport, error) {
	if err := route.Validate(); err != nil {
		return MappingReport{}, fmt.Errorf("build mapping report: %w", err)
	}
	report := MappingReport{
		Identity:            route.Identity(),
		RouteID:             route.ID,
		Provider:            route.Provider,
		EvidenceDigest:      evidenceDigest,
		Reasons:             append([]MappingReason(nil), reasons...),
		CapabilityDecisions: cloneCapabilityDecisions(decisions),
	}
	report = report.Canonical()
	if err := report.Validate(); err != nil {
		return MappingReport{}, err
	}
	return report, nil
}

// Canonical returns a deep copy with set-like audit collections in stable
// lexical order. The receiver is never mutated.
func (r MappingReport) Canonical() MappingReport {
	clone := r.Clone()
	sort.SliceStable(clone.Reasons, func(i, j int) bool {
		if clone.Reasons[i].Code != clone.Reasons[j].Code {
			return clone.Reasons[i].Code < clone.Reasons[j].Code
		}
		return clone.Reasons[i].Detail < clone.Reasons[j].Detail
	})
	sort.SliceStable(clone.CapabilityDecisions, func(i, j int) bool {
		left, right := clone.CapabilityDecisions[i], clone.CapabilityDecisions[j]
		if left.Capability != right.Capability {
			return left.Capability < right.Capability
		}
		if left.Action != right.Action {
			return left.Action < right.Action
		}
		if left.ReasonCode != right.ReasonCode {
			return left.ReasonCode < right.ReasonCode
		}
		return left.Detail < right.Detail
	})
	for index := range clone.CapabilityDecisions {
		sort.Strings(clone.CapabilityDecisions[index].Limitations)
	}
	sort.Strings(clone.Degradations)
	return clone
}

func (r MappingReport) Validate() error {
	var fields []FieldError
	add := func(field, problem string) { fields = append(fields, FieldError{Field: field, Problem: problem}) }
	if err := r.Identity.Validate(); err != nil {
		if identityError, ok := err.(*IdentityValidationError); ok {
			for _, field := range identityError.Fields {
				add("identity."+field.Field, field.Problem)
			}
		} else {
			add("identity", err.Error())
		}
	}
	if r.RouteID != r.Identity.RouteID {
		add("route_id", "must match identity.route_id")
	}
	if r.Provider != r.Identity.Provider {
		add("provider", "must match identity.provider")
	}
	if !validSHA256Digest(r.EvidenceDigest) {
		add("evidence_digest", "must be sha256 followed by 64 lowercase hexadecimal characters")
	}
	seenReasons := make(map[string]struct{}, len(r.Reasons))
	for index, reason := range r.Reasons {
		field := fmt.Sprintf("reasons[%d]", index)
		if !protocolPattern.MatchString(reason.Code) {
			add(field+".code", "must be a stable lowercase reason code")
		}
		if strings.ContainsAny(reason.Detail, "\r\n") {
			add(field+".detail", "must be a single line")
		}
		key := reason.Code + "\x00" + reason.Detail
		if _, exists := seenReasons[key]; exists {
			add(field, "duplicates an earlier reason")
		}
		seenReasons[key] = struct{}{}
	}
	if len(r.CapabilityDecisions) == 0 {
		add("capability_decisions", "at least one capability decision is required")
	}
	seenCapabilities := make(map[string]struct{}, len(r.CapabilityDecisions))
	for index, decision := range r.CapabilityDecisions {
		field := fmt.Sprintf("capability_decisions[%d]", index)
		if !protocolPattern.MatchString(decision.Capability) {
			add(field+".capability", "must be a stable lowercase capability ID")
		}
		if !decision.Action.valid() {
			add(field+".action", "unsupported capability action")
		}
		if !protocolPattern.MatchString(decision.ReasonCode) {
			add(field+".reason_code", "must be a stable lowercase reason code")
		}
		if strings.ContainsAny(decision.Detail, "\r\n") {
			add(field+".detail", "must be a single line")
		}
		if _, exists := seenCapabilities[decision.Capability]; exists {
			add(field+".capability", "duplicates an earlier capability decision")
		}
		seenCapabilities[decision.Capability] = struct{}{}
		seenLimitations := make(map[string]struct{}, len(decision.Limitations))
		for limitationIndex, limitation := range decision.Limitations {
			if strings.TrimSpace(limitation) == "" || strings.ContainsAny(limitation, "\r\n") {
				add(fmt.Sprintf("%s.limitations[%d]", field, limitationIndex), "must be non-blank and single-line")
			}
			if _, exists := seenLimitations[limitation]; exists {
				add(fmt.Sprintf("%s.limitations[%d]", field, limitationIndex), "duplicates an earlier limitation")
			}
			seenLimitations[limitation] = struct{}{}
		}
	}
	seenDegradations := make(map[string]struct{}, len(r.Degradations))
	for index, degradation := range r.Degradations {
		if strings.TrimSpace(degradation) == "" || strings.ContainsAny(degradation, "\r\n") {
			add(fmt.Sprintf("degradations[%d]", index), "must be non-blank and single-line")
		}
		if _, exists := seenDegradations[degradation]; exists {
			add(fmt.Sprintf("degradations[%d]", index), "duplicates an earlier degradation")
		}
		seenDegradations[degradation] = struct{}{}
	}
	if len(fields) == 0 {
		return nil
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
	return &MappingValidationError{Fields: fields}
}

// AuditDigest hashes the canonical report representation. Equal semantic
// reports produce equal digests regardless of input slice ordering.
func (r MappingReport) AuditDigest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(r.Canonical())
	if err != nil {
		return "", fmt.Errorf("marshal canonical mapping report: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func cloneCapabilityDecisions(decisions []CapabilityDecision) []CapabilityDecision {
	clone := make([]CapabilityDecision, len(decisions))
	for index, decision := range decisions {
		clone[index] = decision
		clone[index].Limitations = append([]string(nil), decision.Limitations...)
	}
	return clone
}

func validSHA256Digest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	encoded := strings.TrimPrefix(value, "sha256:")
	if strings.ToLower(encoded) != encoded {
		return false
	}
	decoded, err := hex.DecodeString(encoded)
	return err == nil && len(decoded) == sha256.Size
}

func (action CapabilityDecisionAction) valid() bool {
	switch action {
	case CapabilityExact, CapabilityTransformed, CapabilityDegraded, CapabilityRejected:
		return true
	default:
		return false
	}
}

func formatFieldErrors(subject string, fields []FieldError) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, field.Field+": "+field.Problem)
	}
	return subject + ": " + strings.Join(parts, "; ")
}

func hasField(fields []FieldError, field string) bool {
	for _, candidate := range fields {
		if candidate.Field == field || strings.HasPrefix(candidate.Field, field+".") || strings.HasPrefix(candidate.Field, field+"[") {
			return true
		}
	}
	return false
}
