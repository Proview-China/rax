package control

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// NewEvidenceSourceRegistrationRefV1 derives, rather than accepts, the exact
// immutable/current coordinates exported by the Evidence Source Owner.
func NewEvidenceSourceRegistrationRefV1(fact ports.EvidenceSourceRegistrationFactV2) (ports.EvidenceSourceRegistrationRefV1, error) {
	if err := fact.Validate(); err != nil {
		return ports.EvidenceSourceRegistrationRefV1{}, err
	}
	digest, err := fact.DigestV2()
	if err != nil {
		return ports.EvidenceSourceRegistrationRefV1{}, err
	}
	configuration, err := fact.ConfigurationDigestV2()
	if err != nil {
		return ports.EvidenceSourceRegistrationRefV1{}, err
	}
	ref := ports.EvidenceSourceRegistrationRefV1{RegistrationID: fact.ID, Revision: fact.Revision, FactDigest: digest, ConfigurationDigest: configuration, SourceID: fact.SourceID, SourceEpoch: fact.SourceEpoch}
	return ref, ref.Validate()
}

func NewEvidenceTombstoneRefV1(fact ports.EvidenceTombstoneFactV2) (ports.EvidenceTombstoneRefV1, error) {
	if err := fact.Validate(); err != nil {
		return ports.EvidenceTombstoneRefV1{}, err
	}
	digest, err := fact.DigestV2()
	if err != nil {
		return ports.EvidenceTombstoneRefV1{}, err
	}
	ref := ports.EvidenceTombstoneRefV1{Record: fact.Record, Source: fact.Source, Revision: fact.Revision, Digest: digest}
	return ref, ref.Validate()
}

// NewEvidenceSubjectMutationBundleV1 constructs the one-way canonical publish
// bundle. It does not write. The Evidence Owner store is the only linearization
// point for Projection history, Current Index and immutable Commit.
func NewEvidenceSubjectMutationBundleV1(request ports.EvidenceSubjectMutationRequestV1, projection ports.EvidenceSubjectCurrentProjectionV1, now time.Time) (ports.EvidenceSubjectMutationCommitV1, ports.EvidenceSubjectCurrentProjectionV1, ports.EvidenceSubjectCurrentIndexRefV1, error) {
	if now.IsZero() {
		return ports.EvidenceSubjectMutationCommitV1{}, ports.EvidenceSubjectCurrentProjectionV1{}, ports.EvidenceSubjectCurrentIndexRefV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Evidence subject Mutation bundle requires injected time")
	}
	if err := request.Validate(); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, ports.EvidenceSubjectCurrentProjectionV1{}, ports.EvidenceSubjectCurrentIndexRefV1{}, err
	}
	if projection.Subject != request.Subject {
		return ports.EvidenceSubjectMutationCommitV1{}, ports.EvidenceSubjectCurrentProjectionV1{}, ports.EvidenceSubjectCurrentIndexRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject Mutation Projection belongs to another subject")
	}
	nextRevision := core.Revision(1)
	var previousProjection *ports.EvidenceSubjectProjectionRefV1
	if request.ExpectedCurrentIndex != nil {
		nextRevision = request.ExpectedCurrentIndex.Revision + 1
		copy := *request.ExpectedCurrentProjection
		previousProjection = &copy
	}
	projection.ContractVersion = ports.EvidenceSubjectCurrentContractVersionV1
	projection.Ref.Revision = nextRevision
	projection.PreviousProjection = previousProjection
	if projection.Ref.OwnerWatermark == 0 {
		return ports.EvidenceSubjectMutationCommitV1{}, ports.EvidenceSubjectCurrentProjectionV1{}, ports.EvidenceSubjectCurrentIndexRefV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Evidence subject Mutation requires Owner watermark")
	}
	sealedProjection, err := ports.SealEvidenceSubjectCurrentProjectionV1(projection)
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, ports.EvidenceSubjectCurrentProjectionV1{}, ports.EvidenceSubjectCurrentIndexRefV1{}, err
	}
	index, err := ports.SealEvidenceSubjectCurrentIndexRefV1(ports.EvidenceSubjectCurrentIndexRefV1{Revision: nextRevision, SubjectKeyDigest: sealedProjection.SubjectKeyDigest, PreviousProjection: previousProjection, CurrentProjection: sealedProjection.Ref, OwnerWatermark: sealedProjection.Ref.OwnerWatermark})
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, ports.EvidenceSubjectCurrentProjectionV1{}, ports.EvidenceSubjectCurrentIndexRefV1{}, err
	}
	key, err := ports.DeriveEvidenceSubjectMutationKeyV1(request)
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, ports.EvidenceSubjectCurrentProjectionV1{}, ports.EvidenceSubjectCurrentIndexRefV1{}, err
	}
	commit, err := ports.SealEvidenceSubjectMutationCommitV1(ports.EvidenceSubjectMutationCommitV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Key: key, Request: request, Subject: request.Subject, RequestDigest: request.RequestDigest, ExpectedPreviousIndex: request.ExpectedCurrentIndex, ExpectedPreviousProjection: request.ExpectedCurrentProjection, NewProjection: sealedProjection.Ref, NewIndex: index, CommittedUnixNano: now.UnixNano()})
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, ports.EvidenceSubjectCurrentProjectionV1{}, ports.EvidenceSubjectCurrentIndexRefV1{}, err
	}
	return commit, sealedProjection, index, nil
}

func ValidateEvidenceSubjectProgressionV1(previous ports.EvidenceSubjectCurrentIndexRefV1, next ports.EvidenceSubjectCurrentIndexRefV1) error {
	if err := previous.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if previous.IndexID != next.IndexID || previous.SubjectKeyDigest != next.SubjectKeyDigest || next.Revision != previous.Revision+1 || next.PreviousProjection == nil || *next.PreviousProjection != previous.CurrentProjection || next.OwnerWatermark <= previous.OwnerWatermark {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Evidence subject Current Index is not a legal monotonic successor")
	}
	return nil
}
