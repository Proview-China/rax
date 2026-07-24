package ports_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewBindingAuthoritativeCurrentV1LiteralIdentityGolden(t *testing.T) {
	input := reviewBindingIdentityInputV1()
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{"source":{"binding_set_id":"binding-set-001","binding_set_revision":7,"component_id":"review/auto-worker","manifest_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","artifact_digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","capability":"review/attest"},"subject":{"tenant_id":"tenant-a","assignment_id":"assignment-001","assignment_revision":3,"assignment_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","reviewer_id":"reviewer-001","target_id":"target-001","target_revision":5,"target_digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444"}}`
	if string(payload) != wantJSON {
		t.Fatalf("identity JSON changed:\n got %s\nwant %s", payload, wantJSON)
	}
	id, err := ports.DeriveReviewBindingProjectionIDV1(input)
	if err != nil {
		t.Fatal(err)
	}
	const wantID = "sha256:a7f1fa4cc093ca2dfb2e0e1aaf1660376d5c883e54d25429346e2614203581cc"
	if id != wantID {
		t.Fatalf("stable Projection ID changed: got=%s want=%s", id, wantID)
	}
}

func TestReviewBindingAuthoritativeCurrentV1ProjectionExactClosureAndGolden(t *testing.T) {
	p := reviewBindingProjectionV1(t)
	now := time.Unix(0, 1_600_000_000)
	if err := p.ValidateCurrent(p.Ref, p.Source, p.Subject, now); err != nil {
		t.Fatal(err)
	}
	if p.Ref.Digest != p.ProjectionDigest {
		t.Fatalf("Ref and projection digests differ: %s %s", p.Ref.Digest, p.ProjectionDigest)
	}
	closure := reviewBindingClosureInputV1(t)
	digest, err := closure.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	const wantClosure = "sha256:e56799384b6aea7fa8ef906c537292d1aa38324c85e631226b49b7214314abc7"
	if string(digest) != wantClosure || p.ClosureDigest != digest {
		t.Fatalf("closure digest changed: input=%s projection=%s want=%s", digest, p.ClosureDigest, wantClosure)
	}
	if p.ConsumerAssociation.Ref.ID != "sha256:c30d58cff3e5e3d477ba99dc71da047e08688f78f76d5a9bc2579cbf752bea11" {
		t.Fatalf("association stable ID changed: %s", p.ConsumerAssociation.Ref.ID)
	}
	if p.ConsumerAssociation.ProjectionDigest != digestV1("9fa5ae3337dc08b68f25b4b3f39ef4b76d33ad885b78adf24142cb19c2fb39ff") {
		t.Fatalf("association projection digest changed: %s", p.ConsumerAssociation.ProjectionDigest)
	}
	if p.ConsumerBinding.ProjectionDigest != digestV1("64727f41813a2e85d4cc068de0603eea9f55c650ddd4ce600731ddd4de867724") {
		t.Fatalf("consumer projection digest changed: %s", p.ConsumerBinding.ProjectionDigest)
	}
}

func TestReviewBindingAuthoritativeCurrentV1StableIdentityAndImmutableHistory(t *testing.T) {
	first := reviewBindingProjectionV1(t)
	next := first.CloneV1()
	next.Ref.Revision++
	next.State = ports.ReviewBindingCurrentSupersededV1
	next.Current = false
	next.Ref.Digest = ""
	next.ProjectionDigest = ""
	next, err := ports.SealReviewBindingCurrentProjectionV1(next)
	if err != nil {
		t.Fatal(err)
	}
	if next.Ref.ID != first.Ref.ID {
		t.Fatalf("revision changed stable Projection ID: %s %s", first.Ref.ID, next.Ref.ID)
	}
	if next.Ref.Digest == first.Ref.Digest {
		t.Fatal("terminal revision reused the historical projection digest")
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("historical active/current projection ceased to be self-valid: %v", err)
	}
	if err := first.ValidateCurrent(first.Ref, first.Source, first.Subject, time.Unix(0, first.ExpiresUnixNano)); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("pure time crossing did not fail current without rewriting history: %v", err)
	}
	if first.Ref.Revision != 1 {
		t.Fatalf("time crossing mutated historical revision: %d", first.Ref.Revision)
	}
}

func TestReviewBindingAuthoritativeCurrentV1RejectsDrift(t *testing.T) {
	sealed := reviewBindingProjectionV1(t)
	cases := map[string]func(*ports.ReviewBindingCurrentProjectionV1){
		"target":      func(p *ports.ReviewBindingCurrentProjectionV1) { p.Subject.TargetRevision++ },
		"set":         func(p *ports.ReviewBindingCurrentProjectionV1) { p.BindingSetRevision++ },
		"member":      func(p *ports.ReviewBindingCurrentProjectionV1) { p.Members[0].BindingRevision++ },
		"selected":    func(p *ports.ReviewBindingCurrentProjectionV1) { p.SelectedGrant.BindingRevision++ },
		"association": func(p *ports.ReviewBindingCurrentProjectionV1) { p.ConsumerAssociation.Ref.Revision++ },
		"consumer":    func(p *ports.ReviewBindingCurrentProjectionV1) { p.ConsumerBinding.BindingRevision++ },
		"closure":     func(p *ports.ReviewBindingCurrentProjectionV1) { p.ClosureDigest = digestLabelV1("other-closure") },
		"checked":     func(p *ports.ReviewBindingCurrentProjectionV1) { p.CheckedUnixNano++ },
		"expires":     func(p *ports.ReviewBindingCurrentProjectionV1) { p.ExpiresUnixNano-- },
		"ref_digest":  func(p *ports.ReviewBindingCurrentProjectionV1) { p.Ref.Digest = digestLabelV1("other-ref") },
		"projection_digest": func(p *ports.ReviewBindingCurrentProjectionV1) {
			p.ProjectionDigest = digestLabelV1("other-projection")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := sealed.CloneV1()
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("drifted projection passed Validate")
			}
		})
	}
}

func TestReviewBindingAuthoritativeCurrentV1TrueMinimumTTLAndCurrentBoundary(t *testing.T) {
	p := reviewBindingProjectionV1(t)
	if p.ExpiresUnixNano != p.ConsumerBinding.ExpiresUnixNano {
		t.Fatalf("Consumer Binding unique minimum TTL was omitted: projection=%d consumer=%d", p.ExpiresUnixNano, p.ConsumerBinding.ExpiresUnixNano)
	}
	if err := p.ValidateCurrent(p.Ref, p.Source, p.Subject, time.Unix(0, p.CheckedUnixNano-1)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback did not fail closed: %v", err)
	}
	if err := p.ValidateCurrent(p.Ref, p.Source, p.Subject, time.Unix(0, p.ExpiresUnixNano-1)); err != nil {
		t.Fatalf("last current nanosecond failed: %v", err)
	}
	if err := p.ValidateCurrent(p.Ref, p.Source, p.Subject, time.Unix(0, p.ExpiresUnixNano)); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("TTL boundary did not fail closed: %v", err)
	}

	changed := p.CloneV1()
	changed.Members[0].SetGrantMinExpiresUnixNano = p.ExpiresUnixNano - 1
	changed.Members[0].FactGrantMinExpiresUnixNano = p.ExpiresUnixNano - 1
	changed.Ref.Digest, changed.ClosureDigest, changed.ProjectionDigest = "", "", ""
	if _, err := ports.SealReviewBindingCurrentProjectionV1(changed); err == nil {
		t.Fatal("projection accepted an Expires value above a member Grant minimum")
	}
}

func TestReviewBindingAuthoritativeCurrentV1StateCurrentTruthTable(t *testing.T) {
	base := reviewBindingProjectionV1(t)
	cases := []struct {
		name    string
		state   ports.ReviewBindingCurrentStateV1
		current bool
		valid   bool
	}{
		{"active_current", ports.ReviewBindingCurrentActiveV1, true, true},
		{"active_historical_false", ports.ReviewBindingCurrentActiveV1, false, false},
		{"revoked_historical", ports.ReviewBindingCurrentRevokedV1, false, true},
		{"revoked_current", ports.ReviewBindingCurrentRevokedV1, true, false},
		{"expired_historical", ports.ReviewBindingCurrentExpiredV1, false, true},
		{"superseded_historical", ports.ReviewBindingCurrentSupersededV1, false, true},
		{"unknown", ports.ReviewBindingCurrentStateV1("unknown"), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base.CloneV1()
			p.State, p.Current = tc.state, tc.current
			p.Ref.Digest, p.ProjectionDigest = "", ""
			sealed, err := ports.SealReviewBindingCurrentProjectionV1(p)
			if tc.valid && err != nil {
				t.Fatalf("valid state rejected: %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("invalid State/Current accepted: %+v", sealed)
			}
			if tc.valid && tc.state != ports.ReviewBindingCurrentActiveV1 {
				if err := sealed.ValidateCurrent(sealed.Ref, sealed.Source, sealed.Subject, time.Unix(0, sealed.CheckedUnixNano)); !core.HasReason(err, core.ReasonBindingExpired) {
					t.Fatalf("terminal projection passed current validation: %v", err)
				}
			}
		})
	}
}

func TestReviewBindingAuthoritativeCurrentV1StrictCanonicalNegatives(t *testing.T) {
	valid, err := json.Marshal(reviewBindingIdentityInputV1())
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"renamed":   []byte(`{"source":{},"bindingSet":{},"subject":{}}`),
		"unknown":   append(valid[:len(valid)-1], []byte(`,"extra":1}`)...),
		"duplicate": []byte(`{"source":{"binding_set_id":"a","binding_set_id":"b"},"subject":{}}`),
		"missing":   []byte(`{"source":` + string(mustJSONV1(t, reviewBindingIdentityInputV1().Source)) + `}`),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			var got ports.ReviewBindingProjectionIdentityInputV1
			err := core.DecodeStrictJSON(payload, &got)
			if err == nil {
				err = got.Validate()
			}
			if err == nil {
				t.Fatal("non-exact JSON was accepted")
			}
			if name == "duplicate" && !core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
				t.Fatalf("duplicate key lost typed error: %v", err)
			}
		})
	}
}

func TestReviewBindingAuthoritativeCurrentV1RequestsPublisherAndReceipt(t *testing.T) {
	p := reviewBindingProjectionV1(t)
	createInput := ports.CreateReviewBindingProjectionCommandInputV1{Source: p.Source, Subject: p.Subject, Association: p.ConsumerAssociation.Ref}
	createRef, err := ports.DeriveCreateReviewBindingProjectionPublishRefV1(createInput)
	if err != nil {
		t.Fatal(err)
	}
	create := ports.CreateReviewBindingProjectionRequestV1{PublishRef: createRef, Input: createInput}
	if err := create.Validate(); err != nil {
		t.Fatal(err)
	}
	changedCreate := create
	changedCreate.Input.Subject.TargetRevision++
	if err := changedCreate.Validate(); err == nil {
		t.Fatal("same PublishRef with changed Create input was accepted")
	}

	casInput := ports.CompareAndSwapReviewBindingProjectionCommandInputV1{ExpectedCurrent: p.Ref, Source: p.Source, Subject: p.Subject, Association: p.ConsumerAssociation.Ref}
	casRef, err := ports.DeriveCompareAndSwapReviewBindingProjectionPublishRefV1(casInput)
	if err != nil {
		t.Fatal(err)
	}
	cas := ports.CompareAndSwapReviewBindingProjectionRequestV1{PublishRef: casRef, Input: casInput}
	if err := cas.Validate(); err != nil {
		t.Fatal(err)
	}
	if createRef == casRef {
		t.Fatal("Create and CAS commands shared one idempotency namespace")
	}

	receipt, err := ports.SealReviewBindingProjectionPublishReceiptV1(ports.ReviewBindingProjectionPublishReceiptV1{PublishRef: createRef, Projection: p.Ref, CurrentIndex: p.Ref, HighestRevision: p.Ref.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	broken := receipt
	broken.HighestRevision++
	if err := broken.Validate(); err == nil {
		t.Fatal("Publish receipt accepted a non-atomic highest revision")
	}
}

func TestReviewBindingAuthoritativeCurrentV1ExactRequestsAndDeepClone(t *testing.T) {
	p := reviewBindingProjectionV1(t)
	resolve := ports.ResolveReviewBindingCurrentRequestV1{Source: p.Source, Subject: p.Subject}
	historical := ports.InspectReviewBindingProjectionRequestV1{Ref: p.Ref, ExpectedSource: p.Source, ExpectedSubject: p.Subject}
	current := ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: p.Ref, ExpectedSource: p.Source, ExpectedSubject: p.Subject}
	for name, validate := range map[string]func() error{"resolve": resolve.Validate, "historical": historical.Validate, "current": current.Validate} {
		if err := validate(); err != nil {
			t.Fatalf("%s request rejected: %v", name, err)
		}
	}
	changed := historical
	changed.ExpectedSubject.TargetDigest = digestLabelV1("other-target")
	if err := changed.Validate(); err == nil {
		t.Fatal("historical request accepted a Ref/Subject mismatch")
	}

	clone := p.CloneV1()
	clone.Members[0].BindingID = "mutated"
	if p.Members[0].BindingID == clone.Members[0].BindingID {
		t.Fatal("projection clone aliases Members")
	}
}

func TestReviewBindingAuthoritativeCurrentV1PublicInterfaceShape(t *testing.T) {
	var _ ports.ReviewBindingAuthoritativeCurrentReaderV1 = reviewBindingReaderV1{}
	var _ ports.ReviewBindingConsumerAssociationCurrentReaderV1 = reviewBindingReaderV1{}
	var _ ports.ReviewBindingProjectionPublisherV1 = reviewBindingPublisherV1{}
}

type reviewBindingReaderV1 struct{}

func (reviewBindingReaderV1) ResolveCurrentReviewBindingV1(context.Context, ports.ResolveReviewBindingCurrentRequestV1) (ports.ReviewBindingProjectionRefV1, error) {
	return ports.ReviewBindingProjectionRefV1{}, nil
}
func (reviewBindingReaderV1) InspectReviewBindingProjectionV1(context.Context, ports.InspectReviewBindingProjectionRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	return ports.ReviewBindingCurrentProjectionV1{}, nil
}
func (reviewBindingReaderV1) InspectCurrentReviewBindingV1(context.Context, ports.InspectCurrentReviewBindingRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	return ports.ReviewBindingCurrentProjectionV1{}, nil
}
func (reviewBindingReaderV1) InspectCurrentReviewBindingConsumerAssociationV1(context.Context, ports.ReviewBindingConsumerAssociationRefV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, nil
}

type reviewBindingPublisherV1 struct{}

func (reviewBindingPublisherV1) CreateReviewBindingProjectionV1(context.Context, ports.CreateReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	return ports.ReviewBindingProjectionPublishReceiptV1{}, nil
}
func (reviewBindingPublisherV1) CompareAndSwapReviewBindingProjectionV1(context.Context, ports.CompareAndSwapReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	return ports.ReviewBindingProjectionPublishReceiptV1{}, nil
}
func (reviewBindingPublisherV1) InspectReviewBindingProjectionPublishV1(context.Context, ports.ReviewBindingProjectionPublishRefV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	return ports.ReviewBindingProjectionPublishReceiptV1{}, nil
}

func reviewBindingProjectionV1(t *testing.T) ports.ReviewBindingCurrentProjectionV1 {
	t.Helper()
	closure := reviewBindingClosureInputV1(t)
	identity := reviewBindingIdentityInputV1()
	p, err := ports.SealReviewBindingCurrentProjectionV1(ports.ReviewBindingCurrentProjectionV1{
		Ref: ports.ReviewBindingProjectionRefV1{Revision: 1}, Source: identity.Source, Subject: identity.Subject,
		State: ports.ReviewBindingCurrentActiveV1, Current: true,
		BindingSetID: closure.BindingSet.ID, BindingSetRevision: closure.BindingSet.Revision,
		BindingSetDigest: closure.BindingSet.Digest, BindingSetSemanticDigest: closure.BindingSet.SemanticDigest,
		BindingSetExpiresUnixNano: closure.BindingSet.ExpiresUnixNano, Members: closure.Members,
		SelectedGrant: closure.SelectedGrant, ConsumerAssociation: closure.ConsumerAssociation,
		ConsumerBinding: closure.ConsumerBinding, CheckedUnixNano: 1_550_000_000, ExpiresUnixNano: closure.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func reviewBindingIdentityInputV1() ports.ReviewBindingProjectionIdentityInputV1 {
	return ports.ReviewBindingProjectionIdentityInputV1{
		Source: ports.ReviewComponentBindingRefV2{
			BindingSetID: "binding-set-001", BindingSetRevision: 7, ComponentID: "review/auto-worker",
			ManifestDigest: digestV1("1111111111111111111111111111111111111111111111111111111111111111"),
			ArtifactDigest: digestV1("2222222222222222222222222222222222222222222222222222222222222222"), Capability: "review/attest",
		},
		Subject: ports.ReviewBindingSubjectV1{
			TenantID: "tenant-a", AssignmentID: "assignment-001", AssignmentRevision: 3,
			AssignmentDigest: digestV1("3333333333333333333333333333333333333333333333333333333333333333"),
			ReviewerID:       "reviewer-001", TargetID: "target-001", TargetRevision: 5,
			TargetDigest: digestV1("4444444444444444444444444444444444444444444444444444444444444444"),
		},
	}
}

func reviewBindingClosureInputV1(t *testing.T) ports.ReviewBindingAuthoritativeClosureInputV1 {
	t.Helper()
	identity := reviewBindingIdentityInputV1()
	consumerRef := ports.ProviderBindingRefV2{
		BindingSetID: "host-binding-set-001", BindingSetRevision: 9, ComponentID: "review/verdict-owner",
		ManifestDigest: digestV1("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		ArtifactDigest: digestV1("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Capability:     "runtime/read-review-binding-current",
	}
	consumer, err := ports.SealProviderBindingCurrentProjectionV2(ports.ProviderBindingCurrentProjectionV2{
		ContractVersion: ports.ProviderBindingCurrentnessContractVersionV2, Ref: consumerRef, State: ports.ProviderBindingCurrentActiveV2,
		BindingSetDigest:         digestV1("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
		BindingSetSemanticDigest: digestV1("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
		BindingID:                "consumer-binding-001", BindingRevision: 4,
		GrantDigest:    digestV1("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"),
		IssuedUnixNano: 1_400_000_000, ExpiresUnixNano: 1_700_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	association, err := ports.SealReviewBindingConsumerAssociationCurrentProjectionV1(ports.ReviewBindingConsumerAssociationCurrentProjectionV1{
		Ref: ports.ReviewBindingConsumerAssociationRefV1{Revision: 2}, Consumer: consumerRef, Source: identity.Source,
		Current: true, CheckedUnixNano: 1_500_000_000, ExpiresUnixNano: 1_750_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ports.ReviewBindingAuthoritativeClosureInputV1{
		Source: identity.Source,
		BindingSet: ports.ReviewBindingSetExactRefV1{ID: "binding-set-001", Revision: 7,
			Digest:         digestV1("5555555555555555555555555555555555555555555555555555555555555555"),
			SemanticDigest: digestV1("6666666666666666666666666666666666666666666666666666666666666666"), ExpiresUnixNano: 2_000_000_000},
		Members: []ports.ReviewBindingMemberCurrentRefV1{{
			ComponentID: "review/auto-worker", BindingID: "binding-001", BindingRevision: 11,
			BindingFactDigest: digestV1("7777777777777777777777777777777777777777777777777777777777777777"),
			ManifestDigest:    identity.Source.ManifestDigest, ArtifactDigest: identity.Source.ArtifactDigest,
			SetGrantSetDigest:          digestV1("8888888888888888888888888888888888888888888888888888888888888888"),
			FactGrantSetDigest:         digestV1("8888888888888888888888888888888888888888888888888888888888888888"),
			BindingFactExpiresUnixNano: 1_900_000_000, SetGrantMinExpiresUnixNano: 1_800_000_000, FactGrantMinExpiresUnixNano: 1_800_000_000,
		}},
		SelectedGrant: ports.ReviewBindingSelectedGrantRefV1{
			ComponentID: "review/auto-worker", BindingID: "binding-001", BindingRevision: 11, Capability: "review/attest",
			SetGrantDigest:  digestV1("9999999999999999999999999999999999999999999999999999999999999999"),
			FactGrantDigest: digestV1("9999999999999999999999999999999999999999999999999999999999999999"), ExpiresUnixNano: 1_800_000_000,
		},
		ConsumerAssociation: association, ConsumerBinding: consumer, ExpiresUnixNano: 1_700_000_000,
	}
}

func digestV1(hex string) core.Digest { return core.Digest("sha256:" + hex) }

func digestLabelV1(label string) core.Digest {
	digest, err := core.CanonicalJSONDigest("praxis.test.review-binding", "1.0.0", "label", label)
	if err != nil {
		panic(err)
	}
	return digest
}

func mustJSONV1(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
