package corepack

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

type admittedVerificationPortV1 struct {
	target  *registry.Registry
	fixture verificationFixtureForCorePackV1
	unknown bool
	calls   atomic.Int64
}

func (p *admittedVerificationPortV1) VerifyV1(ctx context.Context, _ toolcontract.ToolPackageVerifyRequestV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	p.calls.Add(1)
	return p.fixture.fact, ctx.Err()
}
func (p *admittedVerificationPortV1) ResolveCurrentToolPackageVerificationV1(ctx context.Context, _ toolcontract.ToolPackageVerificationCurrentIssuanceV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	p.calls.Add(1)
	return p.fixture.current, ctx.Err()
}
func (p *admittedVerificationPortV1) InspectCurrentToolPackageVerificationV1(ctx context.Context, _ toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	p.calls.Add(1)
	return p.fixture.current, ctx.Err()
}
func (p *admittedVerificationPortV1) InspectExactToolPackageVerificationObservationV1(ctx context.Context, _ toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationObservationV1, error) {
	p.calls.Add(1)
	return p.fixture.observation, ctx.Err()
}
func (p *admittedVerificationPortV1) InspectExactToolPackageVerificationFactV1(ctx context.Context, _ toolcontract.ToolPackageVerificationFactRefV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	p.calls.Add(1)
	return p.fixture.fact, ctx.Err()
}
func (p *admittedVerificationPortV1) AdmitPackageV1(ctx context.Context, _ toolcontract.ToolPackageAdmissionCommandV1) (registry.Record, error) {
	p.calls.Add(1)
	admitted, err := p.target.AdmitVerifiedPackageV1(toolcontract.ToolPackageVerifiedAdmissionRequestV1{
		ContractVersion:          toolcontract.PackageVerificationContractVersionV1,
		PackageCurrent:           p.fixture.current.CurrentPackageRegistry,
		VerificationCurrent:      p.fixture.current,
		ExpectedRegistryRevision: p.fixture.current.CurrentPackageRegistry.RegistryRevision,
	}, time.Unix(1_004, 0))
	if err != nil {
		return registry.Record{}, err
	}
	active := admitted
	active.State = registry.StateActive
	if p.unknown {
		return registry.Record{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost admission reply")
	}
	return active, ctx.Err()
}

// activeRegistryViewV1 models the separate governed Package Enable owner that
// is intentionally outside this Kit. It is test-only and never grants the Kit
// a production enable path.
type activeRegistryViewV1 struct{ *registry.Registry }

func (v *activeRegistryViewV1) ResolvePackage(id string) (toolcontract.ToolPackageManifest, registry.Record, bool) {
	manifest, record, ok := v.Registry.ResolvePackage(id)
	if ok && id == string(PackageIDV1) && record.State == registry.StateAdmitted {
		record.State = registry.StateActive
	}
	return manifest, record, ok
}

func (v *activeRegistryViewV1) Snapshot() (registry.Snapshot, error) {
	snapshot, err := v.Registry.Snapshot()
	if err != nil {
		return registry.Snapshot{}, err
	}
	for i := range snapshot.Records {
		if snapshot.Records[i].Kind == "package" && snapshot.Records[i].ID == string(PackageIDV1) && snapshot.Records[i].State == registry.StateAdmitted {
			snapshot.Records[i].State = registry.StateActive
		}
	}
	digest, err := toolcontract.Seal("praxis.tool-mcp.registry", "v1", "Snapshot", snapshot.Records)
	if err != nil {
		return registry.Snapshot{}, err
	}
	snapshot.Digest = digest
	return snapshot, nil
}

type verificationFixtureForCorePackV1 struct {
	request     toolcontract.ToolPackageVerifyRequestV1
	observation toolcontract.ToolPackageVerificationObservationV1
	fact        toolcontract.ToolPackageVerificationFactV1
	current     toolcontract.ToolPackageVerificationCurrentProjectionV1
}

func admittedAssemblyFixtureV1(t *testing.T, unknown bool) (*CorePackAssemblyKitV1, CorePackAssemblyRequestV1, *admittedVerificationPortV1) {
	t.Helper()
	now := time.Unix(1_000, 0).UTC()
	target := registry.New()
	_, preview := previewAssemblyFixtureV1(t)
	catalog, err := BuildCatalogV1(preview.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	record, err := RegisterV1(target, catalog, now)
	if err != nil {
		t.Fatal(err)
	}
	fixture := buildVerificationFixtureForCorePackV1(t, catalog.Package, record, now)
	port := &admittedVerificationPortV1{target: target, fixture: fixture, unknown: unknown}
	verification, err := sdk.NewPackageVerificationV1(port, port, port)
	if err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewV1(&activeRegistryViewV1{Registry: target}, func() time.Time { return now.Add(4 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	kit, err := NewCorePackAssemblyKitV1(target, client, verification, func() time.Time { return now.Add(4 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	preview.Mode = CorePackAssemblyAdmittedV1
	preview.VerificationRequest = fixture.request
	preview.VerificationIssuance = fixture.current.Issuance
	preview.AdmissionCommand = toolcontract.ToolPackageAdmissionCommandV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, VerificationCurrent: fixture.current.Ref, ExpectedRegistryRevision: fixture.current.CurrentPackageRegistry.RegistryRevision}
	return kit, preview, port
}

func TestCorePackAssemblyAdmittedExactClosureV1(t *testing.T) {
	kit, request, port := admittedAssemblyFixtureV1(t, false)
	result, err := kit.AssembleV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Admitted || result.ReferenceOnly || result.Executable || result.PackageAssembly == nil || result.PackageRecord.State != registry.StateActive {
		t.Fatalf("admitted result drifted: %+v", result)
	}
	if err := result.PackageAssembly.Validate(); err != nil {
		t.Fatal(err)
	}
	if port.calls.Load() != 4 {
		t.Fatalf("unexpected verification call count %d", port.calls.Load())
	}
}

func TestCorePackAssemblyAdmissionUnknownOnlyInspectsWinnerV1(t *testing.T) {
	kit, request, port := admittedAssemblyFixtureV1(t, true)
	result, err := kit.AssembleV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Admitted || port.calls.Load() != 4 {
		t.Fatalf("unknown recovery drifted: result=%+v calls=%d", result, port.calls.Load())
	}
}

func TestCorePackAssemblyAdmittedRejectsForgedAssemblyClosureV1(t *testing.T) {
	kit, request, _ := admittedAssemblyFixtureV1(t, false)
	result, err := kit.AssembleV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CorePackAssemblyResultV1){
		func(v *CorePackAssemblyResultV1) {
			v.PackageAssembly.RegistrySnapshot.Digest = core.DigestBytes([]byte("forged-snapshot"))
		},
		func(v *CorePackAssemblyResultV1) {
			v.PackageAssembly.Package.Digest = core.DigestBytes([]byte("forged-package"))
		},
		func(v *CorePackAssemblyResultV1) { v.PackageAssembly.PackageRecord.RegistryRevision++ },
	} {
		forged := result.Clone()
		mutate(&forged)
		forged.Digest = ""
		digest, sealErr := toolcontract.Seal("praxis.tool-mcp.core-pack-assembly", CorePackAssemblyContractVersionV1, "CorePackAssemblyResultV1", forged)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		forged.Digest = digest
		if err := forged.Validate(); err == nil {
			t.Fatal("forged admitted closure was accepted")
		}
	}
}

func buildVerificationFixtureForCorePackV1(t *testing.T, pkg toolcontract.ToolPackageManifest, record registry.Record, now time.Time) verificationFixtureForCorePackV1 {
	t.Helper()
	digest := func(s string) core.Digest { return core.DigestBytes([]byte(s)) }
	pkgRef := toolcontract.ObjectRef{ID: string(pkg.ID), Revision: pkg.Revision, Digest: pkg.Digest}
	source, err := toolcontract.SealToolPackageRegistryRecordSourceV1(toolcontract.ToolPackageRegistryRecordSourceV1{Kind: "package", ID: string(pkg.ID), ObjectRevision: pkg.Revision, ObjectDigest: pkg.Digest, State: "submitted", RegistryRevision: record.RegistryRevision, UpdatedUnixNano: record.UpdatedUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	currentID, err := toolcontract.DeriveToolPackageRegistryCurrentIDV1(pkgRef)
	if err != nil {
		t.Fatal(err)
	}
	pkgCurrent, err := toolcontract.SealToolPackageRegistryCurrentProjectionV1(toolcontract.ToolPackageRegistryCurrentProjectionV1{Ref: toolcontract.ToolPackageRegistryCurrentRefV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, ID: currentID, Revision: source.RegistryRevision, Digest: source.Digest}, Source: source, Package: pkgRef, Manifest: pkg, State: source.State, RegistryRevision: source.RegistryRevision, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	policy := runtimeports.SupplyChainTrustPolicyRefV1{ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1, ID: "policy-core-pack", Revision: 1, Digest: digest("policy")}
	policyDoc := runtimeports.SupplyChainTrustPolicyDocumentRefV1{ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1, ID: "policy-document-core-pack", Revision: 1, MediaType: toolcontract.PackageTrustPolicyMediaTypeV1, Digest: digest("policy-document"), Size: 128}
	root := runtimeports.SupplyChainTrustMaterialRefV1{ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1, ID: "trust-material-core-pack", Revision: 1, Digest: digest("trust-material")}
	trust, err := runtimeports.SealSupplyChainTrustPolicyCurrentProjectionV1(runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{Ref: runtimeports.SupplyChainTrustPolicyCurrentRefV1{ID: "trust-current-core-pack", Revision: 1}, Policy: policy, PolicyDocument: policyDoc, TrustedRoot: root, IdentityPolicyDigest: digest("identity"), PredicatePolicyDigest: digest("predicate"), TransparencyPolicyDigest: digest("transparency"), TimestampPolicyDigest: digest("timestamp"), MaxPackageArtifactBytes: 1 << 20, MaxSigstoreBundleBytes: 1 << 20, MaxInTotoStatementBytes: 1 << 20, MaxTrustMaterialBytes: 1 << 20, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	content := func(media string, d core.Digest) runtimeports.SupplyChainArtifactContentRefV1 {
		return runtimeports.SupplyChainArtifactContentRefV1{ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1, MediaType: media, Digest: d, Size: 1}
	}
	binding, err := toolcontract.SealToolPackageArtifactBindingV1(toolcontract.ToolPackageArtifactBindingV1{Package: pkgRef, OCIManifest: content("application/vnd.oci.image.manifest.v1+json", digest("oci")), PackageArtifact: content("application/octet-stream", pkg.ArtifactDigest), SigstoreBundle: content("application/vnd.dev.sigstore.bundle.v0.3+json", digest("sigstore")), InTotoStatement: content("application/vnd.in-toto+json", digest("intoto")), ArtifactType: toolcontract.PackageArtifactTypeV1})
	if err != nil {
		t.Fatal(err)
	}
	request := toolcontract.ToolPackageVerifyRequestV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, Subject: toolcontract.ToolPackageVerificationSubjectV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, PackageRegistry: pkgCurrent.Ref, ArtifactBinding: binding, TrustPolicy: policy, VerifierProfile: toolcontract.PackageVerifierConformanceV1}, TrustPolicyCurrent: trust.Ref, RequestedExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()}
	observation, err := toolcontract.SealToolPackageVerificationObservationV1(toolcontract.ToolPackageVerificationObservationV1{Request: toolcontract.ToolPackageVerificationObservationEnsureRequestV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, Subject: request.Subject, TrustPolicyCurrent: trust.Ref, TrustedRoot: root, PolicyDocument: policyDoc, IdentityPolicyDigest: trust.IdentityPolicyDigest, PredicatePolicyDigest: trust.PredicatePolicyDigest, TransparencyPolicyDigest: trust.TransparencyPolicyDigest, TimestampPolicyDigest: trust.TimestampPolicyDigest, SignerIdentityDigest: digest("signer"), PredicateType: "https://slsa.dev/provenance/v1", VerifierConformance: toolcontract.PackageVerifierConformanceV1}, ObservedUnixNano: now.Add(time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := toolcontract.SealToolPackageVerificationFactV1(toolcontract.ToolPackageVerificationFactV1{Package: pkgRef, PackageRegistry: pkgCurrent.Ref, ArtifactBindingDigest: binding.BindingDigest, TrustPolicy: policy, Observation: observation.Ref, SignerIdentityDigest: observation.Request.SignerIdentityDigest, PredicateType: observation.Request.PredicateType, VerifierConformance: observation.Request.VerifierConformance, VerifiedUnixNano: now.Add(2 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	issuance := toolcontract.ToolPackageVerificationCurrentIssuanceV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, Fact: fact.Ref, PackageRegistry: pkgCurrent.Ref, TrustPolicyCurrent: trust.Ref, RequestedExpiresUnixNano: request.RequestedExpiresUnixNano}
	current, err := toolcontract.SealToolPackageVerificationCurrentProjectionV1(toolcontract.ToolPackageVerificationCurrentProjectionV1{Issuance: issuance, Fact: fact, CurrentPackageRegistry: pkgCurrent, TrustPolicy: trust, CheckedUnixNano: now.Add(3 * time.Second).UnixNano(), ExpiresUnixNano: request.RequestedExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	return verificationFixtureForCorePackV1{request: request, observation: observation, fact: fact, current: current}
}
