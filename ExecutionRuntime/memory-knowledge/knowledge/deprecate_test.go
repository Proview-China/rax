package knowledge

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgeDeprecatePreservesHistoryAndBlocksNewPackage(t *testing.T) {
	f := newFixture(t, false)
	deprecated, err := f.store.DeprecateSource(f.access, f.source.Ref.ID, "superseded source", contract.ExpectRevision(f.source.Ref.Revision))
	if err != nil || deprecated.State != SourceDeprecated {
		t.Fatalf("deprecated=%+v %v", deprecated, err)
	}
	historical, err := f.store.GetSource(f.access, f.source.Ref)
	if err != nil || historical.State != SourceAvailable {
		t.Fatalf("history=%+v %v", historical, err)
	}
	_, err = f.store.PutPackage(f.access, PackageInput{TenantID: f.access.TenantID, ID: "deprecated-package", Version: "v1", SourceRefs: []contract.Ref{deprecated.Ref}, AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef, License: "internal-use", Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, State: PackageReady}, contract.ExpectAbsent())
	if !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("deprecated source packaged: %v", err)
	}
}
