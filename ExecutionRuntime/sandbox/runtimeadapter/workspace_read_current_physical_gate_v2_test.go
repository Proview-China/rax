package runtimeadapter

import "testing"

func TestWorkspaceReadCurrentV2PhysicalMarkerCannotBeClaimedByRawV1Adapter(t *testing.T) {
	var raw any = (*WorkspaceReadCurrentAdapterV1)(nil)
	if _, ok := raw.(workspaceReadPublishedCurrentProjectionReaderV2); ok {
		t.Fatal("raw V1 current adapter satisfied the physical V2 marker")
	}
	var published any = (*WorkspaceReadPublishedCurrentAdapterV2)(nil)
	if _, ok := published.(workspaceReadPublishedCurrentProjectionReaderV2); !ok {
		t.Fatal("Sandbox Owner published-current adapter lost its physical marker")
	}
	if (&WorkspaceReadPublishedCurrentAdapterV2{}).workspaceReadPublishedCurrentV2() {
		t.Fatal("zero/externally constructible published adapter claimed the physical marker")
	}
}

func TestWorkspaceReadCurrentV2ReferenceAdapterIsNeverPhysicalQualified(t *testing.T) {
	reference := &WorkspaceReadCurrentAdapterV2{}
	if reference.PhysicalQualifiedV2() {
		t.Fatal("zero/reference V2 adapter became physical-qualified")
	}
}
