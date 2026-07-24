package packageverify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	intotov1 "github.com/in-toto/attestation/go/v1"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type packageTestClockV1 struct {
	mu  sync.RWMutex
	now time.Time
}

func (c *packageTestClockV1) Now() time.Time    { c.mu.RLock(); defer c.mu.RUnlock(); return c.now }
func (c *packageTestClockV1) Set(now time.Time) { c.mu.Lock(); c.now = now; c.mu.Unlock() }

type exactArtifactReaderV1 struct{ values map[core.Digest][]byte }

func (r *exactArtifactReaderV1) OpenExactSupplyChainArtifactV1(ctx context.Context, ref runtimeports.SupplyChainArtifactContentRefV1) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, ok := r.values[ref.Digest]
	if !ok {
		return nil, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "artifact missing")
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), value...))), nil
}

type exactTrustReaderV1 struct{ values map[core.Digest][]byte }

func (r *exactTrustReaderV1) OpenExactSupplyChainTrustMaterialV1(ctx context.Context, ref runtimeports.SupplyChainTrustMaterialRefV1) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, ok := r.values[ref.Digest]
	if !ok {
		return nil, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "trust missing")
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), value...))), nil
}

type exactPolicyReaderV1 struct{ values map[core.Digest][]byte }

func (r *exactPolicyReaderV1) OpenExactSupplyChainTrustPolicyDocumentV1(ctx context.Context, ref runtimeports.SupplyChainTrustPolicyDocumentRefV1) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, ok := r.values[ref.Digest]
	if !ok {
		return nil, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "policy missing")
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), value...))), nil
}

type trustCurrentReaderV1 struct {
	value runtimeports.SupplyChainTrustPolicyCurrentProjectionV1
}

func (r *trustCurrentReaderV1) InspectCurrentSupplyChainTrustPolicyV1(ctx context.Context, exact runtimeports.SupplyChainTrustPolicyCurrentRefV1) (runtimeports.SupplyChainTrustPolicyCurrentProjectionV1, error) {
	if err := ctx.Err(); err != nil {
		return runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, err
	}
	if exact != r.value.Ref {
		return runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "policy drift")
	}
	return r.value, nil
}

type fakeCryptoVerifierV1 struct{ calls atomic.Int64 }

func (v *fakeCryptoVerifierV1) VerifyV1(ctx context.Context, _ toolcontract.ToolPackageArtifactBindingV1, _, _, _, _, _, _ []byte) (SigstoreVerificationObservationV1, error) {
	v.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return SigstoreVerificationObservationV1{}, err
	}
	return SigstoreVerificationObservationV1{SignerIdentityDigest: testkit.Digest("package-signer"), PredicateType: "https://slsa.dev/provenance/v1"}, nil
}

type lostReplyAdmissionRegistryV1 struct {
	target *registry.Registry
	once   atomic.Bool
}

func (r *lostReplyAdmissionRegistryV1) AdmitVerifiedPackageV1(request toolcontract.ToolPackageVerifiedAdmissionRequestV1, now time.Time) (registry.Record, error) {
	winner, err := r.target.AdmitVerifiedPackageV1(request, now)
	if err == nil && r.once.CompareAndSwap(false, true) {
		return registry.Record{}, errors.New("admission reply lost")
	}
	return winner, err
}

func (r *lostReplyAdmissionRegistryV1) InspectVerifiedPackageAdmissionV1(pkg toolcontract.ObjectRef, verification toolcontract.ToolPackageVerificationCurrentRefV1) (registry.Record, bool) {
	return r.target.InspectVerifiedPackageAdmissionV1(pkg, verification)
}

type packageServiceFixtureV1 struct {
	clock      *packageTestClockV1
	store      *registry.Registry
	repository *RepositoryV1
	service    *ServiceV1
	request    toolcontract.ToolPackageVerifyRequestV1
	crypto     *fakeCryptoVerifierV1
}

func newPackageServiceFixtureV1(t *testing.T) packageServiceFixtureV1 {
	t.Helper()
	now := time.Now().UTC().Add(time.Minute)
	clock := &packageTestClockV1{now: now}
	store := registry.New()
	capability := testkit.Capability()
	tool := testkit.Tool()
	if _, err := store.SubmitCapability(capability, now); err != nil {
		t.Fatal(err)
	}
	_, capRegistry, _ := store.ResolveCapability(string(capability.ID))
	capRegistry, _ = store.Transition("capability", string(capability.ID), capRegistry.RegistryRevision, registry.StateAdmitted, now)
	if _, err := store.Transition("capability", string(capability.ID), capRegistry.RegistryRevision, registry.StateActive, now); err != nil {
		t.Fatal(err)
	}
	toolRecord, err := store.SubmitTool(tool, now)
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err = store.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateAdmitted, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateActive, now); err != nil {
		t.Fatal(err)
	}

	artifact := []byte("package-artifact-v1")
	artifactRef := artifactRefV1("application/octet-stream", artifact)
	pkg := testkit.Package()
	pkg.ArtifactDigest = artifactRef.Digest
	pkg, err = toolcontract.SealPackage(pkg)
	if err != nil {
		t.Fatal(err)
	}
	pkgRecord, err := store.SubmitPackage(pkg, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := toolcontract.SealToolPackageRegistryRecordSourceV1(toolcontract.ToolPackageRegistryRecordSourceV1{
		Kind: pkgRecord.Kind, ID: pkgRecord.ID, ObjectRevision: pkgRecord.ObjectRevision, ObjectDigest: pkgRecord.ObjectDigest,
		State: string(pkgRecord.State), RegistryRevision: pkgRecord.RegistryRevision, UpdatedUnixNano: pkgRecord.UpdatedUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	pkgObject := toolcontract.ObjectRef{ID: string(pkg.ID), Revision: pkg.Revision, Digest: pkg.Digest}
	registryID, _ := toolcontract.DeriveToolPackageRegistryCurrentIDV1(pkgObject)
	pkgCurrentRef := toolcontract.ToolPackageRegistryCurrentRefV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, ID: registryID, Revision: pkgRecord.RegistryRevision, Digest: source.Digest}

	ociBytes := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"application/vnd.praxis.tool-package.v1","config":{"mediaType":"application/vnd.unknown.config.v1+json","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","size":2},"layers":[{"mediaType":"application/octet-stream","digest":"` + string(artifactRef.Digest) + `","size":19}]}`)
	bundleBytes := []byte("fake-bundle")
	statementBytes := []byte("fake-statement")
	rootBytes := []byte("fake-root")
	policy, err := toolcontract.SealToolPackageTrustPolicyDocumentV1(toolcontract.ToolPackageTrustPolicyDocumentV1{
		IdentityMode:           toolcontract.ToolPackageSigstoreCertificateV1,
		CertificateIdentities:  []toolcontract.ToolPackageSigstoreCertificateIdentityV1{{Issuer: "https://issuer.example", SANValue: "builder@example"}},
		RequiredPredicateTypes: []string{"https://slsa.dev/provenance/v1"},
		TimestampMode:          toolcontract.ToolPackageSigstoreObserverTimestampV1, TimestampThreshold: 1,
		TransparencyLogThreshold: 1, SCTThreshold: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	policyBytes, _ := json.Marshal(policy)
	ociRef, bundleRef, statementRef := artifactRefV1(ociv1.MediaTypeImageManifest, ociBytes), artifactRefV1("application/vnd.dev.sigstore.bundle.v0.3+json", bundleBytes), artifactRefV1("application/vnd.in-toto+json", statementBytes)
	rootRef := runtimeports.SupplyChainTrustMaterialRefV1{ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1, ID: "root-test", Revision: 1, Digest: digestBytesV1(rootBytes)}
	policyRef := runtimeports.SupplyChainTrustPolicyRefV1{ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1, ID: "policy-test", Revision: 1, Digest: testkit.Digest("policy-historical")}
	policyDocRef := runtimeports.SupplyChainTrustPolicyDocumentRefV1{ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1, ID: "policy-doc-test", Revision: 1, MediaType: toolcontract.PackageTrustPolicyMediaTypeV1, Digest: digestBytesV1(policyBytes), Size: uint64(len(policyBytes))}
	trustProjection, err := runtimeports.SealSupplyChainTrustPolicyCurrentProjectionV1(runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{
		Ref:    runtimeports.SupplyChainTrustPolicyCurrentRefV1{ID: "policy-current-test", Revision: 1},
		Policy: policyRef, PolicyDocument: policyDocRef, TrustedRoot: rootRef,
		IdentityPolicyDigest: policy.IdentityPolicyDigest, PredicatePolicyDigest: policy.PredicatePolicyDigest,
		TransparencyPolicyDigest: policy.TransparencyPolicyDigest, TimestampPolicyDigest: policy.TimestampPolicyDigest,
		MaxPackageArtifactBytes: 1 << 20, MaxSigstoreBundleBytes: 1 << 20, MaxInTotoStatementBytes: 1 << 20, MaxTrustMaterialBytes: 1 << 20,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := toolcontract.SealToolPackageArtifactBindingV1(toolcontract.ToolPackageArtifactBindingV1{
		Package: pkgObject, OCIManifest: ociRef, PackageArtifact: artifactRef,
		SigstoreBundle: bundleRef, InTotoStatement: statementRef, ArtifactType: toolcontract.PackageArtifactTypeV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository, _ := NewRepositoryV1(clock)
	packageReader, _ := NewPackageRegistryCurrentReaderV1(store, clock)
	crypto := &fakeCryptoVerifierV1{}
	service, err := NewServiceV1(ServiceConfigV1{
		Artifacts:    &exactArtifactReaderV1{values: map[core.Digest][]byte{ociRef.Digest: ociBytes, artifactRef.Digest: artifact, bundleRef.Digest: bundleBytes, statementRef.Digest: statementBytes}},
		Trust:        &exactTrustReaderV1{values: map[core.Digest][]byte{rootRef.Digest: rootBytes}},
		Policies:     &exactPolicyReaderV1{values: map[core.Digest][]byte{policyDocRef.Digest: policyBytes}},
		TrustCurrent: &trustCurrentReaderV1{value: trustProjection}, Packages: packageReader,
		Repository: repository, Verifier: crypto, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := toolcontract.ToolPackageVerifyRequestV1{
		ContractVersion:    toolcontract.PackageVerificationContractVersionV1,
		Subject:            toolcontract.ToolPackageVerificationSubjectV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, PackageRegistry: pkgCurrentRef, ArtifactBinding: binding, TrustPolicy: policyRef, VerifierProfile: toolcontract.PackageVerifierConformanceV1},
		TrustPolicyCurrent: trustProjection.Ref, RequestedExpiresUnixNano: now.Add(30 * time.Minute).UnixNano(),
	}
	return packageServiceFixtureV1{clock: clock, store: store, repository: repository, service: service, request: request, crypto: crypto}
}

func resolvePackageVerificationCurrentFixtureV1(t *testing.T, f packageServiceFixtureV1) toolcontract.ToolPackageVerificationCurrentProjectionV1 {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, f.request.RequestedExpiresUnixNano))
	defer cancel()
	fact, err := f.service.VerifyV1(ctx, f.request)
	if err != nil {
		t.Fatalf("Verify fixture: %v", err)
	}
	issuance := toolcontract.ToolPackageVerificationCurrentIssuanceV1{
		ContractVersion:          toolcontract.PackageVerificationContractVersionV1,
		Fact:                     fact.Ref,
		PackageRegistry:          f.request.Subject.PackageRegistry,
		TrustPolicyCurrent:       f.request.TrustPolicyCurrent,
		RequestedExpiresUnixNano: f.request.RequestedExpiresUnixNano,
	}
	current, err := f.service.ResolveCurrentToolPackageVerificationV1(ctx, issuance)
	if err != nil {
		t.Fatalf("Resolve current fixture: %v", err)
	}
	return current
}

func TestPackageVerificationServiceFactCurrentAdmissionAndRecoveryV1(t *testing.T) {
	f := newPackageServiceFixtureV1(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, f.request.RequestedExpiresUnixNano))
	defer cancel()
	fact, err := f.service.VerifyV1(ctx, f.request)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	issuance := toolcontract.ToolPackageVerificationCurrentIssuanceV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, Fact: fact.Ref, PackageRegistry: f.request.Subject.PackageRegistry, TrustPolicyCurrent: f.request.TrustPolicyCurrent, RequestedExpiresUnixNano: f.request.RequestedExpiresUnixNano}
	current, err := f.service.ResolveCurrentToolPackageVerificationV1(ctx, issuance)
	if err != nil {
		t.Fatalf("Resolve current: %v", err)
	}
	command := toolcontract.ToolPackageAdmissionCommandV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, VerificationCurrent: current.Ref, ExpectedRegistryRevision: f.request.Subject.PackageRegistry.Revision}
	record, err := f.service.AdmitPackageV1(ctx, f.store, command)
	if err != nil {
		t.Fatalf("Admission: %v", err)
	}
	if record.State != registry.StateAdmitted {
		t.Fatalf("state=%s", record.State)
	}
	recovered, err := f.service.AdmitPackageV1(ctx, f.store, command)
	if err != nil || recovered != record {
		t.Fatalf("lost-reply recovery=%+v err=%v", recovered, err)
	}
	if _, err := f.store.Transition("package", string(current.Fact.Package.ID), record.RegistryRevision, registry.StateActive, f.clock.Now()); !core.HasReason(err, core.ReasonInvalidTransition) {
		t.Fatalf("generic enable error=%v", err)
	}
}

func TestPackageVerificationConcurrentSingleFactAndAdmissionV1(t *testing.T) {
	f := newPackageServiceFixtureV1(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, f.request.RequestedExpiresUnixNano))
	defer cancel()
	const workers = 64
	facts := make(chan toolcontract.ToolPackageVerificationFactV1, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() { defer wg.Done(); fact, err := f.service.VerifyV1(ctx, f.request); facts <- fact; errs <- err }()
	}
	wg.Wait()
	close(facts)
	close(errs)
	var exact toolcontract.ToolPackageVerificationFactRefV1
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for fact := range facts {
		if exact == (toolcontract.ToolPackageVerificationFactRefV1{}) {
			exact = fact.Ref
		} else if fact.Ref != exact {
			t.Fatal("multiple Fact winners")
		}
	}
}

func TestPackageVerificationConcurrentAdmissionHasOneExactWinnerV1(t *testing.T) {
	f := newPackageServiceFixtureV1(t)
	current := resolvePackageVerificationCurrentFixtureV1(t, f)
	command := toolcontract.ToolPackageAdmissionCommandV1{
		ContractVersion:          toolcontract.PackageVerificationContractVersionV1,
		VerificationCurrent:      current.Ref,
		ExpectedRegistryRevision: f.request.Subject.PackageRegistry.Revision,
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, f.request.RequestedExpiresUnixNano))
	defer cancel()
	const workers = 64
	records := make(chan registry.Record, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, err := f.service.AdmitPackageV1(ctx, f.store, command)
			records <- record
			errs <- err
		}()
	}
	wg.Wait()
	close(records)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var winner registry.Record
	for record := range records {
		if winner == (registry.Record{}) {
			winner = record
		} else if record != winner {
			t.Fatalf("concurrent Admission returned different winners: %+v %+v", winner, record)
		}
	}
	if winner.State != registry.StateAdmitted {
		t.Fatalf("Admission winner=%+v", winner)
	}
}

func TestPackageVerificationAdmissionFailsClosedOnExpiredClockAndMaterialDriftV1(t *testing.T) {
	t.Run("expired current", func(t *testing.T) {
		f := newPackageServiceFixtureV1(t)
		current := resolvePackageVerificationCurrentFixtureV1(t, f)
		f.clock.Set(time.Unix(0, current.ExpiresUnixNano))
		command := toolcontract.ToolPackageAdmissionCommandV1{
			ContractVersion: toolcontract.PackageVerificationContractVersionV1, VerificationCurrent: current.Ref,
			ExpectedRegistryRevision: f.request.Subject.PackageRegistry.Revision,
		}
		if _, err := f.service.AdmitPackageV1(context.Background(), f.store, command); err == nil {
			t.Fatal("expired Verification current admitted Package")
		}
		_, record, _ := f.store.ResolvePackage(current.Fact.Package.ID)
		if record.State != registry.StateSubmitted {
			t.Fatalf("expired Admission changed Registry: %+v", record)
		}
	})
	t.Run("material drift", func(t *testing.T) {
		f := newPackageServiceFixtureV1(t)
		current := resolvePackageVerificationCurrentFixtureV1(t, f)
		reader := f.service.config.Artifacts.(*exactArtifactReaderV1)
		ref := f.request.Subject.ArtifactBinding.PackageArtifact
		reader.values[ref.Digest] = []byte("drifted-package-artifact")
		command := toolcontract.ToolPackageAdmissionCommandV1{
			ContractVersion: toolcontract.PackageVerificationContractVersionV1, VerificationCurrent: current.Ref,
			ExpectedRegistryRevision: f.request.Subject.PackageRegistry.Revision,
		}
		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, f.request.RequestedExpiresUnixNano))
		defer cancel()
		if _, err := f.service.AdmitPackageV1(ctx, f.store, command); err == nil {
			t.Fatal("drifting exact Artifact admitted Package")
		}
		_, record, _ := f.store.ResolvePackage(current.Fact.Package.ID)
		if record.State != registry.StateSubmitted {
			t.Fatalf("drifting Admission changed Registry: %+v", record)
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		f := newPackageServiceFixtureV1(t)
		current := resolvePackageVerificationCurrentFixtureV1(t, f)
		f.clock.Set(time.Unix(0, current.CheckedUnixNano-1))
		command := toolcontract.ToolPackageAdmissionCommandV1{
			ContractVersion: toolcontract.PackageVerificationContractVersionV1, VerificationCurrent: current.Ref,
			ExpectedRegistryRevision: f.request.Subject.PackageRegistry.Revision,
		}
		if _, err := f.service.AdmitPackageV1(context.Background(), f.store, command); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback error=%v", err)
		}
		_, record, _ := f.store.ResolvePackage(current.Fact.Package.ID)
		if record.State != registry.StateSubmitted {
			t.Fatalf("clock rollback changed Registry: %+v", record)
		}
	})
}

func TestPackageVerificationAdmissionLostReplyAndChangedCASSourceV1(t *testing.T) {
	f := newPackageServiceFixtureV1(t)
	current := resolvePackageVerificationCurrentFixtureV1(t, f)
	command := toolcontract.ToolPackageAdmissionCommandV1{
		ContractVersion:          toolcontract.PackageVerificationContractVersionV1,
		VerificationCurrent:      current.Ref,
		ExpectedRegistryRevision: f.request.Subject.PackageRegistry.Revision,
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, f.request.RequestedExpiresUnixNano))
	defer cancel()

	target := &lostReplyAdmissionRegistryV1{target: f.store}
	winner, err := f.service.AdmitPackageV1(ctx, target, command)
	if err != nil || winner.State != registry.StateAdmitted || !target.once.Load() {
		t.Fatalf("lost Admission reply was not recovered: winner=%+v err=%v", winner, err)
	}
	changed := command
	changed.ExpectedRegistryRevision++
	if _, err = f.service.AdmitPackageV1(ctx, target, changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("changed CAS source recovered another command: %v", err)
	}
	recovered, ok := f.store.InspectVerifiedPackageAdmissionV1(current.Fact.Package, current.Ref)
	if !ok || recovered != winner {
		t.Fatalf("changed command altered admitted winner: recovered=%+v ok=%v", recovered, ok)
	}
}

func TestPackageVerificationFailClosedV1(t *testing.T) {
	f := newPackageServiceFixtureV1(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, f.request.RequestedExpiresUnixNano))
	defer cancel()
	t.Run("digest drift", func(t *testing.T) {
		reader := f.service.config.Artifacts.(*exactArtifactReaderV1)
		ref := f.request.Subject.ArtifactBinding.PackageArtifact
		original := reader.values[ref.Digest]
		reader.values[ref.Digest] = []byte("changed")
		defer func() { reader.values[ref.Digest] = original }()
		if _, err := f.service.VerifyV1(ctx, f.request); !core.HasReason(err, core.ReasonCanonicalLimitExceeded) && !core.HasReason(err, core.ReasonInvalidDigest) {
			t.Fatalf("drift error=%v", err)
		}
	})
	t.Run("canceled", func(t *testing.T) {
		canceled, stop := context.WithCancel(ctx)
		stop()
		if _, err := f.service.VerifyV1(canceled, f.request); err != context.Canceled {
			t.Fatalf("canceled error=%v", err)
		}
	})
	t.Run("typed nil", func(t *testing.T) {
		var nilArtifacts *exactArtifactReaderV1
		config := f.service.config
		config.Artifacts = nilArtifacts
		if _, err := NewServiceV1(config); err == nil {
			t.Fatal("typed nil accepted")
		}
	})
}

func TestOfficialSigstoreAdapterPolicyOCIAndStatementV1(t *testing.T) {
	f := newPackageServiceFixtureV1(t)
	policyBytes := f.service.config.Policies.(*exactPolicyReaderV1).values[f.service.config.TrustCurrent.(*trustCurrentReaderV1).value.PolicyDocument.Digest]
	if _, err := parseTrustPolicyV1(policyBytes); err != nil {
		t.Fatalf("policy parse: %v", err)
	}
	binding := f.request.Subject.ArtifactBinding
	ociBytes := f.service.config.Artifacts.(*exactArtifactReaderV1).values[binding.OCIManifest.Digest]
	if err := validateOCIManifestV1(ociBytes, binding); err != nil {
		t.Fatalf("OCI: %v", err)
	}
	predicate, _ := structpb.NewStruct(map[string]any{"builder": "test"})
	statement := &intotov1.Statement{Type: intotov1.StatementTypeUri, Subject: []*intotov1.ResourceDescriptor{{Name: "package", Digest: map[string]string{"sha256": strings.TrimPrefix(string(binding.PackageArtifact.Digest), "sha256:")}}}, PredicateType: "https://slsa.dev/provenance/v1", Predicate: predicate}
	raw, _ := protojson.Marshal(statement)
	if _, err := parseStatementV1(raw, binding.PackageArtifact.Digest); err != nil {
		t.Fatalf("statement: %v", err)
	}
	if _, err := (SigstoreBundleVerifierV1{}).VerifyV1(context.Background(), binding, ociBytes, []byte("artifact"), []byte("bad bundle"), raw, []byte("bad root"), policyBytes); err == nil {
		t.Fatal("invalid official Sigstore inputs passed")
	}
}

func TestOfficialSigstoreAdapterVerifiesOfflineKeyBundleV1(t *testing.T) {
	f := newPackageServiceFixtureV1(t)
	binding := f.request.Subject.ArtifactBinding
	artifact := f.service.config.Artifacts.(*exactArtifactReaderV1).values[binding.PackageArtifact.Digest]
	ociBytes := f.service.config.Artifacts.(*exactArtifactReaderV1).values[binding.OCIManifest.Digest]

	predicate, err := structpb.NewStruct(map[string]any{"builder": "offline-conformance"})
	if err != nil {
		t.Fatal(err)
	}
	statement := &intotov1.Statement{
		Type: intotov1.StatementTypeUri,
		Subject: []*intotov1.ResourceDescriptor{{
			Name: "package", Digest: map[string]string{"sha256": strings.TrimPrefix(string(binding.PackageArtifact.Digest), "sha256:")},
		}},
		PredicateType: "https://slsa.dev/provenance/v1", Predicate: predicate,
	}
	statementBytes, err := protojson.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	keypair, err := sign.NewEphemeralKeypair(nil)
	if err != nil {
		t.Fatal(err)
	}
	protoBundle, err := sign.Bundle(&sign.DSSEData{Data: statementBytes, PayloadType: "application/vnd.in-toto+json"}, keypair, sign.BundleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	signedBundle, err := bundle.NewBundle(protoBundle)
	if err != nil {
		t.Fatal(err)
	}
	bundleBytes, err := signedBundle.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	publicKeyPEM, err := keypair.GetPublicKeyPem()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := toolcontract.SealToolPackageTrustPolicyDocumentV1(toolcontract.ToolPackageTrustPolicyDocumentV1{
		IdentityMode:           toolcontract.ToolPackageSigstoreKeyV1,
		RequiredPredicateTypes: []string{"https://slsa.dev/provenance/v1"},
		TimestampMode:          toolcontract.ToolPackageSigstoreNoTimestampForKeyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	policyBytes, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}

	observation, err := (SigstoreBundleVerifierV1{}).VerifyV1(
		context.Background(), binding, ociBytes, artifact, bundleBytes,
		statementBytes, []byte(publicKeyPEM), policyBytes,
	)
	if err != nil {
		t.Fatalf("official offline key bundle verification failed: %v", err)
	}
	if observation.SignerIdentityDigest.Validate() != nil || observation.PredicateType != statement.PredicateType {
		t.Fatalf("official verification observation drifted: %+v", observation)
	}

	tampered := append([]byte(nil), artifact...)
	tampered[0] ^= 0xff
	if _, err = (SigstoreBundleVerifierV1{}).VerifyV1(context.Background(), binding, ociBytes, tampered, bundleBytes, statementBytes, []byte(publicKeyPEM), policyBytes); err == nil {
		t.Fatal("official verifier accepted a tampered Package Artifact")
	}
}

func artifactRefV1(mediaType string, value []byte) runtimeports.SupplyChainArtifactContentRefV1 {
	return runtimeports.SupplyChainArtifactContentRefV1{ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1, MediaType: mediaType, Digest: digestBytesV1(value), Size: uint64(len(value))}
}

func digestBytesV1(value []byte) core.Digest {
	sum := sha256.Sum256(value)
	return core.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
