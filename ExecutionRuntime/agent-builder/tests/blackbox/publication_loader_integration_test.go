package blackbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/compiler"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/loader"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblypublication"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type splicingHistoricalReaderV2 struct {
	delegate *assemblypublication.PublisherV2
}

func (r splicingHistoricalReaderV2) InspectAssemblyPublicationHistoricalV2(ctx context.Context, ref assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationBundleV2, error) {
	bundle, err := r.delegate.InspectAssemblyPublicationHistoricalV2(ctx, ref)
	if err != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, err
	}
	bundle.Manifest.InputDigest = core.DigestBytes([]byte("spliced-input"))
	return bundle, nil
}

func TestPackageLoaderUsesRealHarnessSQLiteHistoricalPublicationAcrossRestart(t *testing.T) {
	ctx := context.Background()
	resolvedResult := resolved(t)
	pkg, err := compiler.NewV1().Compile(resolvedResult)
	if err != nil {
		t.Fatal(err)
	}
	packagePath := t.TempDir() + "/packages.db"
	publicationPath := t.TempDir() + "/publications.db"
	now := time.Unix(1_800_000_000, 0).UTC()

	packageStore, err := repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{Path: packagePath})
	if err != nil {
		t.Fatal(err)
	}
	publicationStore, err := assemblypublication.OpenSQLiteStoreV2(ctx, assemblypublication.SQLiteStoreConfigV2{Path: publicationPath, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := assemblypublication.NewPublisherV2(assemblycompiler.New(), publicationStore, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	published, err := publisher.CompileAndPublishAssemblyV2(ctx, assemblycontract.CompileAndPublishAssemblyRequestV2{
		ContractVersion:          assemblycontract.PublicationContractVersionV2,
		AttemptID:                "agent-package-publication-loader-attempt",
		Input:                    resolvedResult.AssemblyInput,
		ExpectedCurrent:          assemblycontract.AssemblyPublicationCurrentExpectationV2{},
		RequestedExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	publishedRef := assemblycontract.AssemblyPublicationRefV2{PublicationID: published.Publication.PublicationID, Revision: published.Publication.Revision, Digest: published.Publication.Digest}
	if publishedRef != pkg.Lock.PublicationRef {
		t.Fatalf("Package locked a Publication other than the Harness Owner commit: package=%+v owner=%+v", pkg.Lock.PublicationRef, publishedRef)
	}
	if _, err = packageStore.EnsureExactAgentPackageV1(ctx, pkg); err != nil {
		t.Fatal(err)
	}
	if err = packageStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err = publicationStore.Close(); err != nil {
		t.Fatal(err)
	}

	packageStore, err = repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{Path: packagePath})
	if err != nil {
		t.Fatal(err)
	}
	defer packageStore.Close()
	publicationStore, err = assemblypublication.OpenSQLiteStoreV2(ctx, assemblypublication.SQLiteStoreConfigV2{Path: publicationPath, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer publicationStore.Close()
	publisher, err = assemblypublication.NewPublisherV2(assemblycompiler.New(), publicationStore, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	exactLoader, err := loader.NewV1(packageStore, publisher)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := exactLoader.LoadExactV1(ctx, pkg.RefV1())
	if err != nil {
		t.Fatal(err)
	}
	if closure.Package.RefV1() != pkg.RefV1() || closure.Publication.Publication.Digest != pkg.Lock.PublicationRef.Digest {
		t.Fatal("restart did not recover the exact Package-to-Publication closure")
	}

	splicedLoader, err := loader.NewV1(packageStore, splicingHistoricalReaderV2{delegate: publisher})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = splicedLoader.LoadExactV1(ctx, pkg.RefV1()); err == nil {
		t.Fatal("spliced real historical Publication was accepted")
	}
}

func TestPackageLoaderRejectsMissingAndUncommittedHarnessPublication(t *testing.T) {
	ctx := context.Background()
	pkg, bundle := packageAndPublication(t)
	packageStore, err := repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{Path: t.TempDir() + "/packages.db"})
	if err != nil {
		t.Fatal(err)
	}
	defer packageStore.Close()
	if _, err = packageStore.EnsureExactAgentPackageV1(ctx, pkg); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()

	missingStore, err := assemblypublication.OpenSQLiteStoreV2(ctx, assemblypublication.SQLiteStoreConfigV2{Path: t.TempDir() + "/missing.db", Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer missingStore.Close()
	missingPublisher, err := assemblypublication.NewPublisherV2(assemblycompiler.New(), missingStore, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	missingLoader, err := loader.NewV1(packageStore, missingPublisher)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = missingLoader.LoadExactV1(ctx, pkg.RefV1()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("missing historical Publication did not fail closed: %v", err)
	}

	uncommittedStore, err := assemblypublication.OpenSQLiteStoreV2(ctx, assemblypublication.SQLiteStoreConfigV2{Path: t.TempDir() + "/uncommitted.db", Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer uncommittedStore.Close()
	publicationID := bundle.Publication.PublicationID
	if err = uncommittedStore.StageGenerationV2(ctx, publicationID, bundle.Generation); err != nil {
		t.Fatal(err)
	}
	if err = uncommittedStore.StageManifestV2(ctx, publicationID, bundle.Manifest); err != nil {
		t.Fatal(err)
	}
	if err = uncommittedStore.StageGraphV2(ctx, publicationID, bundle.Graph); err != nil {
		t.Fatal(err)
	}
	if err = uncommittedStore.StageHandoffV2(ctx, publicationID, bundle.Handoff); err != nil {
		t.Fatal(err)
	}
	uncommittedPublisher, err := assemblypublication.NewPublisherV2(assemblycompiler.New(), uncommittedStore, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	uncommittedLoader, err := loader.NewV1(packageStore, uncommittedPublisher)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = uncommittedLoader.LoadExactV1(ctx, pkg.RefV1()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged but uncommitted Publication became loadable: %v", err)
	}
}
