package ports

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	GenerationBindingAssociationContractVersionV1 = "1.0.0"
	MaxGenerationBindingComponentsV1              = 256
)

type GenerationArtifactRefV1 struct {
	ID             string        `json:"generation_id"`
	Revision       core.Revision `json:"generation_revision"`
	Digest         core.Digest   `json:"generation_digest"`
	InputDigest    core.Digest   `json:"input_digest"`
	ManifestDigest core.Digest   `json:"manifest_digest"`
	GraphDigest    core.Digest   `json:"graph_digest"`
	CatalogDigest  core.Digest   `json:"catalog_digest"`
}

func (r GenerationArtifactRefV1) Validate() error {
	if validateGenerationBindingIDV1(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "generation identity and revision are required")
	}
	for _, digest := range []core.Digest{r.Digest, r.InputDigest, r.ManifestDigest, r.GraphDigest, r.CatalogDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type GenerationComponentManifestRefV1 struct {
	ComponentID    ComponentIDV2 `json:"component_id"`
	ManifestDigest core.Digest   `json:"manifest_digest"`
	ArtifactDigest core.Digest   `json:"artifact_digest"`
}

func (r GenerationComponentManifestRefV1) Validate() error {
	if err := ValidateNamespacedNameV2(NamespacedNameV2(r.ComponentID)); err != nil {
		return err
	}
	if err := r.ManifestDigest.Validate(); err != nil {
		return err
	}
	return r.ArtifactDigest.Validate()
}

type GenerationGovernanceExtensionRefV1 struct {
	Kind     NamespacedNameV2 `json:"kind"`
	Contract SchemaRefV2      `json:"contract"`
	Digest   core.Digest      `json:"digest"`
}

func (r GenerationGovernanceExtensionRefV1) Validate() error {
	if err := ValidateNamespacedNameV2(r.Kind); err != nil {
		return err
	}
	if err := r.Contract.Validate(); err != nil {
		return err
	}
	return r.Digest.Validate()
}

type GenerationCurrentStateV1 string

const GenerationCurrentSealedV1 GenerationCurrentStateV1 = "sealed"

// GenerationCurrentProjectionV1 is supplied by a host-owned adapter over a
// sealed generation. It is evidence for association, never a Binding Fact.
type GenerationCurrentProjectionV1 struct {
	ContractVersion    string                             `json:"contract_version"`
	Generation         GenerationArtifactRefV1            `json:"generation"`
	ComponentManifests []GenerationComponentManifestRefV1 `json:"component_manifests"`
	Extension          GenerationGovernanceExtensionRefV1 `json:"governance_extension"`
	State              GenerationCurrentStateV1           `json:"state"`
	Current            bool                               `json:"current"`
	Watermark          core.Revision                      `json:"watermark"`
	ProjectionDigest   core.Digest                        `json:"projection_digest"`
	ExpiresUnixNano    int64                              `json:"expires_unix_nano"`
}

func (p GenerationCurrentProjectionV1) Validate() error {
	if p.ContractVersion != GenerationBindingAssociationContractVersionV1 || p.Generation.Validate() != nil || p.Extension.Validate() != nil || p.State != GenerationCurrentSealedV1 || p.Watermark == 0 || p.ExpiresUnixNano <= 0 || len(p.ComponentManifests) == 0 || len(p.ComponentManifests) > MaxGenerationBindingComponentsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "generation current projection is incomplete")
	}
	var previous ComponentIDV2
	for index, component := range p.ComponentManifests {
		if err := component.Validate(); err != nil {
			return err
		}
		if index > 0 && component.ComponentID <= previous {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "generation component manifests must be sorted and unique")
		}
		previous = component.ComponentID
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "generation current projection digest drifted")
	}
	return nil
}

func (p GenerationCurrentProjectionV1) ValidateCurrent(expected GenerationArtifactRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !p.Current || p.Generation != expected || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "generation is no longer current")
	}
	return nil
}

func (p GenerationCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	if copy.ComponentManifests == nil {
		copy.ComponentManifests = []GenerationComponentManifestRefV1{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.generation-binding", GenerationBindingAssociationContractVersionV1, "GenerationCurrentProjectionV1", copy)
}

func SealGenerationCurrentProjectionV1(p GenerationCurrentProjectionV1) (GenerationCurrentProjectionV1, error) {
	p.ContractVersion = GenerationBindingAssociationContractVersionV1
	p.ComponentManifests = append([]GenerationComponentManifestRefV1{}, p.ComponentManifests...)
	sort.Slice(p.ComponentManifests, func(i, j int) bool { return p.ComponentManifests[i].ComponentID < p.ComponentManifests[j].ComponentID })
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return GenerationCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type GenerationBindingSetCurrentProjectionV1 struct {
	ContractVersion            string        `json:"contract_version"`
	BindingSetID               string        `json:"binding_set_id"`
	BindingSetRevision         core.Revision `json:"binding_set_revision"`
	BindingSetDigest           core.Digest   `json:"binding_set_digest"`
	BindingSetSemanticDigest   core.Digest   `json:"binding_set_semantic_digest"`
	PlanDigest                 core.Digest   `json:"plan_digest"`
	GovernanceDigest           core.Digest   `json:"governance_digest"`
	ComponentManifestSetDigest core.Digest   `json:"component_manifest_set_digest"`
	CurrentnessDigest          core.Digest   `json:"currentness_digest"`
	ProjectionDigest           core.Digest   `json:"projection_digest"`
	IssuedUnixNano             int64         `json:"issued_unix_nano"`
	ExpiresUnixNano            int64         `json:"expires_unix_nano"`
}

func (p GenerationBindingSetCurrentProjectionV1) Validate() error {
	if p.ContractVersion != GenerationBindingAssociationContractVersionV1 || validateGenerationBindingIDV1(p.BindingSetID) != nil || p.BindingSetRevision == 0 || p.IssuedUnixNano <= 0 || p.ExpiresUnixNano <= p.IssuedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingDrift, "generation BindingSet current projection is incomplete")
	}
	for _, digest := range []core.Digest{p.BindingSetDigest, p.BindingSetSemanticDigest, p.PlanDigest, p.GovernanceDigest, p.ComponentManifestSetDigest, p.CurrentnessDigest, p.ProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "generation BindingSet projection digest drifted")
	}
	return nil
}

func (p GenerationBindingSetCurrentProjectionV1) ValidateCurrent(expectedID string, expectedRevision core.Revision, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || p.BindingSetID != expectedID || p.BindingSetRevision != expectedRevision || now.Before(time.Unix(0, p.IssuedUnixNano)) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "generation BindingSet is inactive, expired or drifted")
	}
	return nil
}

func (p GenerationBindingSetCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.generation-binding", GenerationBindingAssociationContractVersionV1, "GenerationBindingSetCurrentProjectionV1", copy)
}

func SealGenerationBindingSetCurrentProjectionV1(p GenerationBindingSetCurrentProjectionV1) (GenerationBindingSetCurrentProjectionV1, error) {
	p.ContractVersion = GenerationBindingAssociationContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return GenerationBindingSetCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type GenerationActivationCurrentProjectionV1 struct {
	ContractVersion   string             `json:"contract_version"`
	Operation         OperationSubjectV3 `json:"operation"`
	OperationDigest   core.Digest        `json:"operation_digest"`
	Active            bool               `json:"active"`
	Watermark         core.Revision      `json:"watermark"`
	CurrentnessDigest core.Digest        `json:"currentness_digest"`
	ProjectionDigest  core.Digest        `json:"projection_digest"`
	ExpiresUnixNano   int64              `json:"expires_unix_nano"`
}

func (p GenerationActivationCurrentProjectionV1) Validate() error {
	if p.ContractVersion != GenerationBindingAssociationContractVersionV1 || p.Operation.Validate() != nil || p.Operation.Kind != OperationScopeActivationV3 || p.Watermark == 0 || p.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonActivationFactDrift, "generation activation projection is incomplete")
	}
	operationDigest, err := p.Operation.DigestV3()
	if err != nil || operationDigest != p.OperationDigest || p.CurrentnessDigest.Validate() != nil || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonActivationFactDrift, "generation activation projection binding drifted")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonActivationFactDrift, "generation activation projection digest drifted")
	}
	return nil
}

func (p GenerationActivationCurrentProjectionV1) ValidateCurrent(expected OperationSubjectV3, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !p.Active || !SameOperationSubjectV3(p.Operation, expected) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonActivationFactDrift, "generation activation scope is inactive, expired or drifted")
	}
	return nil
}

func (p GenerationActivationCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.generation-binding", GenerationBindingAssociationContractVersionV1, "GenerationActivationCurrentProjectionV1", copy)
}

func SealGenerationActivationCurrentProjectionV1(p GenerationActivationCurrentProjectionV1) (GenerationActivationCurrentProjectionV1, error) {
	p.ContractVersion = GenerationBindingAssociationContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return GenerationActivationCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

// GenerationBindingAssociationCandidateV1 is an immutable handoff candidate.
// It is not a Binding Fact and cannot activate a generation by itself.
type GenerationBindingAssociationCandidateV1 struct {
	ContractVersion          string                                  `json:"contract_version"`
	AssociationID            string                                  `json:"association_id"`
	Generation               GenerationCurrentProjectionV1           `json:"generation"`
	Binding                  GenerationBindingSetCurrentProjectionV1 `json:"binding"`
	Activation               GenerationActivationCurrentProjectionV1 `json:"activation"`
	RequestedExpiresUnixNano int64                                   `json:"requested_expires_unix_nano"`
	Digest                   core.Digest                             `json:"digest"`
}

func (c GenerationBindingAssociationCandidateV1) Validate() error {
	if c.ContractVersion != GenerationBindingAssociationContractVersionV1 || validateGenerationBindingIDV1(c.AssociationID) != nil || c.Generation.Validate() != nil || c.Binding.Validate() != nil || c.Activation.Validate() != nil || c.RequestedExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "generation binding association candidate is incomplete")
	}
	if c.Binding.ComponentManifestSetDigest != GenerationComponentManifestSetDigestV1(c.Generation.ComponentManifests) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "generation component manifest set does not match BindingSet projection")
	}
	digest, err := c.DigestV1()
	if err != nil || digest != c.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "generation binding candidate digest drifted")
	}
	return nil
}

func (c GenerationBindingAssociationCandidateV1) DigestV1() (core.Digest, error) {
	copy := c
	copy.Digest = ""
	if copy.Generation.ComponentManifests == nil {
		copy.Generation.ComponentManifests = []GenerationComponentManifestRefV1{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.generation-binding", GenerationBindingAssociationContractVersionV1, "GenerationBindingAssociationCandidateV1", copy)
}

func SealGenerationBindingAssociationCandidateV1(c GenerationBindingAssociationCandidateV1) (GenerationBindingAssociationCandidateV1, error) {
	c.ContractVersion = GenerationBindingAssociationContractVersionV1
	c.Digest = ""
	digest, err := c.DigestV1()
	if err != nil {
		return GenerationBindingAssociationCandidateV1{}, err
	}
	c.Digest = digest
	return c, c.Validate()
}

type GenerationBindingAssociationStateV1 string

const (
	GenerationBindingAssociationActiveV1  GenerationBindingAssociationStateV1 = "active"
	GenerationBindingAssociationRevokedV1 GenerationBindingAssociationStateV1 = "revoked"
	GenerationBindingAssociationExpiredV1 GenerationBindingAssociationStateV1 = "expired"
)

type GenerationBindingAssociationFactV1 struct {
	ContractVersion    string                                  `json:"contract_version"`
	ID                 string                                  `json:"id"`
	Revision           core.Revision                           `json:"revision"`
	State              GenerationBindingAssociationStateV1     `json:"state"`
	Candidate          GenerationBindingAssociationCandidateV1 `json:"candidate"`
	CandidateDigest    core.Digest                             `json:"candidate_digest"`
	CreatedUnixNano    int64                                   `json:"created_unix_nano"`
	UpdatedUnixNano    int64                                   `json:"updated_unix_nano"`
	ExpiresUnixNano    int64                                   `json:"expires_unix_nano"`
	InvalidationReason core.ReasonCode                         `json:"invalidation_reason,omitempty"`
	Digest             core.Digest                             `json:"digest"`
}

func (f GenerationBindingAssociationFactV1) Validate() error {
	if f.ContractVersion != GenerationBindingAssociationContractVersionV1 || f.ID != f.Candidate.AssociationID || f.Revision == 0 || f.Candidate.Validate() != nil || f.CandidateDigest != f.Candidate.Digest || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "generation binding association fact is incomplete")
	}
	expires := minimumGenerationBindingExpiryV1(f.Candidate)
	if f.ExpiresUnixNano != expires {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "association expiry must equal the earliest bound currentness expiry")
	}
	switch f.State {
	case GenerationBindingAssociationActiveV1:
		if f.InvalidationReason != "" || f.UpdatedUnixNano >= f.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "active association is invalidated or expired")
		}
	case GenerationBindingAssociationRevokedV1:
		if f.InvalidationReason == "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "revoked association requires a reason")
		}
	case GenerationBindingAssociationExpiredV1:
		if f.InvalidationReason != core.ReasonBindingExpired || f.UpdatedUnixNano < f.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "expired association must cross the exact TTL boundary")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown generation binding association state")
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "generation binding association fact digest drifted")
	}
	return nil
}

func (f GenerationBindingAssociationFactV1) DigestV1() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	if copy.Candidate.Generation.ComponentManifests == nil {
		copy.Candidate.Generation.ComponentManifests = []GenerationComponentManifestRefV1{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.generation-binding", GenerationBindingAssociationContractVersionV1, "GenerationBindingAssociationFactV1", copy)
}

func SealGenerationBindingAssociationFactV1(f GenerationBindingAssociationFactV1) (GenerationBindingAssociationFactV1, error) {
	f.ContractVersion = GenerationBindingAssociationContractVersionV1
	f.Digest = ""
	digest, err := f.DigestV1()
	if err != nil {
		return GenerationBindingAssociationFactV1{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

type GenerationBindingAssociationRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r GenerationBindingAssociationRefV1) Validate() error {
	if validateGenerationBindingIDV1(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "generation binding association reference is incomplete")
	}
	return r.Digest.Validate()
}

func (f GenerationBindingAssociationFactV1) RefV1() GenerationBindingAssociationRefV1 {
	return GenerationBindingAssociationRefV1{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

type GenerationBindingAssociationCASRequestV1 struct {
	ExpectedRevision core.Revision                      `json:"expected_revision"`
	Next             GenerationBindingAssociationFactV1 `json:"next"`
}

func ValidateGenerationBindingAssociationTransitionV1(current, next GenerationBindingAssociationFactV1, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "association transition clock regressed")
	}
	if current.ID != next.ID || current.CandidateDigest != next.CandidateDigest || current.Candidate.Digest != next.Candidate.Digest || current.CreatedUnixNano != next.CreatedUnixNano || current.ExpiresUnixNano != next.ExpiresUnixNano || next.Revision != current.Revision+1 || next.UpdatedUnixNano != now.UnixNano() {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "association immutable content or revision drifted")
	}
	if current.State != GenerationBindingAssociationActiveV1 || (next.State != GenerationBindingAssociationRevokedV1 && next.State != GenerationBindingAssociationExpiredV1) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "association only permits active to revoked or expired")
	}
	if next.State == GenerationBindingAssociationExpiredV1 && now.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "association cannot expire before its TTL boundary")
	}
	return nil
}

func NextGenerationBindingAssociationStateV1(current GenerationBindingAssociationFactV1, state GenerationBindingAssociationStateV1, reason core.ReasonCode, now time.Time) (GenerationBindingAssociationFactV1, error) {
	next := current
	next.Revision++
	next.State = state
	next.InvalidationReason = reason
	next.UpdatedUnixNano = now.UnixNano()
	return SealGenerationBindingAssociationFactV1(next)
}

type GenerationBindingAssociationFactPortV1 interface {
	CreateGenerationBindingAssociationV1(context.Context, GenerationBindingAssociationFactV1) (GenerationBindingAssociationFactV1, error)
	InspectGenerationBindingAssociationV1(context.Context, string) (GenerationBindingAssociationFactV1, error)
	CompareAndSwapGenerationBindingAssociationV1(context.Context, GenerationBindingAssociationCASRequestV1) (GenerationBindingAssociationFactV1, error)
}

type GenerationCurrentReaderV1 interface {
	InspectGenerationCurrentV1(context.Context, GenerationArtifactRefV1) (GenerationCurrentProjectionV1, error)
}

type GenerationActivationCurrentReaderV1 interface {
	InspectGenerationActivationCurrentV1(context.Context, OperationSubjectV3) (GenerationActivationCurrentProjectionV1, error)
}

// GenerationBindingAssociationCurrentReaderV1 is the capability-narrowed,
// read-only surface for consumers that must prove an association is current
// without receiving Runtime's Associate mutation authority.
type GenerationBindingAssociationCurrentReaderV1 interface {
	InspectCurrentGenerationBindingAssociationV1(context.Context, string) (GenerationBindingAssociationFactV1, error)
}

type GenerationBindingAssociationGovernancePortV1 interface {
	GenerationBindingAssociationCurrentReaderV1
	AssociateGenerationBindingV1(context.Context, GenerationBindingAssociationCandidateV1) (GenerationBindingAssociationFactV1, error)
}

func GenerationComponentManifestSetDigestV1(components []GenerationComponentManifestRefV1) core.Digest {
	copy := append([]GenerationComponentManifestRefV1{}, components...)
	sort.Slice(copy, func(i, j int) bool { return copy[i].ComponentID < copy[j].ComponentID })
	digest, _ := core.CanonicalJSONDigest("praxis.runtime.generation-binding", GenerationBindingAssociationContractVersionV1, "GenerationComponentManifestSetV1", copy)
	return digest
}

func minimumGenerationBindingExpiryV1(candidate GenerationBindingAssociationCandidateV1) int64 {
	minimum := candidate.RequestedExpiresUnixNano
	for _, value := range []int64{candidate.Generation.ExpiresUnixNano, candidate.Binding.ExpiresUnixNano, candidate.Activation.ExpiresUnixNano} {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func validateGenerationBindingIDV1(value string) error {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "generation binding identity must be bounded and canonical")
	}
	for _, char := range []byte(value) {
		if char < 0x21 || char > 0x7e {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "generation binding identity contains unstable characters")
		}
	}
	return nil
}
