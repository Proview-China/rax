package control

import (
	"context"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const BindingAdmissionAttemptContractVersionV1 = "praxis.runtime.binding-admission-attempt/v1"

type BindingAdmissionAttemptStateV1 string

const (
	BindingAdmissionIntentRecordedV1         BindingAdmissionAttemptStateV1 = "intent_recorded"
	BindingAdmissionResultRecordedV1         BindingAdmissionAttemptStateV1 = "result_recorded"
	BindingAdmissionOutcomeUnknownV1         BindingAdmissionAttemptStateV1 = "outcome_unknown"
	BindingAdmissionReconciliationRequiredV1 BindingAdmissionAttemptStateV1 = "reconciliation_required"
)

type BindingAdmissionInputSnapshotV1 struct {
	Definition     ports.OwnerCurrentRefV1                  `json:"definition"`
	Plan           ports.BindingAdmissionPlanCurrentV1      `json:"plan"`
	Assembly       ports.BindingAdmissionAssemblyCurrentV1  `json:"assembly"`
	Catalog        ports.BindingAdmissionCatalogCurrentV1   `json:"catalog"`
	Resolution     ports.OwnerCurrentRefV1                  `json:"resolution"`
	Releases       []ports.BindingAdmissionReleaseCurrentV1 `json:"releases"`
	Resources      ports.ResourceBindingSetV1               `json:"resources"`
	Authority      ports.OwnerCurrentRefV1                  `json:"authority"`
	Policy         ports.OwnerCurrentRefV1                  `json:"policy"`
	SnapshotDigest core.Digest                              `json:"snapshot_digest"`
}

func (s BindingAdmissionInputSnapshotV1) canonicalV1() BindingAdmissionInputSnapshotV1 {
	c := s
	c.Releases = append([]ports.BindingAdmissionReleaseCurrentV1{}, s.Releases...)
	sort.Slice(c.Releases, func(i, j int) bool { return c.Releases[i].Expected.ComponentID < c.Releases[j].Expected.ComponentID })
	return c
}

func (s BindingAdmissionInputSnapshotV1) DigestV1() (core.Digest, error) {
	c := s.canonicalV1()
	c.SnapshotDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.binding-admission-attempt", BindingAdmissionAttemptContractVersionV1, "BindingAdmissionInputSnapshotV1", c)
}

func SealBindingAdmissionInputSnapshotV1(s BindingAdmissionInputSnapshotV1) (BindingAdmissionInputSnapshotV1, error) {
	s = s.canonicalV1()
	provided := s.SnapshotDigest
	s.SnapshotDigest = ""
	d, err := s.DigestV1()
	if err != nil {
		return BindingAdmissionInputSnapshotV1{}, err
	}
	if provided != "" && provided != d {
		return BindingAdmissionInputSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission input snapshot supplied a wrong digest")
	}
	s.SnapshotDigest = d
	return s, s.Validate()
}

func (s BindingAdmissionInputSnapshotV1) Validate() error {
	for _, ref := range []ports.OwnerCurrentRefV1{s.Definition, s.Resolution, s.Authority, s.Policy} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := s.Plan.Validate(); err != nil {
		return err
	}
	if err := s.Assembly.Validate(); err != nil {
		return err
	}
	if err := s.Catalog.Validate(); err != nil {
		return err
	}
	if err := s.Resources.Validate(); err != nil {
		return err
	}
	if len(s.Releases) == 0 || len(s.Releases) > ports.MaxBindingAdmissionReleasesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission snapshot release set is incomplete")
	}
	for i, release := range s.Releases {
		if err := release.Validate(); err != nil {
			return err
		}
		if i > 0 && s.Releases[i-1].Expected.ComponentID >= release.Expected.ComponentID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Binding admission snapshot releases must be canonical")
		}
	}
	d, err := s.DigestV1()
	if err != nil || d != s.SnapshotDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission input snapshot digest drifted")
	}
	return nil
}

func (s BindingAdmissionInputSnapshotV1) ValidateAgainstRequestV1(r ports.BindingAdmissionRequestV1) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Definition != r.DefinitionCurrent || s.Plan.Ref != r.PlanCurrent || s.Assembly.Ref != r.AssemblyCurrent || s.Catalog.Ref != r.CatalogCurrent || s.Resolution != r.ResolutionCurrent || s.Resources.Ref != r.ResourceBindingSet || s.Authority != r.AuthorityCurrent || s.Policy != r.PolicyCurrent || len(s.Releases) != len(r.Releases) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding admission input snapshot spliced request coordinates")
	}
	for i := range r.Releases {
		if s.Releases[i].Expected != r.Releases[i] {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding admission release snapshot drifted")
		}
	}
	return nil
}

type BindingAdmissionAttemptFactV1 struct {
	ContractVersion              string                          `json:"contract_version"`
	AttemptID                    string                          `json:"attempt_id"`
	Revision                     core.Revision                   `json:"revision"`
	Request                      ports.BindingAdmissionRequestV1 `json:"request"`
	Inputs                       BindingAdmissionInputSnapshotV1 `json:"inputs"`
	DeclaredCandidates           []BindingFactV2                 `json:"declared_candidates"`
	ProbedCandidates             []BindingFactV2                 `json:"probed_candidates"`
	CertifiedCandidates          []BindingFactV2                 `json:"certified_candidates"`
	BindingSetCandidate          BindingSetFactV2                `json:"binding_set_candidate"`
	CommittedBindingSetCandidate BindingSetFactV2                `json:"committed_binding_set_candidate"`
	State                        BindingAdmissionAttemptStateV1  `json:"state"`
	Result                       *ports.BindingAdmissionResultV1 `json:"result,omitempty"`
	CreatedUnixNano              int64                           `json:"created_unix_nano"`
	UpdatedUnixNano              int64                           `json:"updated_unix_nano"`
	Digest                       core.Digest                     `json:"digest"`
}

func cloneBindingAdmissionFactSliceV1(values []BindingFactV2) []BindingFactV2 {
	cloned := append([]BindingFactV2{}, values...)
	for i := range cloned {
		cloned[i].Manifest.Schemas = append([]ports.SchemaRefV2{}, cloned[i].Manifest.Schemas...)
		cloned[i].Manifest.Dependencies = append([]ports.ComponentDependencyV2{}, cloned[i].Manifest.Dependencies...)
		cloned[i].Manifest.RequiredCapabilities = append([]ports.CapabilityRequirementV2{}, cloned[i].Manifest.RequiredCapabilities...)
		cloned[i].Manifest.ProvidedCapabilities = append([]ports.ProvidedCapabilityV2{}, cloned[i].Manifest.ProvidedCapabilities...)
		for j := range cloned[i].Manifest.ProvidedCapabilities {
			cloned[i].Manifest.ProvidedCapabilities[j].Schemas = append([]ports.SchemaRefV2{}, cloned[i].Manifest.ProvidedCapabilities[j].Schemas...)
		}
		cloned[i].Manifest.Owners = append([]ports.OwnerAssignmentV2{}, cloned[i].Manifest.Owners...)
		cloned[i].Manifest.Credentials = append([]ports.CredentialRequirementV2{}, cloned[i].Manifest.Credentials...)
		cloned[i].Manifest.Extensions = append([]ports.GovernanceExtensionV2{}, cloned[i].Manifest.Extensions...)
		for j := range cloned[i].Manifest.Extensions {
			cloned[i].Manifest.Extensions[j].Payload.Inline = append([]byte{}, cloned[i].Manifest.Extensions[j].Payload.Inline...)
		}
		cloned[i].Manifest.Annotations = append([]ports.DisplayAnnotationV2{}, cloned[i].Manifest.Annotations...)
		cloned[i].Grants = append([]ports.CapabilityGrantV2{}, cloned[i].Grants...)
		cloned[i].RenewalEvidence = append([]ports.EvidenceRecordRefV2{}, cloned[i].RenewalEvidence...)
	}
	return cloned
}

func (a BindingAdmissionAttemptFactV1) CloneV1() BindingAdmissionAttemptFactV1 {
	c := a
	c.Request.Releases = append([]ports.PreBindingComponentReleaseV1{}, a.Request.Releases...)
	c.Inputs = a.Inputs.canonicalV1()
	c.DeclaredCandidates = cloneBindingAdmissionFactSliceV1(a.DeclaredCandidates)
	c.ProbedCandidates = cloneBindingAdmissionFactSliceV1(a.ProbedCandidates)
	c.CertifiedCandidates = cloneBindingAdmissionFactSliceV1(a.CertifiedCandidates)
	c.BindingSetCandidate.Members = append([]BindingMemberV2{}, a.BindingSetCandidate.Members...)
	c.BindingSetCandidate.TopologicalOrder = append([]ports.ComponentIDV2{}, a.BindingSetCandidate.TopologicalOrder...)
	c.BindingSetCandidate.Residuals = append([]BindingResidualV2{}, a.BindingSetCandidate.Residuals...)
	c.CommittedBindingSetCandidate.Members = append([]BindingMemberV2{}, a.CommittedBindingSetCandidate.Members...)
	c.CommittedBindingSetCandidate.TopologicalOrder = append([]ports.ComponentIDV2{}, a.CommittedBindingSetCandidate.TopologicalOrder...)
	c.CommittedBindingSetCandidate.Residuals = append([]BindingResidualV2{}, a.CommittedBindingSetCandidate.Residuals...)
	if a.Result != nil {
		x := *a.Result
		x.Bindings = append([]ports.BindingAdmissionBindingRefV1{}, a.Result.Bindings...)
		c.Result = &x
	}
	return c
}

func (a BindingAdmissionAttemptFactV1) DigestV1() (core.Digest, error) {
	c := a.CloneV1()
	c.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.binding-admission-attempt", BindingAdmissionAttemptContractVersionV1, "BindingAdmissionAttemptFactV1", c)
}

func SealBindingAdmissionAttemptFactV1(a BindingAdmissionAttemptFactV1) (BindingAdmissionAttemptFactV1, error) {
	a = a.CloneV1()
	a.ContractVersion = BindingAdmissionAttemptContractVersionV1
	provided := a.Digest
	a.Digest = ""
	d, err := a.DigestV1()
	if err != nil {
		return BindingAdmissionAttemptFactV1{}, err
	}
	if provided != "" && provided != d {
		return BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission attempt supplied a wrong digest")
	}
	a.Digest = d
	return a, a.Validate()
}

func (a BindingAdmissionAttemptFactV1) Validate() error {
	if a.ContractVersion != BindingAdmissionAttemptContractVersionV1 || a.AttemptID != a.Request.AttemptID || a.Revision == 0 || a.CreatedUnixNano <= 0 || a.UpdatedUnixNano < a.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission attempt is incomplete")
	}
	if err := a.Request.Validate(); err != nil {
		return err
	}
	if err := a.Inputs.ValidateAgainstRequestV1(a.Request); err != nil {
		return err
	}
	if len(a.DeclaredCandidates) != len(a.ProbedCandidates) || len(a.ProbedCandidates) != len(a.CertifiedCandidates) || len(a.CertifiedCandidates) != len(a.Request.Releases) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Binding admission candidate lifecycle sets differ")
	}
	for i := range a.CertifiedCandidates {
		d, p, c := a.DeclaredCandidates[i], a.ProbedCandidates[i], a.CertifiedCandidates[i]
		if err := d.Validate(); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		if err := c.Validate(); err != nil {
			return err
		}
		if d.State != BindingDeclared || d.Revision != 1 || p.State != BindingProbed || p.Revision != 2 || c.State != BindingCertified || c.Revision != 3 || d.ID != p.ID || p.ID != c.ID || d.ComponentID != a.Request.Releases[i].ComponentID || d.ManifestDigest != p.ManifestDigest || p.ManifestDigest != c.ManifestDigest {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding admission lifecycle candidates drifted")
		}
	}
	if err := a.BindingSetCandidate.Validate(); err != nil {
		return err
	}
	if err := a.CommittedBindingSetCandidate.Validate(); err != nil {
		return err
	}
	if a.BindingSetCandidate.ID != a.Request.ExpectedBindingSetID || a.BindingSetCandidate.PlanDigest != a.Inputs.Plan.Plan.PlanDigest || a.BindingSetCandidate.GovernanceDigest != a.Inputs.Plan.Plan.GovernanceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Binding admission BindingSet candidate drifted")
	}
	if len(a.BindingSetCandidate.Members) != len(a.CommittedBindingSetCandidate.Members) || a.BindingSetCandidate.ID != a.CommittedBindingSetCandidate.ID || a.BindingSetCandidate.PlanDigest != a.CommittedBindingSetCandidate.PlanDigest || a.BindingSetCandidate.GovernanceDigest != a.CommittedBindingSetCandidate.GovernanceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Binding admission committed BindingSet candidate drifted")
	}
	for i := range a.BindingSetCandidate.Members {
		before, after := a.BindingSetCandidate.Members[i], a.CommittedBindingSetCandidate.Members[i]
		if before.BindingID != after.BindingID || before.ComponentID != after.ComponentID || before.ManifestDigest != after.ManifestDigest || before.BindingRevision+1 != after.BindingRevision {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Binding admission committed member watermark drifted")
		}
	}
	switch a.State {
	case BindingAdmissionIntentRecordedV1, BindingAdmissionOutcomeUnknownV1, BindingAdmissionReconciliationRequiredV1:
		if a.Result != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "non-terminal Binding admission attempt cannot carry a result")
		}
	case BindingAdmissionResultRecordedV1:
		if a.Result == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "terminal Binding admission attempt requires a result")
		}
		if err := a.Result.Validate(); err != nil {
			return err
		}
		setDigest, digestErr := BindingSetFactContentDigestV2(a.CommittedBindingSetCandidate)
		if digestErr != nil {
			return digestErr
		}
		if a.Result.AttemptID != a.AttemptID || a.Result.RequestDigest != a.Request.RequestDigest || a.Result.BindingSet.ID != a.CommittedBindingSetCandidate.ID || a.Result.BindingSet.Revision != a.CommittedBindingSetCandidate.Revision || a.Result.BindingSet.Digest != setDigest || a.Result.BindingSet.ExpiresUnixNano != a.CommittedBindingSetCandidate.ExpiresUnixNano || a.Result.ResourceBindingSet != a.Request.ResourceBindingSet || len(a.Result.Bindings) != len(a.CertifiedCandidates) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Binding admission attempt result drifted")
		}
		for i, certified := range a.CertifiedCandidates {
			bound := certified
			bound.State = BindingBound
			bound.Revision++
			bound.BindingSetID = a.CommittedBindingSetCandidate.ID
			bindingDigest, digestErr := BindingFactContentDigestV2(bound)
			if digestErr != nil {
				return digestErr
			}
			ref := a.Result.Bindings[i]
			if ref.ComponentID != bound.ComponentID || ref.ID != bound.ID || ref.Revision != bound.Revision || ref.Digest != bindingDigest || ref.ExpiresUnixNano != bound.ExpiresUnixNano {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding admission result member drifted")
			}
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Binding admission attempt state is unsupported")
	}
	d, err := a.DigestV1()
	if err != nil || d != a.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission attempt digest drifted")
	}
	return nil
}

func ValidateBindingAdmissionAttemptSuccessorV1(current, next BindingAdmissionAttemptFactV1) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.AttemptID != next.AttemptID || current.Request.RequestDigest != next.Request.RequestDigest || current.Inputs.SnapshotDigest != next.Inputs.SnapshotDigest || current.BindingSetCandidate.ID != next.BindingSetCandidate.ID || current.CommittedBindingSetCandidate.ID != next.CommittedBindingSetCandidate.ID || current.CreatedUnixNano != next.CreatedUnixNano || next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Binding admission attempt immutable identity or revision drifted")
	}
	allowed := current.State == BindingAdmissionIntentRecordedV1 && (next.State == BindingAdmissionResultRecordedV1 || next.State == BindingAdmissionOutcomeUnknownV1) || current.State == BindingAdmissionOutcomeUnknownV1 && (next.State == BindingAdmissionResultRecordedV1 || next.State == BindingAdmissionReconciliationRequiredV1) || current.State == BindingAdmissionReconciliationRequiredV1 && (next.State == BindingAdmissionResultRecordedV1 || next.State == BindingAdmissionReconciliationRequiredV1)
	if !allowed {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Binding admission attempt transition is not allowed")
	}
	return nil
}

type BindingAdmissionAttemptCASRequestV1 struct {
	ExpectedRevision core.Revision                 `json:"expected_revision"`
	ExpectedDigest   core.Digest                   `json:"expected_digest"`
	Next             BindingAdmissionAttemptFactV1 `json:"next"`
}

type BindingAdmissionAttemptFactPortV1 interface {
	CreateBindingAdmissionAttemptV1(context.Context, BindingAdmissionAttemptFactV1) (BindingAdmissionAttemptFactV1, error)
	CompareAndSwapBindingAdmissionAttemptV1(context.Context, BindingAdmissionAttemptCASRequestV1) (BindingAdmissionAttemptFactV1, error)
	InspectBindingAdmissionAttemptV1(context.Context, string) (BindingAdmissionAttemptFactV1, error)
}

func BindingFactContentDigestV2(f BindingFactV2) (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	payload, err := ports.EncodeComponentManifestV2(f.Manifest)
	if err != nil {
		return "", err
	}
	manifest, err := ports.DecodeComponentManifestV2(payload)
	if err != nil {
		return "", err
	}
	f.Manifest = manifest
	if f.Grants == nil {
		f.Grants = []ports.CapabilityGrantV2{}
	}
	if f.RenewalEvidence == nil {
		f.RenewalEvidence = []ports.EvidenceRecordRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.binding-fact-content", ports.BindingContractVersionV2, "BindingFactV2", f)
}

func BindingSetFactContentDigestV2(s BindingSetFactV2) (core.Digest, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	if s.Members == nil {
		s.Members = []BindingMemberV2{}
	}
	for i := range s.Members {
		if s.Members[i].Owners == nil {
			s.Members[i].Owners = []ports.OwnerAssignmentV2{}
		}
		if s.Members[i].Grants == nil {
			s.Members[i].Grants = []ports.CapabilityGrantV2{}
		}
	}
	if s.TopologicalOrder == nil {
		s.TopologicalOrder = []ports.ComponentIDV2{}
	}
	if s.Residuals == nil {
		s.Residuals = []BindingResidualV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.binding-set-fact-content", ports.BindingContractVersionV2, "BindingSetFactV2", s)
}
