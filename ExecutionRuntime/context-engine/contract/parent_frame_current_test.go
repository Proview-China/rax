package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestParentFrameApplicabilityCoordinateSealsCompleteSubject(t *testing.T) {
	subject := parentFrameSubject(t)
	base, err := contract.SealContextParentFrameApplicabilitySourceCoordinateV1(subject)
	if err != nil {
		t.Fatal(err)
	}
	if base.ID != subject.FrameRef.ID || base.Revision != subject.FrameRef.Revision {
		t.Fatalf("ID/revision must be the exact Frame query key: %+v", base)
	}

	tests := []struct {
		name   string
		mutate func(*contract.ContextParentFrameApplicabilitySubjectV1)
	}{
		{"frame_digest", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.FrameRef.Digest = testkit.D("frame-drift")
		}},
		{"manifest_ref", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.ManifestRef.Digest = testkit.D("manifest-drift")
		}},
		{"generation_ref", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.GenerationRef.Digest = testkit.D("generation-drift")
		}},
		{"generation_ordinal", func(s *contract.ContextParentFrameApplicabilitySubjectV1) { s.GenerationOrdinal++ }},
		{"scope", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.ExecutionScopeDigest = testkit.D("other-scope")
		}},
		{"run", func(s *contract.ContextParentFrameApplicabilitySubjectV1) { s.RunID = "run-2" }},
		{"session", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.SessionRef.Digest = testkit.D("other-session")
		}},
		{"turn", func(s *contract.ContextParentFrameApplicabilitySubjectV1) { s.Turn++ }},
		{"parent_frame", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.ParentFrameRef = &contract.FactRef{ID: "parent-frame", Revision: 1, Digest: testkit.D("parent-frame")}
		}},
		{"parent_generation", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.ParentGenerationRef = &contract.FactRef{ID: "parent-generation", Revision: 1, Digest: testkit.D("parent-generation")}
		}},
		{"parent_binding", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.ParentFrameGenerationBindingDigest = testkit.D("other-parent-binding")
		}},
		{"recipe", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.RecipeRef.Digest = testkit.D("other-recipe")
		}},
		{"authority", func(s *contract.ContextParentFrameApplicabilitySubjectV1) {
			s.AuthorityDigest = testkit.D("other-authority")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := subject
			tt.mutate(&changed)
			coordinate, err := contract.SealContextParentFrameApplicabilitySourceCoordinateV1(changed)
			if err != nil {
				t.Fatal(err)
			}
			if coordinate.ID != base.ID || coordinate.Revision != base.Revision {
				t.Fatalf("non-Frame identity change must keep query key: %+v", coordinate)
			}
			if coordinate.Digest == base.Digest {
				t.Fatal("complete subject change did not change coordinate digest")
			}
		})
	}

	changedFrame := subject
	changedFrame.FrameRef.ID = "frame-2"
	coordinate, err := contract.SealContextParentFrameApplicabilitySourceCoordinateV1(changedFrame)
	if err != nil {
		t.Fatal(err)
	}
	if coordinate.ID != "frame-2" || coordinate.Digest == base.Digest {
		t.Fatalf("Frame identity must change query key and digest: %+v", coordinate)
	}
}

func TestParentFrameCurrentRequestAndProjectionAreExact(t *testing.T) {
	subject := parentFrameSubject(t)
	source, err := contract.SealContextParentFrameApplicabilitySourceCoordinateV1(subject)
	if err != nil {
		t.Fatal(err)
	}
	now := testkit.Now
	request, err := contract.SealContextParentFrameCurrentRequestV1(contract.ContextParentFrameCurrentRequestV1{
		Source: source, Subject: subject, CheckedUnixNano: now, NotAfterUnixNano: now + int64(30*time.Second),
	})
	if err != nil || request.Validate() != nil {
		t.Fatalf("request seal: %v", err)
	}
	projection, err := contract.SealContextParentFrameCurrentProjectionV1(contract.ContextParentFrameCurrentProjectionV1{
		Source: source, FrameRef: subject.FrameRef, ManifestRef: subject.ManifestRef,
		GenerationRef: subject.GenerationRef, GenerationOrdinal: subject.GenerationOrdinal,
		ExecutionScopeDigest: subject.ExecutionScopeDigest, Current: true,
		CheckedUnixNano: now, ExpiresUnixNano: now + int64(time.Second),
	}, now)
	if err != nil || projection.ValidateAt(now) != nil {
		t.Fatalf("projection seal: %v", err)
	}
	projection.Source.Digest = testkit.D("type-pun")
	if projection.ValidateAt(now) == nil {
		t.Fatal("drifted nominal source was accepted")
	}
}

func parentFrameSubject(t *testing.T) contract.ContextParentFrameApplicabilitySubjectV1 {
	t.Helper()
	return contract.ContextParentFrameApplicabilitySubjectV1{
		ContractVersion:                    contract.Version,
		FrameRef:                           contract.FactRef{ID: "frame-1", Revision: 2, Digest: testkit.D("frame")},
		ManifestRef:                        contract.FactRef{ID: "manifest-1", Revision: 3, Digest: testkit.D("manifest")},
		GenerationRef:                      contract.FactRef{ID: "generation-1", Revision: 4, Digest: testkit.D("generation")},
		GenerationOrdinal:                  9,
		ExecutionScopeDigest:               testkit.D("scope"),
		RunID:                              "run-1",
		SessionRef:                         contract.FactRef{ID: "session-1", Revision: 5, Digest: testkit.D("session")},
		Turn:                               7,
		ParentFrameGenerationBindingDigest: testkit.D("parent-binding"),
		RecipeRef:                          contract.FactRef{ID: "recipe-1", Revision: 6, Digest: testkit.D("recipe")},
		AuthorityDigest:                    testkit.D("authority"),
	}
}
