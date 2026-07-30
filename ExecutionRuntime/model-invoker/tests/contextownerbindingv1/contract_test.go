package contextownerbindingv1_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var fixtureNowV1 = time.Unix(1_800_000_000, 0)

func TestContextOwnerAuthoritativeIdentityMapsCompleteRawOwnerV1(t *testing.T) {
	owner := rawOwnerV1()
	identity, neutral, err := modelinvoker.MapContextOwnerRefToNeutralOwnerV1(owner)
	if err != nil {
		t.Fatal(err)
	}
	if identity != "sha256:dabae4b62e515a25dd3c6bfa65c7ef8c5ffc6e3edb150054356441c7f3497a60" {
		t.Fatalf("canonical identity vector drifted: %s", identity)
	}
	if neutral.Domain != modelinvoker.ContextNeutralOwnerDomainV1 ||
		neutral.ID != core.OwnerID(identity) {
		t.Fatalf("neutral owner mapping drifted: %+v", neutral)
	}

	componentChanged := owner
	componentChanged.ComponentID = "context-component-drift"
	componentDigest, _, err := modelinvoker.MapContextOwnerRefToNeutralOwnerV1(componentChanged)
	if err != nil {
		t.Fatal(err)
	}
	bindingChanged := owner
	bindingChanged.BindingDigest = digestV1("context-binding-drift")
	bindingDigest, _, err := modelinvoker.MapContextOwnerRefToNeutralOwnerV1(bindingChanged)
	if err != nil {
		t.Fatal(err)
	}
	if componentDigest == identity || bindingDigest == identity || componentDigest == bindingDigest {
		t.Fatal("complete raw Context owner did not bind the neutral identity")
	}
}

func TestContextOwnerBindingAcceptsDistinctMaterialAndFrameExactSourcesV1(t *testing.T) {
	request, projection := fixtureV1(t)
	if projection.Material.Kind == projection.Frame.Kind {
		t.Fatal("fixture lost nominal Kind separation")
	}
	if projection.Material.Digest == projection.Frame.Digest {
		t.Fatal("fixture lost exact digest separation")
	}
	if err := projection.ValidateAgainstV1(request, fixtureNowV1.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestContextOwnerBindingRequestRejectsEveryBoundFieldDriftV1(t *testing.T) {
	request, _ := fixtureV1(t)
	mutations := map[string]func(*modelinvoker.InvocationContextOwnerBindingRequestV1){
		"contract": func(v *modelinvoker.InvocationContextOwnerBindingRequestV1) {
			v.ContractVersion += "-drift"
		},
		"material kind": func(v *modelinvoker.InvocationContextOwnerBindingRequestV1) {
			v.MaterialLookup.Kind += "-drift"
		},
		"material id": func(v *modelinvoker.InvocationContextOwnerBindingRequestV1) {
			v.MaterialLookup.ID += "-drift"
		},
		"material revision": func(v *modelinvoker.InvocationContextOwnerBindingRequestV1) {
			v.MaterialLookup.Revision++
		},
		"material digest": func(v *modelinvoker.InvocationContextOwnerBindingRequestV1) {
			v.MaterialLookup.Digest = digestV1("material-drift")
		},
		"checked": func(v *modelinvoker.InvocationContextOwnerBindingRequestV1) {
			v.CheckedUnixNano++
		},
		"not after": func(v *modelinvoker.InvocationContextOwnerBindingRequestV1) {
			v.NotAfterUnixNano++
		},
		"digest": func(v *modelinvoker.InvocationContextOwnerBindingRequestV1) {
			v.Digest = digestV1("request-drift")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := request
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("drifted request remained valid")
			}
		})
	}
}

func TestContextOwnerBindingProjectionRejectsEveryBoundFieldDriftV1(t *testing.T) {
	request, projection := fixtureV1(t)
	otherOwner := core.OwnerRef{
		Domain: "praxis.context",
		ID:     core.OwnerID(digestV1("other-neutral-owner")),
	}
	mutations := map[string]func(*modelinvoker.InvocationContextOwnerBindingProjectionV1){
		"contract": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.ContractVersion += "-drift"
		},
		"raw component": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.ContextOwner.ComponentID += "-drift"
		},
		"raw binding": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.ContextOwner.BindingDigest = digestV1("binding-drift")
		},
		"identity digest": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.ContextOwnerIdentityDigest = digestV1("identity-drift")
		},
		"neutral domain": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.NeutralOwner.Domain += "-drift"
		},
		"neutral id": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.NeutralOwner.ID = core.OwnerID(digestV1("neutral-drift"))
		},
		"material owner": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Material.Owner = otherOwner
		},
		"material kind": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Material.Kind += "-drift"
		},
		"material id": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Material.ID += "-drift"
		},
		"material revision": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Material.Revision++
		},
		"material digest": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Material.Digest = digestV1("material-drift")
		},
		"frame owner": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Frame.Owner = otherOwner
		},
		"frame kind": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Frame.Kind += "-drift"
		},
		"frame id": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Frame.ID += "-drift"
		},
		"frame revision": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Frame.Revision++
		},
		"frame digest": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.Frame.Digest = digestV1("frame-drift")
		},
		"lineage digest": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.ContextLineageDigest = digestV1("lineage-drift")
		},
		"checked": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.CheckedUnixNano++
		},
		"expires": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.ExpiresUnixNano++
		},
		"projection digest": func(v *modelinvoker.InvocationContextOwnerBindingProjectionV1) {
			v.ProjectionDigest = digestV1("projection-drift")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := projection
			mutate(&changed)
			if err := changed.ValidateAgainstV1(request, fixtureNowV1.Add(2*time.Second)); err == nil {
				t.Fatal("drifted projection remained valid")
			}
		})
	}
}

func TestContextOwnerBindingRejectsLookupTTLAndCanonicalResealDriftV1(t *testing.T) {
	request, projection := fixtureV1(t)

	lookupDrift := request
	lookupDrift.MaterialLookup.ID += "-drift"
	lookupDrift.Digest = ""
	lookupDrift, err := modelinvoker.SealInvocationContextOwnerBindingRequestV1(lookupDrift)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateAgainstV1(lookupDrift, fixtureNowV1.Add(2*time.Second)); err == nil {
		t.Fatal("projection accepted a different sealed lookup")
	}

	if err := projection.ValidateAgainstV1(request, time.Unix(0, projection.ExpiresUnixNano)); err == nil {
		t.Fatal("projection accepted its exclusive expiry boundary")
	}
	if _, err := modelinvoker.SealInvocationContextOwnerBindingProjectionV1(
		func() modelinvoker.InvocationContextOwnerBindingProjectionV1 {
			changed := projection
			changed.ContextLineageDigest = digestV1("reseal-drift")
			return changed
		}(),
		request,
		fixtureNowV1.Add(2*time.Second),
	); err == nil {
		t.Fatal("projection seal accepted a stale supplied canonical digest")
	}
}

func TestContextOwnerBindingReaderIsReadOnlyPortAndValidationCallsNoDownstreamV1(t *testing.T) {
	request, projection := fixtureV1(t)
	reader := &countingReaderV1{projection: projection}
	var port modelinvoker.InvocationContextOwnerBindingReaderV1 = reader

	if err := projection.ValidateAgainstV1(request, fixtureNowV1.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 0 {
		t.Fatalf("DTO validation called the Reader Port %d times", reader.calls)
	}

	got, err := port.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || !reflect.DeepEqual(got, projection) {
		t.Fatalf("read-only port mismatch: calls=%d got=%+v", reader.calls, got)
	}
}

type countingReaderV1 struct {
	calls      int
	projection modelinvoker.InvocationContextOwnerBindingProjectionV1
}

func (r *countingReaderV1) InspectCurrentInvocationContextOwnerBindingV1(
	context.Context,
	modelinvoker.InvocationContextOwnerBindingRequestV1,
) (modelinvoker.InvocationContextOwnerBindingProjectionV1, error) {
	r.calls++
	return r.projection, nil
}

func fixtureV1(
	t *testing.T,
) (
	modelinvoker.InvocationContextOwnerBindingRequestV1,
	modelinvoker.InvocationContextOwnerBindingProjectionV1,
) {
	t.Helper()
	request, err := modelinvoker.SealInvocationContextOwnerBindingRequestV1(
		modelinvoker.InvocationContextOwnerBindingRequestV1{
			MaterialLookup: modelinvoker.ContextMaterialLookupV1{
				Kind:     "caller-validated-context-material-kind",
				ID:       "context-material-1",
				Revision: 7,
				Digest:   digestV1("context-material"),
			},
			CheckedUnixNano:  fixtureNowV1.UnixNano(),
			NotAfterUnixNano: fixtureNowV1.Add(30 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, neutral, err := modelinvoker.MapContextOwnerRefToNeutralOwnerV1(rawOwnerV1())
	if err != nil {
		t.Fatal(err)
	}
	projection, err := modelinvoker.SealInvocationContextOwnerBindingProjectionV1(
		modelinvoker.InvocationContextOwnerBindingProjectionV1{
			ContextOwner: rawOwnerV1(),
			Material: modelinvoker.InvocationMaterialExactSourceRefV1{
				Owner:    neutral,
				Kind:     request.MaterialLookup.Kind,
				ID:       request.MaterialLookup.ID,
				Revision: request.MaterialLookup.Revision,
				Digest:   request.MaterialLookup.Digest,
			},
			Frame: modelinvoker.InvocationMaterialExactSourceRefV1{
				Owner:    neutral,
				Kind:     "caller-validated-context-frame-kind",
				ID:       "context-frame-1",
				Revision: 11,
				Digest:   digestV1("context-frame"),
			},
			ContextLineageDigest: digestV1("context-lineage"),
			CheckedUnixNano:      fixtureNowV1.Add(time.Second).UnixNano(),
			ExpiresUnixNano:      fixtureNowV1.Add(20 * time.Second).UnixNano(),
		},
		request,
		fixtureNowV1.Add(2*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	return request, projection
}

func rawOwnerV1() modelinvoker.ContextOwnerRef {
	return modelinvoker.ContextOwnerRef{
		ComponentID:   "components/context",
		BindingDigest: digestV1("context-binding"),
	}
}

func digestV1(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}
