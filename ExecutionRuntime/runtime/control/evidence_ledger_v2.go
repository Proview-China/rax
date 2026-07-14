package control

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func ValidateNewEvidenceSourceV2(fact ports.EvidenceSourceRegistrationFactV2, now time.Time) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	if now.IsZero() || fact.Revision != 1 || fact.State != ports.EvidenceSourceActive || fact.NextSourceSequence != 1 || fact.CreatedUnixNano != fact.UpdatedUnixNano || fact.CreatedUnixNano > now.UnixNano() || fact.ExpiresUnixNano <= fact.CreatedUnixNano || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceSourceMissing, "new source requires revision one, cursor one, non-future creation time and future expiry")
	}
	return nil
}

// ValidateEvidenceSourceTransitionV2 validates the raw Source Fact lifecycle.
// Advancing NextSourceSequence is reserved for EvidenceLedgerFactPortV2.Append
// so cursor and record can be committed atomically by the sole Ledger Owner.
func ValidateEvidenceSourceTransitionV2(current, next ports.EvidenceSourceRegistrationFactV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "source transition clock regressed")
	}
	if current.ID != next.ID || current.SourceID != next.SourceID || current.SourceEpoch != next.SourceEpoch || current.LedgerScope != next.LedgerScope ||
		!ports.SameExecutionScopeV2(current.ExecutionScope, next.ExecutionScope) || current.ActionScopeDigest != next.ActionScopeDigest ||
		!sameEvidenceMappingsV2(current.ClassMappings, next.ClassMappings) || !sameEvidenceKindsV2(current.AllowedKinds, next.AllowedKinds) || current.GapPolicy != next.GapPolicy {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceSourceStale, "source registration identity and authority bindings are immutable")
	}
	if !validEvidenceScopeWatermarkAdvanceV2(current, next) || !validEvidenceProducerWatermarkAdvanceV2(current.Producer, next.Producer) || !validEvidenceAuthorityWatermarkAdvanceV2(current.Authority, next.Authority) || !validEvidencePolicyWatermarkAdvanceV2(current.Policy, next.Policy) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceSourceStale, "source governance watermarks must remain exact or advance one revision")
	}
	if next.Revision != current.Revision+1 || next.NextSourceSequence != current.NextSourceSequence || next.CreatedUnixNano != current.CreatedUnixNano || next.UpdatedUnixNano != now.UnixNano() {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "source CAS must advance one revision without moving its append cursor")
	}
	if current.State != ports.EvidenceSourceActive {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "terminal source registration is immutable")
	}
	switch next.State {
	case ports.EvidenceSourceActive:
		if next.ExpiresUnixNano <= current.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "source renewal must extend expiry")
		}
	case ports.EvidenceSourceRevoked:
		if next.ExpiresUnixNano != current.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "revocation cannot rewrite source expiry")
		}
	case ports.EvidenceSourceExpired:
		if next.ExpiresUnixNano != current.ExpiresUnixNano || now.Before(time.Unix(0, current.ExpiresUnixNano)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "source cannot expire before its exact TTL or rewrite expiry")
		}
	default:
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "active source may only renew, revoke or expire through CAS")
	}
	return nil
}

func ValidateEvidenceAppendV2(source ports.EvidenceSourceRegistrationFactV2, request ports.EvidenceAppendRequestV2, now time.Time) error {
	if err := source.Validate(); err != nil {
		return err
	}
	if err := request.Candidate.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < source.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "evidence append clock regressed")
	}
	if source.State != ports.EvidenceSourceActive || !ports.EvidenceTimeCurrentV2(source.ExpiresUnixNano, now) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "evidence source is not current")
	}
	event := request.Candidate
	configurationDigest, err := source.ConfigurationDigestV2()
	if err != nil {
		return err
	}
	if request.ExpectedSourceRevision != source.Revision || event.RegistrationID != source.ID || event.RegistrationRevision != source.Revision || event.SourceConfigurationDigest != configurationDigest || event.SourcePolicy != source.Policy || event.SourceID != source.SourceID || event.SourceEpoch != source.SourceEpoch || event.LedgerScope != source.LedgerScope || !ports.SameExecutionScopeV2(event.ExecutionScope, source.ExecutionScope) || event.Producer != source.Producer || event.Authority != source.Authority {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "event drifted from its exact source registration")
	}
	if event.SourceSequence != source.NextSourceSequence {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceSequenceGap, "source sequence is stale or contains a gap")
	}
	if !evidenceKindAllowedV2(source.AllowedKinds, event.EventKind) {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "event kind is not registered")
	}
	trust, ok := evidenceMappedTrustV2(source.ClassMappings, event.CustomClass)
	if !ok || trust != event.TrustClass || event.TrustClass == ports.EvidenceTrustLateObservation {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "custom class does not map to the declared Runtime trust class")
	}
	if event.ObservedUnixNano > now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "evidence cannot be observed in the future")
	}
	return nil
}

func ValidateEvidenceLateAppendV2(source ports.EvidenceSourceRegistrationFactV2, request ports.EvidenceAppendLateRequestV2, now time.Time) error {
	event := request.Candidate
	if event.TrustClass != ports.EvidenceTrustLateObservation || event.HistoricalSource == nil || event.OwnerFact != nil {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "late append only accepts downgraded historical observation")
	}
	ordinary := event
	ordinary.TrustClass = ports.EvidenceTrustObservation
	ordinary.HistoricalSource = nil
	if err := ValidateEvidenceAppendV2(source, ports.EvidenceAppendRequestV2{Candidate: ordinary, ExpectedSourceRevision: request.ExpectedSourceRevision}, now); err != nil {
		return err
	}
	return nil
}

func NewEvidenceLedgerRecordV2(candidate ports.EvidenceEventCandidateV2, ledgerSequence uint64, previous core.Digest, ingested time.Time) (ports.EvidenceLedgerRecordV2, error) {
	if err := candidate.Validate(); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	if ledgerSequence == 0 || ingested.IsZero() || ingested.UnixNano() < candidate.ObservedUnixNano {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceChainConflict, "ledger sequence and monotonic ingest time are required")
	}
	scopeDigest, err := candidate.LedgerScope.DigestV2()
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	candidateDigest, err := candidate.DigestV2()
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	if ledgerSequence == 1 {
		if previous != ports.EvidenceGenesisDigestV2 {
			return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceChainConflict, "first ledger record requires the fixed genesis digest")
		}
	} else if err := previous.Validate(); err != nil {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceChainConflict, "non-first ledger record requires a predecessor digest")
	}
	record := ports.EvidenceLedgerRecordV2{Ref: ports.EvidenceRecordRefV2{LedgerScopeDigest: scopeDigest, Sequence: ledgerSequence}, Candidate: candidate, CandidateDigest: candidateDigest, PreviousRecordDigest: previous, IngestedUnixNano: ingested.UnixNano()}
	digest, err := EvidenceLedgerRecordDigestV2(record)
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	record.Ref.RecordDigest = digest
	return record, nil
}

func EvidenceLedgerRecordDigestV2(record ports.EvidenceLedgerRecordV2) (core.Digest, error) {
	if record.Ref.Sequence == 0 || record.IngestedUnixNano <= 0 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceChainConflict, "record owner fields are incomplete")
	}
	if err := record.Ref.LedgerScopeDigest.Validate(); err != nil {
		return "", err
	}
	digest, err := record.Candidate.DigestV2()
	if err != nil {
		return "", err
	}
	if digest != record.CandidateDigest {
		return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "record candidate digest drifted")
	}
	copy := record
	copy.Ref.RecordDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.evidence", ports.EvidenceContractVersionV2, "EvidenceLedgerRecordV2", copy)
}

func ValidateEvidenceLedgerRecordV2(record ports.EvidenceLedgerRecordV2) error {
	if err := record.Validate(); err != nil {
		return err
	}
	scopeDigest, err := record.Candidate.LedgerScope.DigestV2()
	if err != nil {
		return err
	}
	if record.Ref.LedgerScopeDigest != scopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceScopeConflict, "record ref does not match candidate ledger scope")
	}
	if record.Ref.Sequence == 1 && record.PreviousRecordDigest != ports.EvidenceGenesisDigestV2 {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceChainConflict, "genesis predecessor digest drifted")
	}
	digest, err := EvidenceLedgerRecordDigestV2(record)
	if err != nil {
		return err
	}
	if record.Ref.RecordDigest != digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceChainConflict, "ledger record digest drifted")
	}
	return nil
}

func evidenceMappedTrustV2(mappings []ports.EvidenceClassMappingV2, class ports.NamespacedNameV2) (ports.EvidenceTrustClassV2, bool) {
	for _, mapping := range mappings {
		if mapping.Class == class {
			return mapping.Trust, true
		}
	}
	return "", false
}
func evidenceKindAllowedV2(kinds []ports.NamespacedNameV2, kind ports.NamespacedNameV2) bool {
	for _, current := range kinds {
		if current == kind {
			return true
		}
	}
	return false
}
func sameEvidenceMappingsV2(left, right []ports.EvidenceClassMappingV2) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
func sameEvidenceKindsV2(left, right []ports.NamespacedNameV2) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validEvidenceScopeWatermarkAdvanceV2(current, next ports.EvidenceSourceRegistrationFactV2) bool {
	if current.CurrentScope == next.CurrentScope {
		return current.CurrentScopeWatermark == next.CurrentScopeWatermark
	}
	return current.CurrentScope.Ref == next.CurrentScope.Ref && next.CurrentScope.Revision == current.CurrentScope.Revision+1 && next.CurrentScope.Digest != current.CurrentScope.Digest && next.CurrentScopeWatermark > current.CurrentScopeWatermark
}
func validEvidenceProducerWatermarkAdvanceV2(current, next ports.EvidenceProducerBindingRefV2) bool {
	if current == next {
		return true
	}
	return current.BindingSetID == next.BindingSetID && current.ComponentID == next.ComponentID && current.ManifestDigest == next.ManifestDigest && current.ArtifactDigest == next.ArtifactDigest && current.Capability == next.Capability && next.BindingSetRevision == current.BindingSetRevision+1
}
func validEvidenceAuthorityWatermarkAdvanceV2(current, next ports.AuthorityBindingRefV2) bool {
	if current == next {
		return true
	}
	return current.Ref == next.Ref && current.Epoch == next.Epoch && next.Revision == current.Revision+1 && next.Digest != current.Digest
}
func validEvidencePolicyWatermarkAdvanceV2(current, next ports.EvidenceSourcePolicyBindingRefV2) bool {
	if current == next {
		return true
	}
	return current.Ref == next.Ref && next.Revision == current.Revision+1 && next.Digest != current.Digest
}

func ValidateEvidenceSourceKeyV2(key ports.EvidenceSourceKeyV2) error {
	if strings.TrimSpace(key.RegistrationID) == "" || key.SourceEpoch == 0 || key.SourceSequence == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceSourceMissing, "complete source key is required")
	}
	return nil
}
