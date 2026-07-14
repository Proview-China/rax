package control

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type BindingLifecycleStateV2 string

const (
	BindingDeclared  BindingLifecycleStateV2 = "declared"
	BindingProbed    BindingLifecycleStateV2 = "probed"
	BindingCertified BindingLifecycleStateV2 = "certified"
	BindingBound     BindingLifecycleStateV2 = "bound"
	BindingRevoked   BindingLifecycleStateV2 = "revoked"
	BindingExpired   BindingLifecycleStateV2 = "expired"
)

type BindingSetStateV2 string

const (
	BindingSetActive  BindingSetStateV2 = "active"
	BindingSetRevoked BindingSetStateV2 = "revoked"
	BindingSetExpired BindingSetStateV2 = "expired"
)

type BindingFactV2 struct {
	ID                        string                      `json:"id"`
	ComponentID               ports.ComponentIDV2         `json:"component_id"`
	Manifest                  ports.ComponentManifestV2   `json:"manifest"`
	ManifestDigest            core.Digest                 `json:"manifest_digest"`
	GovernanceDigest          core.Digest                 `json:"governance_digest"`
	State                     BindingLifecycleStateV2     `json:"state"`
	Revision                  core.Revision               `json:"revision"`
	Grants                    []ports.CapabilityGrantV2   `json:"grants"`
	ProbedUnixNano            int64                       `json:"probed_unix_nano"`
	CertifiedUnixNano         int64                       `json:"certified_unix_nano"`
	ConformanceEvidenceDigest core.Digest                 `json:"conformance_evidence_digest,omitempty"`
	ExpiresUnixNano           int64                       `json:"expires_unix_nano"`
	BindingSetID              string                      `json:"binding_set_id,omitempty"`
	InvalidationReason        core.ReasonCode             `json:"invalidation_reason,omitempty"`
	RenewalEvidence           []ports.EvidenceRecordRefV2 `json:"renewal_evidence"`
}

type BindingFactCASRequestV2 struct {
	ExpectedRevision core.Revision `json:"expected_revision"`
	Next             BindingFactV2 `json:"next"`
}

type BindingMemberV2 struct {
	BindingID       string                    `json:"binding_id"`
	BindingRevision core.Revision             `json:"binding_revision"`
	ComponentID     ports.ComponentIDV2       `json:"component_id"`
	Kind            ports.ComponentKindV2     `json:"kind"`
	ManifestDigest  core.Digest               `json:"manifest_digest"`
	ArtifactDigest  core.Digest               `json:"artifact_digest"`
	Contract        ports.ContractBindingV2   `json:"contract"`
	Owners          []ports.OwnerAssignmentV2 `json:"owners"`
	Grants          []ports.CapabilityGrantV2 `json:"grants"`
}

type BindingResidualV2 struct {
	ComponentID ports.ComponentIDV2 `json:"component_id"`
	Reason      core.ReasonCode     `json:"reason"`
}

type BindingSetFactV2 struct {
	ID                 string                `json:"id"`
	PlanID             string                `json:"plan_id"`
	PlanDigest         core.Digest           `json:"plan_digest"`
	GovernanceDigest   core.Digest           `json:"governance_digest"`
	State              BindingSetStateV2     `json:"state"`
	Revision           core.Revision         `json:"revision"`
	Members            []BindingMemberV2     `json:"members"`
	TopologicalOrder   []ports.ComponentIDV2 `json:"topological_order"`
	Residuals          []BindingResidualV2   `json:"residuals"`
	CreatedUnixNano    int64                 `json:"created_unix_nano"`
	ExpiresUnixNano    int64                 `json:"expires_unix_nano"`
	InvalidationReason core.ReasonCode       `json:"invalidation_reason,omitempty"`
}

type ExpectedBindingRevisionV2 struct {
	BindingID        string        `json:"binding_id"`
	ExpectedRevision core.Revision `json:"expected_revision"`
}

type CommitBindingSetRequestV2 struct {
	Set      BindingSetFactV2            `json:"set"`
	Expected []ExpectedBindingRevisionV2 `json:"expected"`
}

type BindingSetCASRequestV2 struct {
	ExpectedRevision core.Revision    `json:"expected_revision"`
	Next             BindingSetFactV2 `json:"next"`
}

// RenewBindingSetRequestV2 is a single Binding Fact Owner transaction. Raw
// BindingSet CAS cannot renew an active set because it cannot prove that each
// member grant and its certification evidence advanced together.
type RenewBindingSetRequestV2 struct {
	ExpectedSetRevision core.Revision    `json:"expected_set_revision"`
	NextSet             BindingSetFactV2 `json:"next_set"`
	NextBindings        []BindingFactV2  `json:"next_bindings"`
}

const BindingRenewalAttestationKindV2 ports.NamespacedNameV2 = "runtime/binding-renewal"
const BindingRenewalCertifierCapabilityV2 ports.CapabilityNameV2 = "runtime/certify-binding-renewal"

// BindingRenewalAttestationV2 is an immutable projection returned by an
// independently governed certification owner. The Binding store never accepts
// a caller-provided EvidenceRecordRef as certification by itself.
type BindingRenewalAttestationV2 struct {
	Evidence         ports.EvidenceRecordRefV2          `json:"evidence"`
	Kind             ports.NamespacedNameV2             `json:"kind"`
	BindingID        string                             `json:"binding_id"`
	ComponentID      ports.ComponentIDV2                `json:"component_id"`
	ManifestDigest   core.Digest                        `json:"manifest_digest"`
	GrantSetDigest   core.Digest                        `json:"grant_set_digest"`
	SubjectDigest    core.Digest                        `json:"subject_digest"`
	Certifier        ports.EvidenceProducerBindingRefV2 `json:"certifier"`
	SourceEpoch      core.Epoch                         `json:"source_epoch"`
	SourceSequence   uint64                             `json:"source_sequence"`
	ObservedUnixNano int64                              `json:"observed_unix_nano"`
	ExpiresUnixNano  int64                              `json:"expires_unix_nano"`
}

func (a BindingRenewalAttestationV2) Validate() error {
	if err := a.Evidence.Validate(); err != nil {
		return err
	}
	if a.Kind != BindingRenewalAttestationKindV2 || strings.TrimSpace(a.BindingID) == "" || len(a.BindingID) > 128 || a.SourceEpoch == 0 || a.SourceSequence == 0 || a.ObservedUnixNano <= 0 || a.ExpiresUnixNano <= a.ObservedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingNotCertified, "binding renewal attestation identity, sequence and TTL are incomplete")
	}
	if err := ports.ValidateNamespacedNameV2(ports.NamespacedNameV2(a.ComponentID)); err != nil {
		return err
	}
	for _, digest := range []core.Digest{a.ManifestDigest, a.GrantSetDigest, a.SubjectDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := a.Certifier.Validate(); err != nil {
		return err
	}
	if a.Certifier.Capability != BindingRenewalCertifierCapabilityV2 || a.Certifier.ComponentID == a.ComponentID {
		return core.NewError(core.ErrorForbidden, core.ReasonBindingNotCertified, "binding renewal requires an independent certified owner")
	}
	subject, err := BindingRenewalSubjectDigestV2(a.BindingID, a.ComponentID, a.ManifestDigest, a.GrantSetDigest)
	if err != nil || subject != a.SubjectDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "binding renewal attestation subject drifted")
	}
	return nil
}

func BindingRenewalSubjectDigestV2(bindingID string, componentID ports.ComponentIDV2, manifestDigest, grantSetDigest core.Digest) (core.Digest, error) {
	if strings.TrimSpace(bindingID) == "" || len(bindingID) > 128 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "binding renewal subject requires bounded Binding ID")
	}
	if err := ports.ValidateNamespacedNameV2(ports.NamespacedNameV2(componentID)); err != nil {
		return "", err
	}
	if err := manifestDigest.Validate(); err != nil {
		return "", err
	}
	if err := grantSetDigest.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "BindingRenewalSubjectV2", struct {
		BindingID      string              `json:"binding_id"`
		ComponentID    ports.ComponentIDV2 `json:"component_id"`
		ManifestDigest core.Digest         `json:"manifest_digest"`
		GrantSetDigest core.Digest         `json:"grant_set_digest"`
	}{bindingID, componentID, manifestDigest, grantSetDigest})
}

type BindingRenewalAttestationReaderV2 interface {
	InspectBindingRenewalAttestationV2(context.Context, ports.EvidenceRecordRefV2) (BindingRenewalAttestationV2, error)
}

type BindingRenewalPortV2 interface {
	RenewBindingSetV2(context.Context, RenewBindingSetRequestV2) (BindingSetFactV2, error)
}

func BindingGrantSetDigestV2(grants []ports.CapabilityGrantV2) (core.Digest, error) {
	if err := ports.ValidateCapabilityGrantStructureV2(grants); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "BindingGrantSetV2", grants)
}

type BindingCurrentProbeV2 struct {
	ComponentID    ports.ComponentIDV2 `json:"component_id"`
	ManifestDigest core.Digest         `json:"manifest_digest"`
}

type BindingFactPortV2 interface {
	CreateBinding(context.Context, BindingFactV2) (BindingFactV2, error)
	InspectBinding(context.Context, string) (BindingFactV2, error)
	CompareAndSwapBinding(context.Context, BindingFactCASRequestV2) (BindingFactV2, error)
	CommitBindingSet(context.Context, CommitBindingSetRequestV2) (BindingSetFactV2, error)
	InspectBindingSet(context.Context, string) (BindingSetFactV2, error)
	CompareAndSwapBindingSet(context.Context, BindingSetCASRequestV2) (BindingSetFactV2, error)
}

func (f BindingFactV2) Validate() error {
	if f.ID == "" || len(f.ID) > 128 || strings.TrimSpace(f.ID) != f.ID || f.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "binding fact identity and revision are required")
	}
	if err := f.Manifest.Validate(); err != nil {
		return err
	}
	if f.ComponentID != f.Manifest.ComponentID {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "binding component does not match its manifest")
	}
	digest, err := f.Manifest.BindingDigestV2()
	if err != nil {
		return err
	}
	if digest != f.ManifestDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "binding manifest digest does not match the embedded manifest")
	}
	if err := f.GovernanceDigest.Validate(); err != nil {
		return err
	}
	if !validBindingStateV2(f.State) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "binding lifecycle state is unknown")
	}
	if err := validateBindingGrantSetV2(f); err != nil {
		return err
	}
	if len(f.RenewalEvidence) > ports.MaxManifestSetEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "binding renewal evidence exceeds its bounded history")
	}
	for index, evidence := range f.RenewalEvidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
		if index > 0 && evidence.Sequence <= f.RenewalEvidence[index-1].Sequence {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "binding renewal evidence must be append ordered")
		}
	}
	switch f.State {
	case BindingDeclared:
		if len(f.Grants) != 0 || f.ProbedUnixNano != 0 || f.CertifiedUnixNano != 0 || f.ExpiresUnixNano != 0 || f.ConformanceEvidenceDigest != "" || f.BindingSetID != "" || f.InvalidationReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "declared binding cannot contain probe, certification, grant or set facts")
		}
	case BindingProbed:
		if f.ProbedUnixNano <= 0 || f.CertifiedUnixNano != 0 || f.ExpiresUnixNano <= f.ProbedUnixNano || f.ConformanceEvidenceDigest != "" || f.BindingSetID != "" || f.InvalidationReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "probed binding has inconsistent timestamps or authority fields")
		}
	case BindingCertified:
		if f.ProbedUnixNano <= 0 || f.CertifiedUnixNano <= 0 || f.ExpiresUnixNano <= f.ProbedUnixNano || f.BindingSetID != "" || f.InvalidationReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "certified binding has inconsistent timestamps or set fields")
		}
		if err := f.ConformanceEvidenceDigest.Validate(); err != nil {
			return err
		}
		if f.Manifest.Conformance == ports.ConformanceRejected {
			return core.NewError(core.ErrorForbidden, core.ReasonBindingNotCertified, "rejected conformance cannot be certified")
		}
	case BindingBound:
		if f.ProbedUnixNano <= 0 || f.CertifiedUnixNano <= 0 || f.ExpiresUnixNano <= f.ProbedUnixNano || f.BindingSetID == "" || f.InvalidationReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "bound binding requires certification, expiry and a binding set")
		}
		if err := f.ConformanceEvidenceDigest.Validate(); err != nil {
			return err
		}
	case BindingExpired:
		if f.InvalidationReason != core.ReasonBindingExpired && f.InvalidationReason != core.ReasonBindingDrift && f.InvalidationReason != core.ReasonCapabilityExpired && f.InvalidationReason != core.ReasonComponentMissing {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "expired binding requires a machine-readable expiry or drift reason")
		}
	case BindingRevoked:
		if f.InvalidationReason == "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "revoked binding requires a machine-readable reason")
		}
	}
	return nil
}

func (r BindingFactCASRequestV2) Validate(now time.Time) error {
	if now.IsZero() || r.ExpectedRevision == 0 || r.Next.Revision != r.ExpectedRevision+1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "binding CAS requires injected time and the next consecutive revision")
	}
	return r.Next.Validate()
}

func ValidateBindingFactTransitionV2(current, next BindingFactV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "binding transition time must come from an injected clock")
	}
	if current.ID != next.ID || current.ComponentID != next.ComponentID || current.ManifestDigest != next.ManifestDigest || current.GovernanceDigest != next.GovernanceDigest || current.Manifest.ArtifactDigest != next.Manifest.ArtifactDigest || current.Manifest.Contract != next.Manifest.Contract {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "binding identity, manifest, artifact, governance and contract are immutable")
	}
	if next.Revision != current.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "binding revision is stale or skipped")
	}
	if current.State != BindingDeclared && (!reflect.DeepEqual(current.Grants, next.Grants) || current.ProbedUnixNano != next.ProbedUnixNano || current.ExpiresUnixNano != next.ExpiresUnixNano) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "probed grants, probe time and expiry are immutable")
	}
	if current.State == BindingCertified || current.State == BindingBound {
		if current.CertifiedUnixNano != next.CertifiedUnixNano || current.ConformanceEvidenceDigest != next.ConformanceEvidenceDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "certification evidence and time are immutable")
		}
	}
	if now.UnixNano() < current.ProbedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "injected clock regressed before the persisted probe time")
	}
	valid := false
	switch current.State {
	case BindingDeclared:
		valid = next.State == BindingProbed || next.State == BindingRevoked
	case BindingProbed:
		valid = next.State == BindingCertified || next.State == BindingExpired || next.State == BindingRevoked
	case BindingCertified:
		valid = next.State == BindingBound || next.State == BindingExpired || next.State == BindingRevoked
	case BindingBound:
		valid = next.State == BindingExpired || next.State == BindingRevoked
	case BindingExpired, BindingRevoked:
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "expired or revoked binding is terminal")
	}
	if !valid {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "binding lifecycle transition is not allowed")
	}
	if (next.State == BindingCertified || next.State == BindingBound) && !now.Before(time.Unix(0, next.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "expired capability grants cannot be certified or bound")
	}
	if next.State == BindingExpired && now.Before(time.Unix(0, current.ExpiresUnixNano)) && next.InvalidationReason == core.ReasonBindingExpired {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "TTL expiry cannot be asserted before the boundary")
	}
	return nil
}

func (s BindingSetFactV2) Validate() error {
	if s.ID == "" || len(s.ID) > 128 || s.PlanID == "" || s.Revision == 0 || s.CreatedUnixNano <= 0 || s.ExpiresUnixNano <= s.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "binding set identity, revision and bounded lifetime are required")
	}
	if err := s.PlanDigest.Validate(); err != nil {
		return err
	}
	if err := s.GovernanceDigest.Validate(); err != nil {
		return err
	}
	if !validBindingSetStateV2(s.State) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "binding set state is unknown")
	}
	if s.State == BindingSetActive && s.InvalidationReason != "" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "active binding set cannot have an invalidation reason")
	}
	if s.State != BindingSetActive && s.InvalidationReason == "" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "inactive binding set requires a machine-readable reason")
	}
	if len(s.Members) == 0 || len(s.Members) > ports.MaxManifestSetEntries || len(s.TopologicalOrder) != len(s.Members) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "binding set requires a bounded member set and complete topological order")
	}
	members := make(map[ports.ComponentIDV2]BindingMemberV2, len(s.Members))
	minimumExpiry := int64(^uint64(0) >> 1)
	for _, member := range s.Members {
		if member.BindingID == "" || member.BindingRevision == 0 {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "binding member requires a binding fact reference")
		}
		if err := ports.ValidateNamespacedNameV2(ports.NamespacedNameV2(member.ComponentID)); err != nil {
			return err
		}
		for _, digest := range []core.Digest{member.ManifestDigest, member.ArtifactDigest} {
			if err := digest.Validate(); err != nil {
				return err
			}
		}
		if err := member.Contract.Validate(); err != nil {
			return err
		}
		if err := ports.ValidateOwnerAssignmentsV2(member.Owners); err != nil {
			return err
		}
		if err := ports.ValidateCapabilityGrantStructureV2(member.Grants); err != nil {
			return err
		}
		for _, grant := range member.Grants {
			if grant.ExpiresUnixNano < minimumExpiry {
				minimumExpiry = grant.ExpiresUnixNano
			}
		}
		if _, exists := members[member.ComponentID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "binding set contains a duplicate component")
		}
		members[member.ComponentID] = member
	}
	if s.ExpiresUnixNano != minimumExpiry {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "BindingSet expiry must equal its earliest current grant expiry")
	}
	seenOrder := make(map[ports.ComponentIDV2]struct{}, len(s.TopologicalOrder))
	for _, componentID := range s.TopologicalOrder {
		if _, exists := members[componentID]; !exists {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingSetConflict, "topological order references a non-member")
		}
		if _, exists := seenOrder[componentID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "topological order contains a duplicate")
		}
		seenOrder[componentID] = struct{}{}
	}
	return nil
}

func (s BindingSetFactV2) CapabilityGrantDigestV2() (core.Digest, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	type memberGrants struct {
		ComponentID ports.ComponentIDV2       `json:"component_id"`
		Grants      []ports.CapabilityGrantV2 `json:"grants"`
	}
	grants := make([]memberGrants, 0, len(s.Members))
	for _, member := range s.Members {
		grants = append(grants, memberGrants{ComponentID: member.ComponentID, Grants: append([]ports.CapabilityGrantV2{}, member.Grants...)})
	}
	return core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "CapabilityGrantSetV2", grants)
}

func ValidateBindingSetTransitionV2(current, next BindingSetFactV2) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.ID != next.ID || current.PlanID != next.PlanID || current.PlanDigest != next.PlanDigest || current.GovernanceDigest != next.GovernanceDigest || current.CreatedUnixNano != next.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingSetConflict, "binding set identity and plan binding are immutable")
	}
	if next.Revision != current.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "binding set revision is stale or skipped")
	}
	if current.State != BindingSetActive {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "inactive BindingSet is terminal")
	}
	if next.State == BindingSetActive {
		return core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "active BindingSet renewal requires the governed atomic renewal Port")
	}
	if next.State != BindingSetExpired && next.State != BindingSetRevoked {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "BindingSet may renew active or leave active for expiry/revocation")
	}
	if !reflect.DeepEqual(current.Members, next.Members) || !reflect.DeepEqual(current.TopologicalOrder, next.TopologicalOrder) || !reflect.DeepEqual(current.Residuals, next.Residuals) || current.ExpiresUnixNano != next.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingSetConflict, "revocation/expiry cannot mutate BindingSet content")
	}
	return nil
}

// ValidateBindingFactRenewalV2 validates Bound->Bound lease renewal. It never
// certifies a component: the Fact Owner requires a new governed evidence record
// and atomically advances the BindingSet that consumes it.
func ValidateBindingFactRenewalV2(current, next BindingFactV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || current.State != BindingBound || next.State != BindingBound || current.ID != next.ID || next.Revision != current.Revision+1 || current.ComponentID != next.ComponentID || current.ManifestDigest != next.ManifestDigest || current.GovernanceDigest != next.GovernanceDigest || current.BindingSetID != next.BindingSetID || current.ProbedUnixNano != next.ProbedUnixNano || current.CertifiedUnixNano != next.CertifiedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Binding renewal changed immutable certified identity")
	}
	if len(next.RenewalEvidence) != len(current.RenewalEvidence)+1 || !sameEvidenceRecordRefsV2(current.RenewalEvidence, next.RenewalEvidence[:len(current.RenewalEvidence)]) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Binding renewal changed stable Manifest or did not append one evidence record")
	}
	if len(current.Grants) != len(next.Grants) || next.ExpiresUnixNano <= current.ExpiresUnixNano || !now.Before(time.Unix(0, next.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Binding renewal must extend every certified grant TTL")
	}
	for index := range current.Grants {
		oldGrant, newGrant := current.Grants[index], next.Grants[index]
		if oldGrant.Capability != newGrant.Capability || newGrant.ObservedUnixNano < oldGrant.ObservedUnixNano || newGrant.ExpiresUnixNano <= oldGrant.ExpiresUnixNano || newGrant.EvidenceDigest == oldGrant.EvidenceDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Binding renewal changed capability semantics or lacks fresh grant evidence")
		}
	}
	return nil
}

func sameEvidenceRecordRefsV2(left, right []ports.EvidenceRecordRefV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// BindingSetSemanticDigestV2 excludes renewable revisions, evidence watermarks
// and TTLs while retaining every stable component, owner, contract, artifact,
// capability, topology and residual semantic.
func BindingSetSemanticDigestV2(set BindingSetFactV2) (core.Digest, error) {
	if err := set.Validate(); err != nil {
		return "", err
	}
	type semanticMember struct {
		BindingID      string                    `json:"binding_id"`
		ComponentID    ports.ComponentIDV2       `json:"component_id"`
		Kind           ports.ComponentKindV2     `json:"kind"`
		ManifestDigest core.Digest               `json:"manifest_digest"`
		ArtifactDigest core.Digest               `json:"artifact_digest"`
		Contract       ports.ContractBindingV2   `json:"contract"`
		Owners         []ports.OwnerAssignmentV2 `json:"owners"`
		Capabilities   []ports.CapabilityNameV2  `json:"capabilities"`
	}
	members := make([]semanticMember, 0, len(set.Members))
	for _, member := range set.Members {
		capabilities := make([]ports.CapabilityNameV2, 0, len(member.Grants))
		for _, grant := range member.Grants {
			capabilities = append(capabilities, grant.Capability)
		}
		owners := append([]ports.OwnerAssignmentV2{}, member.Owners...)
		sort.Slice(owners, func(i, j int) bool { return owners[i].Role < owners[j].Role })
		sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
		members = append(members, semanticMember{BindingID: member.BindingID, ComponentID: member.ComponentID, Kind: member.Kind, ManifestDigest: member.ManifestDigest, ArtifactDigest: member.ArtifactDigest, Contract: member.Contract, Owners: owners, Capabilities: capabilities})
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ComponentID < members[j].ComponentID })
	value := struct {
		ID               string                `json:"id"`
		PlanID           string                `json:"plan_id"`
		PlanDigest       core.Digest           `json:"plan_digest"`
		GovernanceDigest core.Digest           `json:"governance_digest"`
		Members          []semanticMember      `json:"members"`
		Order            []ports.ComponentIDV2 `json:"topological_order"`
		Residuals        []BindingResidualV2   `json:"residuals"`
	}{set.ID, set.PlanID, set.PlanDigest, set.GovernanceDigest, members, append([]ports.ComponentIDV2{}, set.TopologicalOrder...), append([]BindingResidualV2{}, set.Residuals...)}
	return core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "BindingSetSemanticV2", value)
}

func BuildBindingSetV2(setID string, plan ports.BindingPlanV2, catalog ports.GovernanceCatalogV2, facts []BindingFactV2, now time.Time) (BindingSetFactV2, error) {
	if setID == "" || len(setID) > 128 || now.IsZero() {
		return BindingSetFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "binding set id and injected time are required")
	}
	if err := plan.Validate(); err != nil {
		return BindingSetFactV2{}, err
	}
	catalogDigest, err := catalog.DigestV2()
	if err != nil {
		return BindingSetFactV2{}, err
	}
	if catalogDigest != plan.GovernanceDigest {
		return BindingSetFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "binding plan governance digest drifted")
	}
	factsByComponent := make(map[ports.ComponentIDV2]BindingFactV2, len(facts))
	for _, fact := range facts {
		if err := fact.Validate(); err != nil {
			return BindingSetFactV2{}, err
		}
		if _, exists := factsByComponent[fact.ComponentID]; exists {
			return BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "multiple binding facts exist for one component")
		}
		factsByComponent[fact.ComponentID] = fact
	}
	selected := make(map[ports.ComponentIDV2]BindingFactV2, len(plan.Requirements))
	residuals := make([]BindingResidualV2, 0)
	for _, requirement := range plan.Requirements {
		fact, exists := factsByComponent[requirement.ComponentID]
		reason := core.ReasonComponentMissing
		if exists {
			reason = bindingRequirementFailureV2(requirement, fact, catalog, now)
		}
		if !exists || reason != "" {
			if requirement.Required || !requirement.AllowResidual {
				return BindingSetFactV2{}, core.NewError(core.ErrorCapabilityUnavailable, reason, "required component cannot enter the binding set")
			}
			residuals = append(residuals, BindingResidualV2{ComponentID: requirement.ComponentID, Reason: reason})
			continue
		}
		selected[requirement.ComponentID] = fact
	}
	if len(selected) == 0 {
		return BindingSetFactV2{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "binding set has no certified members")
	}
	graph, err := validateSelectedDependenciesV2(selected)
	if err != nil {
		return BindingSetFactV2{}, err
	}
	order, err := stableTopologicalOrderV2(graph)
	if err != nil {
		return BindingSetFactV2{}, err
	}
	members := make([]BindingMemberV2, 0, len(selected))
	expires := int64(^uint64(0) >> 1)
	for _, componentID := range order {
		fact := selected[componentID]
		if fact.ExpiresUnixNano < expires {
			expires = fact.ExpiresUnixNano
		}
		members = append(members, BindingMemberV2{BindingID: fact.ID, BindingRevision: fact.Revision, ComponentID: fact.ComponentID, Kind: fact.Manifest.Kind, ManifestDigest: fact.ManifestDigest, ArtifactDigest: fact.Manifest.ArtifactDigest, Contract: fact.Manifest.Contract, Owners: append([]ports.OwnerAssignmentV2{}, fact.Manifest.Owners...), Grants: append([]ports.CapabilityGrantV2{}, fact.Grants...)})
	}
	sort.Slice(residuals, func(i, j int) bool { return residuals[i].ComponentID < residuals[j].ComponentID })
	set := BindingSetFactV2{ID: setID, PlanID: plan.ID, PlanDigest: plan.PlanDigest, GovernanceDigest: plan.GovernanceDigest, State: BindingSetActive, Revision: 1, Members: members, TopologicalOrder: order, Residuals: residuals, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	if err := set.Validate(); err != nil {
		return BindingSetFactV2{}, err
	}
	return set, nil
}

func ValidateBindingSetCurrentV2(set BindingSetFactV2, facts []BindingFactV2, probes []BindingCurrentProbeV2, now time.Time) error {
	if err := set.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "binding currentness requires injected time")
	}
	if set.State != BindingSetActive || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "binding set is inactive or expired at the TTL boundary")
	}
	factsByID := make(map[string]BindingFactV2, len(facts))
	for _, fact := range facts {
		factsByID[fact.ID] = fact
	}
	probesByComponent := make(map[ports.ComponentIDV2]core.Digest, len(probes))
	for _, probe := range probes {
		if _, exists := probesByComponent[probe.ComponentID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "current probe set contains a duplicate component")
		}
		probesByComponent[probe.ComponentID] = probe.ManifestDigest
	}
	for _, member := range set.Members {
		fact, exists := factsByID[member.BindingID]
		if !exists || fact.State != BindingBound || fact.BindingSetID != set.ID || fact.Revision != member.BindingRevision || fact.ManifestDigest != member.ManifestDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "binding member fact no longer matches the committed set")
		}
		probeDigest, exists := probesByComponent[member.ComponentID]
		if !exists || probeDigest != member.ManifestDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "current component probe is missing or drifted")
		}
	}
	return nil
}

func bindingRequirementFailureV2(requirement ports.BindingRequirementV2, fact BindingFactV2, catalog ports.GovernanceCatalogV2, now time.Time) core.ReasonCode {
	if fact.State != BindingCertified {
		return core.ReasonBindingNotCertified
	}
	if !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return core.ReasonBindingExpired
	}
	manifest := fact.Manifest
	if manifest.ComponentID != requirement.ComponentID || manifest.Kind != requirement.Kind || manifest.ArtifactDigest != requirement.ArtifactDigest || manifest.Contract.Name != requirement.ContractName || !requirement.SemanticVersion.Contains(manifest.SemanticVersion) || !requirement.Contract.Contains(manifest.Contract.Version) {
		return core.ReasonBindingDrift
	}
	if ports.ValidateManifestAgainstCatalogV2(manifest, catalog) != nil {
		return core.ReasonUnknownGovernanceCategory
	}
	grants := make(map[ports.CapabilityNameV2]ports.CapabilityGrantV2, len(fact.Grants))
	for _, grant := range fact.Grants {
		grants[grant.Capability] = grant
	}
	for _, required := range requirement.RequiredCapabilities {
		grant, exists := grants[required]
		if !exists || !now.Before(time.Unix(0, grant.ExpiresUnixNano)) {
			return core.ReasonUnknownCapability
		}
	}
	return ""
}

func validateSelectedDependenciesV2(selected map[ports.ComponentIDV2]BindingFactV2) (map[ports.ComponentIDV2][]ports.ComponentIDV2, error) {
	graph := make(map[ports.ComponentIDV2][]ports.ComponentIDV2, len(selected))
	for componentID, fact := range selected {
		graph[componentID] = []ports.ComponentIDV2{}
		for _, dependency := range fact.Manifest.Dependencies {
			if _, exists := selected[dependency.ComponentID]; !exists {
				if dependency.Optional {
					continue
				}
				return nil, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "required component dependency is not selected")
			}
			graph[componentID] = append(graph[componentID], dependency.ComponentID)
		}
		for _, requirement := range fact.Manifest.RequiredCapabilities {
			provider, exists := selected[requirement.ProviderComponent]
			if !exists {
				if requirement.Optional {
					continue
				}
				return nil, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "required capability provider is not selected")
			}
			providerGrants := make(map[ports.CapabilityNameV2]struct{}, len(provider.Grants))
			for _, grant := range provider.Grants {
				providerGrants[grant.Capability] = struct{}{}
			}
			if _, exists := providerGrants[requirement.Capability]; !exists && !requirement.Optional {
				return nil, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "required capability has no certified grant")
			}
		}
		for _, owner := range fact.Manifest.Owners {
			if _, exists := selected[owner.OwnerComponentID]; !exists {
				return nil, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonOwnerMissing, "binding owner is outside the selected binding set")
			}
		}
	}
	return graph, nil
}

func stableTopologicalOrderV2(graph map[ports.ComponentIDV2][]ports.ComponentIDV2) ([]ports.ComponentIDV2, error) {
	indegree := make(map[ports.ComponentIDV2]int, len(graph))
	dependents := make(map[ports.ComponentIDV2][]ports.ComponentIDV2, len(graph))
	for node := range graph {
		indegree[node] = 0
	}
	for node, dependencies := range graph {
		for _, dependency := range dependencies {
			indegree[node]++
			dependents[dependency] = append(dependents[dependency], node)
		}
	}
	ready := make([]ports.ComponentIDV2, 0)
	for node, degree := range indegree {
		if degree == 0 {
			ready = append(ready, node)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
	order := make([]ports.ComponentIDV2, 0, len(graph))
	for len(ready) != 0 {
		node := ready[0]
		ready = ready[1:]
		order = append(order, node)
		for _, dependent := range dependents[node] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
			}
		}
	}
	if len(order) != len(graph) {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonDependencyCycle, "component or capability dependency graph contains a cycle")
	}
	return order, nil
}

func validateBindingGrantSetV2(f BindingFactV2) error {
	provided := make(map[ports.CapabilityNameV2]ports.ProvidedCapabilityV2, len(f.Manifest.ProvidedCapabilities))
	for _, capability := range f.Manifest.ProvidedCapabilities {
		provided[capability.Capability] = capability
	}
	seen := make(map[ports.CapabilityNameV2]struct{}, len(f.Grants))
	minimumExpiry := int64(^uint64(0) >> 1)
	var previous ports.CapabilityNameV2
	for index, grant := range f.Grants {
		declaration, exists := provided[grant.Capability]
		if !exists {
			return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "binding grant is not declared by the manifest")
		}
		if _, exists := seen[grant.Capability]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonUnknownCapability, "binding grant set contains a duplicate")
		}
		if index > 0 && grant.Capability < previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "binding grant set must be sorted by capability")
		}
		previous = grant.Capability
		seen[grant.Capability] = struct{}{}
		if err := grant.EvidenceDigest.Validate(); err != nil {
			return err
		}
		if grant.ObservedUnixNano <= 0 || grant.ExpiresUnixNano <= grant.ObservedUnixNano || grant.ExpiresUnixNano-grant.ObservedUnixNano > int64(declaration.TTLSeconds)*int64(time.Second) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "binding grant violates its declared TTL")
		}
		if grant.ExpiresUnixNano < minimumExpiry {
			minimumExpiry = grant.ExpiresUnixNano
		}
	}
	if f.State != BindingDeclared && f.State != BindingRevoked && len(seen) != len(provided) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "probed, certified and bound facts require a grant for every declared capability")
	}
	if len(f.Grants) != 0 && f.ExpiresUnixNano != minimumExpiry {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "binding expiry must equal its earliest capability grant expiry")
	}
	return nil
}

func validBindingStateV2(value BindingLifecycleStateV2) bool {
	switch value {
	case BindingDeclared, BindingProbed, BindingCertified, BindingBound, BindingRevoked, BindingExpired:
		return true
	default:
		return false
	}
}

func validBindingSetStateV2(value BindingSetStateV2) bool {
	return value == BindingSetActive || value == BindingSetRevoked || value == BindingSetExpired
}
