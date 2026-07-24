package ports_test

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelPreDispatchAssemblyCurrentV1CanonicalAndExact(t *testing.T) {
	now := time.Unix(2_100_000_000, 0)
	projection := modelPreDispatchAssemblyProjectionV1(t, now)
	if err := projection.ValidateCurrent(projection.Ref, now); err != nil {
		t.Fatal(err)
	}
	digest, err := projection.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	if digest != projection.ProjectionDigest || digest != projection.Ref.Digest || digest != projection.Ref.ProjectionDigest {
		t.Fatalf("projection digest closure drifted: projection=%s ref=%s ref_projection=%s recomputed=%s", projection.ProjectionDigest, projection.Ref.Digest, projection.Ref.ProjectionDigest, digest)
	}
	watermark, err := ports.DigestModelPreDispatchAssemblyWatermarkV1(projection)
	if err != nil || watermark != projection.Ref.WatermarkDigest {
		t.Fatalf("watermark mismatch: got=%s want=%s err=%v", projection.Ref.WatermarkDigest, watermark, err)
	}
	refDigest, err := projection.Ref.DigestV1()
	if err != nil || refDigest != projection.ProjectionDigest {
		t.Fatalf("Ref cannot reproduce projection digest: %s %v", refDigest, err)
	}

	next := modelPreDispatchAssemblyProjectionInputV1(now.Add(time.Second))
	next.Ref.Revision = 2
	next.CurrentnessDigest = modelPreDispatchDigestV1("currentness-next")
	next.Ref.CurrentnessDigest = next.CurrentnessDigest
	next, err = ports.SealModelPreDispatchAssemblyCurrentProjectionV1(next)
	if err != nil {
		t.Fatal(err)
	}
	if next.Ref.ID != projection.Ref.ID {
		t.Fatalf("mutable current watermarks changed stable ID: first=%s next=%s", projection.Ref.ID, next.Ref.ID)
	}
}

func TestModelPreDispatchAssemblyCurrentV1RejectsAllRefAndDigestDrift(t *testing.T) {
	now := time.Unix(2_100_000_100, 0)
	sealed := modelPreDispatchAssemblyProjectionV1(t, now)
	cases := map[string]func(*ports.ModelPreDispatchAssemblyCurrentProjectionV1){
		"ref_generation": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.Generation.Revision++
		},
		"outer_handoff": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Handoff.Revision++
		},
		"binding_semantic": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.BindingSet.SemanticDigest = modelPreDispatchDigestV1("other-binding-semantic")
		},
		"manifest": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Manifest.Digest = modelPreDispatchDigestV1("other-manifest")
		},
		"conformance": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Conformance.Digest = modelPreDispatchDigestV1("other-conformance")
		},
		"tool_surface": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.ToolSurface.Digest = modelPreDispatchDigestV1("other-tool-surface")
		},
		"profile": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.ProfileDigest = modelPreDispatchDigestV1("other-profile")
		},
		"registry_owner": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.RegistrySnapshot.Owner.ID = core.OwnerID("another-registry-owner")
		},
		"semantic": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.SemanticDigest = modelPreDispatchDigestV1("other-semantic")
		},
		"currentness": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.CurrentnessDigest = modelPreDispatchDigestV1("other-currentness")
		},
		"checked": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.CheckedUnixNano++
		},
		"expires": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.ExpiresUnixNano--
		},
		"watermark": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.WatermarkDigest = modelPreDispatchDigestV1("other-watermark")
		},
		"ref_digest": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.Digest = modelPreDispatchDigestV1("other-ref-digest")
		},
		"ref_projection_digest": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.ProjectionDigest = modelPreDispatchDigestV1("other-ref-projection")
		},
		"projection_digest": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.ProjectionDigest = modelPreDispatchDigestV1("other-projection")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := sealed
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("drifted projection passed Validate")
			}
		})
	}
}

func TestModelPreDispatchAssemblyCurrentV1SealRejectsWrongNonzeroDerivedFields(t *testing.T) {
	now := time.Unix(2_100_000_200, 0)
	cases := map[string]func(*ports.ModelPreDispatchAssemblyCurrentProjectionV1){
		"contract": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.ContractVersion = "2.0.0"
		},
		"current_id": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.ID = "model-predispatch-wrong"
		},
		"generation": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.Generation = p.Generation
			p.Ref.Generation.Revision++
		},
		"registry": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.RegistrySnapshot = p.RegistrySnapshot
			p.Ref.RegistrySnapshot.Revision++
		},
		"watermark": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.WatermarkDigest = modelPreDispatchDigestV1("wrong-watermark")
		},
		"ref_digest": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.Digest = modelPreDispatchDigestV1("wrong-ref-digest")
		},
		"ref_projection_digest": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.Ref.ProjectionDigest = modelPreDispatchDigestV1("wrong-ref-projection")
		},
		"projection_digest": func(p *ports.ModelPreDispatchAssemblyCurrentProjectionV1) {
			p.ProjectionDigest = modelPreDispatchDigestV1("wrong-projection")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := modelPreDispatchAssemblyProjectionInputV1(now)
			mutate(&input)
			if _, err := ports.SealModelPreDispatchAssemblyCurrentProjectionV1(input); err == nil {
				t.Fatal("Seal washed a wrong nonzero derived field")
			}
		})
	}
}

func TestRegistrySnapshotRefV1ValidationCanonicalAndExactReaderConformance(t *testing.T) {
	ref := modelPreDispatchRegistryRefV1()
	if err := ref.Validate(); err != nil {
		t.Fatal(err)
	}
	firstDigest, err := ref.CanonicalIdentityDigestV1()
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := ref.CanonicalIdentityDigestV1()
	if err != nil || firstDigest != secondDigest {
		t.Fatalf("Registry Ref canonical digest is not deterministic: %s %s %v", firstDigest, secondDigest, err)
	}
	reader := &modelPreDispatchRegistryReaderV1{stored: ref}
	report, err := conformance.CheckRegistrySnapshotExactReaderV1(context.Background(), conformance.RegistrySnapshotExactReaderCaseV1{Reader: reader, Expected: ref})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ExactCurrentObserved || report.MutationAuthorityUsed || report.ProductionClaimEligible {
		t.Fatalf("Registry Reader conformance widened authority: %+v", report)
	}
	got, err := reader.InspectExactRegistrySnapshotV1(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	got.Owner.Domain = "mutated-return"
	again, err := reader.InspectExactRegistrySnapshotV1(context.Background(), ref)
	if err != nil || again != ref {
		t.Fatalf("Registry Reader leaked an aliased Ref: %+v %v", again, err)
	}
}

func TestRegistrySnapshotExactReaderV1RejectsTypedNilDriftAndPassesClosedErrors(t *testing.T) {
	ref := modelPreDispatchRegistryRefV1()
	var typedNil *modelPreDispatchRegistryReaderV1
	if _, err := conformance.CheckRegistrySnapshotExactReaderV1(context.Background(), conformance.RegistrySnapshotExactReaderCaseV1{Reader: typedNil, Expected: ref}); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Registry Reader did not fail closed: %v", err)
	}

	for name, mutate := range map[string]func(*ports.RegistrySnapshotRefV1){
		"owner":    func(r *ports.RegistrySnapshotRefV1) { r.Owner.ID = core.OwnerID("other-owner") },
		"version":  func(r *ports.RegistrySnapshotRefV1) { r.ContractVersion = "2.0.0" },
		"id":       func(r *ports.RegistrySnapshotRefV1) { r.ID = "registry-other" },
		"revision": func(r *ports.RegistrySnapshotRefV1) { r.Revision++ },
		"digest":   func(r *ports.RegistrySnapshotRefV1) { r.Digest = modelPreDispatchDigestV1("registry-other") },
	} {
		t.Run(name, func(t *testing.T) {
			changed := ref
			mutate(&changed)
			_, err := conformance.CheckRegistrySnapshotExactReaderV1(context.Background(), conformance.RegistrySnapshotExactReaderCaseV1{Reader: &modelPreDispatchRegistryReaderV1{stored: changed}, Expected: ref})
			if !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("Registry drift did not return Conflict: %v", err)
			}
		})
	}

	for _, sentinel := range []error{
		core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "missing"),
		core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "historical only"),
		core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unavailable"),
		core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "unknown"),
	} {
		_, err := conformance.CheckRegistrySnapshotExactReaderV1(context.Background(), conformance.RegistrySnapshotExactReaderCaseV1{Reader: &modelPreDispatchRegistryReaderV1{err: sentinel}, Expected: ref})
		if !errors.Is(err, sentinel) {
			t.Fatalf("Registry Reader error was translated: got=%v want=%v", err, sentinel)
		}
	}
}

func TestModelPreDispatchAssemblyCurrentReaderV1PublicConformanceAndTimeBoundaries(t *testing.T) {
	now := time.Unix(2_100_000_300, 0)
	projection := modelPreDispatchAssemblyProjectionV1(t, now)
	reader := &modelPreDispatchAssemblyReaderV1{projection: projection}
	report, err := conformance.CheckModelPreDispatchAssemblyCurrentReaderV1(context.Background(), conformance.ModelPreDispatchAssemblyCurrentReaderCaseV1{Reader: reader, Expected: projection.Ref, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ExactCurrentObserved || report.PublishAuthorityUsed || report.ProductionClaimEligible {
		t.Fatalf("Assembly Reader conformance widened authority: %+v", report)
	}
	if err := projection.ValidateCurrent(projection.Ref, time.Unix(0, projection.CheckedUnixNano-1)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock regression did not fail closed: %v", err)
	}
	if err := projection.ValidateCurrent(projection.Ref, time.Unix(0, projection.ExpiresUnixNano)); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expiry boundary did not fail closed: %v", err)
	}
	var typedNil *modelPreDispatchAssemblyReaderV1
	if _, err := conformance.CheckModelPreDispatchAssemblyCurrentReaderV1(context.Background(), conformance.ModelPreDispatchAssemblyCurrentReaderCaseV1{Reader: typedNil, Expected: projection.Ref, Now: now}); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Assembly Reader did not fail closed: %v", err)
	}

	wrong := projection
	wrong.Ref.Revision++
	_, err = conformance.CheckModelPreDispatchAssemblyCurrentReaderV1(context.Background(), conformance.ModelPreDispatchAssemblyCurrentReaderCaseV1{Reader: &modelPreDispatchAssemblyReaderV1{projection: wrong}, Expected: projection.Ref, Now: now})
	if err == nil {
		t.Fatal("drifted Assembly projection was accepted")
	}
}

func TestModelPreDispatchAssemblyCurrentV1FrozenPublicShapeAndReaderSignatures(t *testing.T) {
	expectedTags := map[any][]string{
		ports.RegistrySnapshotRefV1{}:                       {"owner", "contract_version", "id", "revision", "digest"},
		ports.ModelPreDispatchAssemblyExactRefV1{}:          {"id", "revision", "digest"},
		ports.ModelPreDispatchAssemblyBindingSetRefV1{}:     {"id", "revision", "digest", "semantic_digest", "currentness_digest", "projection_digest", "expires_unix_nano"},
		ports.ModelPreDispatchAssemblyCurrentRefV1{}:        {"contract_version", "id", "revision", "digest", "generation", "handoff", "binding_set", "manifest", "conformance", "watermark_digest", "tool_surface", "profile_digest", "registry_snapshot", "semantic_digest", "currentness_digest", "checked_unix_nano", "expires_unix_nano", "projection_digest"},
		ports.ModelPreDispatchAssemblyCurrentProjectionV1{}: {"contract_version", "ref", "generation", "handoff", "binding_set", "manifest", "conformance", "tool_surface", "profile_digest", "registry_snapshot", "semantic_digest", "currentness_digest", "checked_unix_nano", "expires_unix_nano", "projection_digest"},
	}
	for value, tags := range expectedTags {
		typeOf := reflect.TypeOf(value)
		if typeOf.NumField() != len(tags) {
			t.Fatalf("%s field count=%d want=%d", typeOf.Name(), typeOf.NumField(), len(tags))
		}
		for index, tag := range tags {
			if got := typeOf.Field(index).Tag.Get("json"); got != tag {
				t.Fatalf("%s field %d tag=%q want=%q", typeOf.Name(), index, got, tag)
			}
		}
	}

	assertReaderSignatureV1(t, reflect.TypeOf((*ports.RegistrySnapshotExactReaderV1)(nil)).Elem(), "InspectExactRegistrySnapshotV1", reflect.TypeOf(ports.RegistrySnapshotRefV1{}), reflect.TypeOf(ports.RegistrySnapshotRefV1{}))
	assertReaderSignatureV1(t, reflect.TypeOf((*ports.ModelPreDispatchAssemblyCurrentReaderV1)(nil)).Elem(), "InspectCurrentModelPreDispatchAssemblyV1", reflect.TypeOf(ports.ModelPreDispatchAssemblyCurrentRefV1{}), reflect.TypeOf(ports.ModelPreDispatchAssemblyCurrentProjectionV1{}))
}

func TestModelPreDispatchAssemblyCurrentV1PublicTypeSetAndImportBoundary(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Surface neutral ports test")
	}
	source := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", "ports", "model_predispatch_assembly_current_v1.go"))
	parsed, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			name := spec.(*ast.TypeSpec).Name.Name
			if name == "RegistrySnapshotRefV1" || name == "RegistrySnapshotExactReaderV1" || len(name) >= len("ModelPreDispatchAssembly") && name[:len("ModelPreDispatchAssembly")] == "ModelPreDispatchAssembly" {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	expected := []string{"ModelPreDispatchAssemblyBindingSetRefV1", "ModelPreDispatchAssemblyCurrentProjectionV1", "ModelPreDispatchAssemblyCurrentReaderV1", "ModelPreDispatchAssemblyCurrentRefV1", "ModelPreDispatchAssemblyExactRefV1", "RegistrySnapshotExactReaderV1", "RegistrySnapshotRefV1"}
	sort.Strings(expected)
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("Surface neutral public type set drifted: got=%v want=%v", names, expected)
	}
	if err := conformance.CheckAdapterRuntimeImportsV2([]string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/core", "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"}); err != nil {
		t.Fatalf("neutral Reader consumer cannot use public Runtime boundary: %v", err)
	}
	if err := conformance.CheckAdapterRuntimeImportsV2([]string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"}); err == nil {
		t.Fatal("neutral Reader consumer was allowed to import Runtime Owner implementation")
	}
}

func modelPreDispatchAssemblyProjectionV1(t *testing.T, now time.Time) ports.ModelPreDispatchAssemblyCurrentProjectionV1 {
	t.Helper()
	projection, err := ports.SealModelPreDispatchAssemblyCurrentProjectionV1(modelPreDispatchAssemblyProjectionInputV1(now))
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func modelPreDispatchAssemblyProjectionInputV1(now time.Time) ports.ModelPreDispatchAssemblyCurrentProjectionV1 {
	expires := now.Add(time.Minute).UnixNano()
	return ports.ModelPreDispatchAssemblyCurrentProjectionV1{
		Ref: ports.ModelPreDispatchAssemblyCurrentRefV1{Revision: 1},
		Generation: ports.GenerationArtifactRefV1{
			ID: "generation-surface", Revision: 3,
			Digest: modelPreDispatchDigestV1("generation"), InputDigest: modelPreDispatchDigestV1("input"), ManifestDigest: modelPreDispatchDigestV1("generation-manifest"), GraphDigest: modelPreDispatchDigestV1("graph"), CatalogDigest: modelPreDispatchDigestV1("catalog"),
		},
		Handoff: ports.ModelPreDispatchAssemblyExactRefV1{ID: "handoff-surface", Revision: 4, Digest: modelPreDispatchDigestV1("handoff")},
		BindingSet: ports.ModelPreDispatchAssemblyBindingSetRefV1{
			ID: "binding-set-surface", Revision: 5, Digest: modelPreDispatchDigestV1("binding-set"), SemanticDigest: modelPreDispatchDigestV1("binding-semantic"), CurrentnessDigest: modelPreDispatchDigestV1("binding-current"), ProjectionDigest: modelPreDispatchDigestV1("binding-projection"), ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano(),
		},
		Manifest:          ports.ModelPreDispatchAssemblyExactRefV1{ID: "manifest-surface", Revision: 6, Digest: modelPreDispatchDigestV1("manifest")},
		Conformance:       ports.ModelPreDispatchAssemblyExactRefV1{ID: "conformance-surface", Revision: 7, Digest: modelPreDispatchDigestV1("conformance")},
		ToolSurface:       ports.ModelPreDispatchAssemblyExactRefV1{ID: "tool-surface", Revision: 8, Digest: modelPreDispatchDigestV1("tool-surface")},
		ProfileDigest:     modelPreDispatchDigestV1("profile"),
		RegistrySnapshot:  modelPreDispatchRegistryRefV1(),
		SemanticDigest:    modelPreDispatchDigestV1("semantic"),
		CurrentnessDigest: modelPreDispatchDigestV1("currentness"),
		CheckedUnixNano:   now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano:   expires,
	}
}

func modelPreDispatchRegistryRefV1() ports.RegistrySnapshotRefV1 {
	return ports.RegistrySnapshotRefV1{
		Owner:           core.OwnerRef{Domain: "registry", ID: core.OwnerID("registry-owner")},
		ContractVersion: "1.0.0",
		ID:              "registry-snapshot",
		Revision:        9,
		Digest:          modelPreDispatchDigestV1("registry-snapshot"),
	}
}

func modelPreDispatchDigestV1(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}

func assertReaderSignatureV1(t *testing.T, reader reflect.Type, methodName string, input, output reflect.Type) {
	t.Helper()
	if reader.NumMethod() != 1 {
		t.Fatalf("%s exposes %d methods, want one", reader.Name(), reader.NumMethod())
	}
	method, ok := reader.MethodByName(methodName)
	if !ok || method.Type.NumIn() != 2 || method.Type.NumOut() != 2 {
		t.Fatalf("%s signature drifted: %+v", methodName, method)
	}
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if method.Type.In(0) != contextType || method.Type.In(1) != input || method.Type.Out(0) != output || method.Type.Out(1) != errorType {
		t.Fatalf("%s parameter/result types drifted: %v", methodName, method.Type)
	}
}

type modelPreDispatchRegistryReaderV1 struct {
	stored ports.RegistrySnapshotRefV1
	err    error
}

func (r *modelPreDispatchRegistryReaderV1) InspectExactRegistrySnapshotV1(context.Context, ports.RegistrySnapshotRefV1) (ports.RegistrySnapshotRefV1, error) {
	if r.err != nil {
		return ports.RegistrySnapshotRefV1{}, r.err
	}
	return r.stored, nil
}

type modelPreDispatchAssemblyReaderV1 struct {
	projection ports.ModelPreDispatchAssemblyCurrentProjectionV1
	err        error
}

func (r *modelPreDispatchAssemblyReaderV1) InspectCurrentModelPreDispatchAssemblyV1(context.Context, ports.ModelPreDispatchAssemblyCurrentRefV1) (ports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if r.err != nil {
		return ports.ModelPreDispatchAssemblyCurrentProjectionV1{}, r.err
	}
	return r.projection, nil
}
