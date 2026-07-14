package fakes_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type bindingRenewalAttestationReaderV2 struct {
	facts map[ports.EvidenceRecordRefV2]control.BindingRenewalAttestationV2
}

func (r bindingRenewalAttestationReaderV2) InspectBindingRenewalAttestationV2(_ context.Context, ref ports.EvidenceRecordRefV2) (control.BindingRenewalAttestationV2, error) {
	fact, ok := r.facts[ref]
	if !ok {
		return control.BindingRenewalAttestationV2{}, core.NewError(core.ErrorNotFound, core.ReasonBindingNotCertified, "renewal attestation does not exist")
	}
	return fact, nil
}

func TestBindingStoreV2AtomicCommitAndLostReplyRecovery(t *testing.T) {
	t.Parallel()
	now := time.Unix(5_000, 0)
	clockNow := now.Add(2 * time.Second)
	store := fakes.NewBindingStoreV2(func() time.Time { return clockNow })
	certified, set := fakeBindingFixtureV2(t, now)
	declared := certified
	declared.State = control.BindingDeclared
	declared.Revision = 1
	declared.Grants = []ports.CapabilityGrantV2{}
	declared.ProbedUnixNano = 0
	declared.CertifiedUnixNano = 0
	declared.ConformanceEvidenceDigest = ""
	declared.ExpiresUnixNano = 0
	if _, err := store.CreateBinding(context.Background(), declared); err != nil {
		t.Fatal(err)
	}
	probed := certified
	probed.State = control.BindingProbed
	probed.Revision = 2
	probed.CertifiedUnixNano = 0
	probed.ConformanceEvidenceDigest = ""
	if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 1, Next: probed}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 2, Next: certified}); err != nil {
		t.Fatal(err)
	}
	store.LoseNextCommitReply()
	request := control.CommitBindingSetRequestV2{Set: set, Expected: []control.ExpectedBindingRevisionV2{{BindingID: certified.ID, ExpectedRevision: certified.Revision}}}
	if _, err := store.CommitBindingSet(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("lost reply injection should hide an already durable commit: %v", err)
	}
	inspected, err := store.InspectBindingSet(context.Background(), set.ID)
	if err != nil || inspected.State != control.BindingSetActive {
		t.Fatalf("restart must inspect the durable binding set instead of re-committing: %v %+v", err, inspected)
	}
	bound, err := store.InspectBinding(context.Background(), certified.ID)
	if err != nil || bound.State != control.BindingBound || bound.BindingSetID != set.ID {
		t.Fatalf("atomic commit must bind every member with the set: %v %+v", err, bound)
	}
}

func TestBindingStoreV2CASLinearizesOnceUnderConcurrency(t *testing.T) {
	t.Parallel()
	now := time.Unix(6_000, 0)
	store := fakes.NewBindingStoreV2(func() time.Time { return now })
	certified, _ := fakeBindingFixtureV2(t, now)
	declared := certified
	declared.State = control.BindingDeclared
	declared.Revision = 1
	declared.Grants = []ports.CapabilityGrantV2{}
	declared.ProbedUnixNano, declared.CertifiedUnixNano, declared.ExpiresUnixNano = 0, 0, 0
	declared.ConformanceEvidenceDigest = ""
	if _, err := store.CreateBinding(context.Background(), declared); err != nil {
		t.Fatal(err)
	}
	probed := certified
	probed.State = control.BindingProbed
	probed.Revision = 2
	probed.CertifiedUnixNano = 0
	probed.ConformanceEvidenceDigest = ""
	var successes int
	var mu sync.Mutex
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 1, Next: probed}); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else if !core.HasReason(err, core.ReasonRevisionConflict) {
				t.Errorf("unexpected concurrent CAS error: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes != 1 {
		t.Fatalf("exactly one CAS must linearize, got %d", successes)
	}
}

func TestBindingStoreV2GovernedBoundAndSetRenewalUsesOneCurrentWatermark(t *testing.T) {
	now := time.Unix(7_000, 0)
	clockNow := now.Add(2 * time.Second)
	store := fakes.NewBindingStoreV2(func() time.Time { return clockNow })
	targetCertified, targetSetRequest := fakeBindingFixtureV2(t, now)
	targetSet := commitBindingFixtureV2(t, store, targetCertified, targetSetRequest)
	targetBound, _ := store.InspectBinding(context.Background(), targetCertified.ID)
	if targetSet.Members[0].BindingRevision != targetBound.Revision {
		t.Fatalf("initial Set member must point at post-commit current Binding revision: set=%d fact=%d", targetSet.Members[0].BindingRevision, targetBound.Revision)
	}
	certifierCertified, certifierSetRequest := fakeCertifierBindingFixtureV2(t, now)
	certifierSet := commitBindingFixtureV2(t, store, certifierCertified, certifierSetRequest)
	certifierMember := certifierSet.Members[0]
	certifier := ports.EvidenceProducerBindingRefV2{BindingSetID: certifierSet.ID, BindingSetRevision: certifierSet.Revision, ComponentID: certifierMember.ComponentID, ManifestDigest: certifierMember.ManifestDigest, ArtifactDigest: certifierMember.ArtifactDigest, Capability: control.BindingRenewalCertifierCapabilityV2}
	reader := bindingRenewalAttestationReaderV2{facts: map[ports.EvidenceRecordRefV2]control.BindingRenewalAttestationV2{}}
	store.SetRenewalAttestations(reader)

	currentFact, currentSet := targetBound, targetSet
	for attempt := uint64(1); attempt <= 2; attempt++ {
		clockNow = clockNow.Add(time.Minute)
		nextFact := currentFact
		nextFact.Revision++
		nextFact.Grants = append([]ports.CapabilityGrantV2{}, currentFact.Grants...)
		nextFact.Grants[0].ObservedUnixNano = clockNow.UnixNano()
		nextFact.Grants[0].ExpiresUnixNano = clockNow.Add(5 * time.Minute).UnixNano()
		nextFact.Grants[0].EvidenceDigest = fakeBindingDigestV2(t, "renewed-grant-"+strconv.FormatUint(attempt, 10))
		nextFact.ExpiresUnixNano = nextFact.Grants[0].ExpiresUnixNano
		ref := ports.EvidenceRecordRefV2{LedgerScopeDigest: fakeBindingDigestV2(t, "renewal-ledger"), Sequence: attempt, RecordDigest: fakeBindingDigestV2(t, attempt)}
		nextFact.RenewalEvidence = append(append([]ports.EvidenceRecordRefV2{}, currentFact.RenewalEvidence...), ref)
		grantDigest, _ := control.BindingGrantSetDigestV2(nextFact.Grants)
		subject, _ := control.BindingRenewalSubjectDigestV2(nextFact.ID, nextFact.ComponentID, nextFact.ManifestDigest, grantDigest)
		reader.facts[ref] = control.BindingRenewalAttestationV2{Evidence: ref, Kind: control.BindingRenewalAttestationKindV2, BindingID: nextFact.ID, ComponentID: nextFact.ComponentID, ManifestDigest: nextFact.ManifestDigest, GrantSetDigest: grantDigest, SubjectDigest: subject, Certifier: certifier, SourceEpoch: 1, SourceSequence: attempt, ObservedUnixNano: clockNow.UnixNano(), ExpiresUnixNano: clockNow.Add(time.Minute).UnixNano()}
		store.SetRenewalAttestations(reader)
		nextSet := currentSet
		nextSet.Revision++
		nextSet.Members = append([]control.BindingMemberV2{}, currentSet.Members...)
		nextSet.Members[0].BindingRevision = nextFact.Revision
		nextSet.Members[0].Grants = append([]ports.CapabilityGrantV2{}, nextFact.Grants...)
		nextSet.ExpiresUnixNano = nextFact.ExpiresUnixNano
		renewed, err := store.RenewBindingSetV2(context.Background(), control.RenewBindingSetRequestV2{ExpectedSetRevision: currentSet.Revision, NextSet: nextSet, NextBindings: []control.BindingFactV2{nextFact}})
		if err != nil {
			t.Fatalf("governed renewal %d failed: %v", attempt, err)
		}
		currentSet = renewed
		currentFact, _ = store.InspectBinding(context.Background(), nextFact.ID)
		if err := control.ValidateBindingSetCurrentV2(currentSet, []control.BindingFactV2{currentFact}, []control.BindingCurrentProbeV2{{ComponentID: currentFact.ComponentID, ManifestDigest: currentFact.ManifestDigest}}, clockNow); err != nil {
			t.Fatalf("renewed Set and member current watermark diverged: %v", err)
		}
	}

	rootOnly := currentSet
	rootOnly.Revision++
	rootOnly.ExpiresUnixNano = rootOnly.ExpiresUnixNano + int64(time.Minute)
	if _, err := store.CompareAndSwapBindingSet(context.Background(), control.BindingSetCASRequestV2{ExpectedRevision: currentSet.Revision, Next: rootOnly}); !core.HasReason(err, core.ReasonInvalidTransition) && !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("raw CAS or root-only TTL extension must fail closed: %v", err)
	}
}

func commitBindingFixtureV2(t *testing.T, store *fakes.BindingStoreV2, certified control.BindingFactV2, set control.BindingSetFactV2) control.BindingSetFactV2 {
	t.Helper()
	declared := certified
	declared.State, declared.Revision = control.BindingDeclared, 1
	declared.Grants = []ports.CapabilityGrantV2{}
	declared.ProbedUnixNano, declared.CertifiedUnixNano, declared.ExpiresUnixNano = 0, 0, 0
	declared.ConformanceEvidenceDigest = ""
	if _, err := store.CreateBinding(context.Background(), declared); err != nil {
		t.Fatal(err)
	}
	probed := certified
	probed.State, probed.Revision = control.BindingProbed, 2
	probed.CertifiedUnixNano, probed.ConformanceEvidenceDigest = 0, ""
	if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 1, Next: probed}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 2, Next: certified}); err != nil {
		t.Fatal(err)
	}
	committed, err := store.CommitBindingSet(context.Background(), control.CommitBindingSetRequestV2{Set: set, Expected: []control.ExpectedBindingRevisionV2{{BindingID: certified.ID, ExpectedRevision: certified.Revision}}})
	if err != nil {
		t.Fatal(err)
	}
	return committed
}

func fakeCertifierBindingFixtureV2(t *testing.T, now time.Time) (control.BindingFactV2, control.BindingSetFactV2) {
	fact, set := fakeBindingFixtureV2(t, now)
	fact.ID = "binding-certifier"
	fact.ComponentID = "runtime/binding-certifier"
	fact.Manifest.ComponentID = fact.ComponentID
	fact.Manifest.Kind = "runtime/binding-certifier"
	fact.Manifest.ArtifactDigest = fakeBindingDigestV2(t, "certifier-artifact")
	fact.Manifest.ProvidedCapabilities = []ports.ProvidedCapabilityV2{{Capability: control.BindingRenewalCertifierCapabilityV2, TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}
	fact.Manifest.Owners = []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: fact.ComponentID}, {Role: ports.OwnerSettlement, OwnerComponentID: fact.ComponentID}, {Role: ports.OwnerCleanup, OwnerComponentID: fact.ComponentID}}
	fact.ManifestDigest, _ = fact.Manifest.BindingDigestV2()
	fact.Grants[0].Capability = control.BindingRenewalCertifierCapabilityV2
	fact.Grants[0].EvidenceDigest = fakeBindingDigestV2(t, "certifier-grant")
	fact.ExpiresUnixNano = fact.Grants[0].ExpiresUnixNano
	set.ID, set.PlanID = "set-certifier", "plan-certifier"
	set.PlanDigest = fakeBindingDigestV2(t, "plan-certifier")
	set.Members[0] = control.BindingMemberV2{BindingID: fact.ID, BindingRevision: fact.Revision, ComponentID: fact.ComponentID, Kind: fact.Manifest.Kind, ManifestDigest: fact.ManifestDigest, ArtifactDigest: fact.Manifest.ArtifactDigest, Contract: fact.Manifest.Contract, Owners: append([]ports.OwnerAssignmentV2{}, fact.Manifest.Owners...), Grants: append([]ports.CapabilityGrantV2{}, fact.Grants...)}
	set.TopologicalOrder = []ports.ComponentIDV2{fact.ComponentID}
	return fact, set
}

func fakeBindingFixtureV2(t *testing.T, now time.Time) (control.BindingFactV2, control.BindingSetFactV2) {
	t.Helper()
	digest := fakeBindingDigestV2(t, "artifact")
	manifest := ports.ComponentManifestV2{ContractVersion: ports.BindingContractVersionV2, ComponentID: "vendor/component", Kind: "vendor/kind", GovernanceCategory: "vendor/execution", SemanticVersion: "1.0.0", ArtifactDigest: digest, Contract: ports.ContractBindingV2{Name: "vendor/contract", Version: "1.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: []ports.SchemaRefV2{}, Locality: ports.LocalityHostControlPlane, Dependencies: []ports.ComponentDependencyV2{}, RequiredCapabilities: []ports.CapabilityRequirementV2{}, ProvidedCapabilities: []ports.ProvidedCapabilityV2{{Capability: "vendor/execute", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}, Conformance: ports.ConformanceFullyControlled, ResidualClass: ports.ResidualInspectable, Owners: []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: "vendor/component"}, {Role: ports.OwnerSettlement, OwnerComponentID: "vendor/component"}, {Role: ports.OwnerCleanup, OwnerComponentID: "vendor/component"}}, Credentials: []ports.CredentialRequirementV2{}, OfflinePolicy: ports.OfflineDenied, Extensions: []ports.GovernanceExtensionV2{}, Annotations: []ports.DisplayAnnotationV2{}}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	governanceDigest := fakeBindingDigestV2(t, "governance")
	expires := now.Add(5 * time.Minute).UnixNano()
	grant := ports.CapabilityGrantV2{Capability: "vendor/execute", EvidenceDigest: fakeBindingDigestV2(t, "grant"), ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	fact := control.BindingFactV2{ID: "binding-component", ComponentID: manifest.ComponentID, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: governanceDigest, State: control.BindingCertified, Revision: 3, Grants: []ports.CapabilityGrantV2{grant}, ProbedUnixNano: now.UnixNano(), CertifiedUnixNano: now.Add(time.Second).UnixNano(), ConformanceEvidenceDigest: fakeBindingDigestV2(t, "conformance"), ExpiresUnixNano: expires}
	set := control.BindingSetFactV2{ID: "set-1", PlanID: "plan-1", PlanDigest: fakeBindingDigestV2(t, "plan"), GovernanceDigest: governanceDigest, State: control.BindingSetActive, Revision: 1, Members: []control.BindingMemberV2{{BindingID: fact.ID, BindingRevision: fact.Revision, ComponentID: fact.ComponentID, Kind: manifest.Kind, ManifestDigest: manifestDigest, ArtifactDigest: digest, Contract: manifest.Contract, Owners: append([]ports.OwnerAssignmentV2{}, manifest.Owners...), Grants: []ports.CapabilityGrantV2{grant}}}, TopologicalOrder: []ports.ComponentIDV2{manifest.ComponentID}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: expires}
	return fact, set
}

func fakeBindingDigestV2(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
