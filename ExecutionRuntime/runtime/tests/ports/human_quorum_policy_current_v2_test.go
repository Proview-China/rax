package ports_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHumanQuorumPolicyCurrentV2CanonicalSealAndCurrentness(t *testing.T) {
	now := time.Unix(2_500_000_000, 0)
	projection := humanQuorumPolicyProjectionV2(t, now, "tenant-a", "production/release", 1)
	if projection.RoleRequirements[0].Role != "legal" || projection.RoleRequirements[1].Role != "security" || !reflect.DeepEqual(projection.RejectVetoRoles, []string{"legal", "security"}) {
		t.Fatalf("Seal did not canonicalize role sets: %+v %+v", projection.RoleRequirements, projection.RejectVetoRoles)
	}
	if err := projection.ValidateCurrent(projection.Ref, projection.Subject, now); err != nil {
		t.Fatal(err)
	}
	wantID, err := ports.DeriveHumanQuorumPolicyCurrentProjectionIDV2(projection.Subject)
	if err != nil || projection.Ref.ID != wantID || projection.Ref.Digest != projection.ProjectionDigest {
		t.Fatalf("stable identity or digest drifted: %s %s %v", projection.Ref.ID, wantID, err)
	}
	clone := projection.Clone()
	clone.RoleRequirements[0].Role = "mutated"
	clone.RejectVetoRoles[0] = "mutated"
	if projection.RoleRequirements[0].Role == "mutated" || projection.RejectVetoRoles[0] == "mutated" {
		t.Fatal("Clone leaked mutable role slices")
	}
	if err := projection.ValidateCurrent(projection.Ref, projection.Subject, now.Add(31*time.Second)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("expired projection remained current: %v", err)
	}
	if err := projection.ValidateCurrent(projection.Ref, projection.Subject, now.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback remained current: %v", err)
	}
}

func TestHumanQuorumPolicyCurrentV2HardNegatives(t *testing.T) {
	now := time.Unix(2_510_000_000, 0)
	base := humanQuorumPolicyProjectionV2(t, now, "tenant-negative", "finance/payment", 1)
	cases := map[string]func(*ports.HumanQuorumPolicyCurrentProjectionV2){
		"zero K":      func(p *ports.HumanQuorumPolicyCurrentProjectionV2) { p.AcceptThreshold = 0 },
		"K exceeds N": func(p *ports.HumanQuorumPolicyCurrentProjectionV2) { p.AcceptThreshold = p.MaximumPanelSize + 1 },
		"duplicate role": func(p *ports.HumanQuorumPolicyCurrentProjectionV2) {
			p.RoleRequirements = append(p.RoleRequirements, p.RoleRequirements[0])
		},
		"role minima exceed N": func(p *ports.HumanQuorumPolicyCurrentProjectionV2) {
			p.RoleRequirements[0].Minimum = p.MaximumPanelSize
		},
		"duplicate veto": func(p *ports.HumanQuorumPolicyCurrentProjectionV2) {
			p.RejectVetoRoles = append(p.RejectVetoRoles, p.RejectVetoRoles[0])
		},
		"delegation optional":    func(p *ports.HumanQuorumPolicyCurrentProjectionV2) { p.DelegationRequired = false },
		"production self review": func(p *ports.HumanQuorumPolicyCurrentProjectionV2) { p.ProductionSelfReviewAllowed = true },
		"zero panel duration":    func(p *ports.HumanQuorumPolicyCurrentProjectionV2) { p.MaxPanelDurationNanos = 0 },
		"zero vote TTL":          func(p *ports.HumanQuorumPolicyCurrentProjectionV2) { p.MaxVoteTTLNanos = 0 },
		"active not current":     func(p *ports.HumanQuorumPolicyCurrentProjectionV2) { p.Current = false },
		"bad stable ID":          func(p *ports.HumanQuorumPolicyCurrentProjectionV2) { p.Ref.ID = "other-id" },
		"digest drift": func(p *ports.HumanQuorumPolicyCurrentProjectionV2) {
			p.ProjectionDigest = core.DigestBytes([]byte("drift"))
			p.Ref.Digest = p.ProjectionDigest
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := base.Clone()
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatalf("invalid policy was accepted: %+v", changed)
			}
		})
	}
}

func TestHumanQuorumPolicyCurrentV2TerminalAndPublishShape(t *testing.T) {
	now := time.Unix(2_520_000_000, 0)
	first := humanQuorumPolicyProjectionV2(t, now, "tenant-shape", "legal/opinion", 1)
	if err := (ports.HumanQuorumPolicyCurrentPublishRequestV2{Value: first}).Validate(); err != nil {
		t.Fatal(err)
	}
	terminal := first.Clone()
	terminal.Ref.Revision = 2
	terminal.State = ports.HumanQuorumPolicyProjectionRevokedV2
	terminal.Current = false
	terminal.CheckedUnixNano = now.Add(time.Second).UnixNano()
	terminal, err := ports.SealHumanQuorumPolicyCurrentProjectionV2(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if err := terminal.ValidateCurrent(terminal.Ref, terminal.Subject, now.Add(time.Second)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("terminal policy was accepted as current: %v", err)
	}
	request := ports.HumanQuorumPolicyCurrentPublishRequestV2{Previous: &first.Ref, Value: terminal}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	gap := terminal
	gap.Ref.Revision++
	gap, err = ports.SealHumanQuorumPolicyCurrentProjectionV2(gap)
	if err != nil {
		t.Fatal(err)
	}
	if err := (ports.HumanQuorumPolicyCurrentPublishRequestV2{Previous: &first.Ref, Value: gap}).Validate(); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("revision gap was accepted: %v", err)
	}
}

func TestHumanQuorumPolicyCurrentV2JSONShapeIsNominal(t *testing.T) {
	now := time.Unix(2_530_000_000, 0)
	projection := humanQuorumPolicyProjectionV2(t, now, "tenant-json", "artifact/quality", 1)
	payload, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"ref", "subject", "accept_threshold", "maximum_panel_size", "role_requirements", "reject_veto_roles", "delegation_required", "production_self_review_allowed", "max_panel_duration_nanos", "max_vote_ttl_nanos", "checked_unix_nano", "expires_unix_nano", "projection_digest"} {
		if _, ok := object[field]; !ok {
			t.Fatalf("neutral projection omitted JSON field %q", field)
		}
	}
}

func humanQuorumPolicyProjectionV2(t *testing.T, now time.Time, tenant core.TenantID, domain string, revision core.Revision) ports.HumanQuorumPolicyCurrentProjectionV2 {
	t.Helper()
	projection, err := ports.SealHumanQuorumPolicyCurrentProjectionV2(ports.HumanQuorumPolicyCurrentProjectionV2{
		Ref:                         ports.HumanQuorumPolicyCurrentProjectionRefV2{Revision: revision},
		Subject:                     ports.HumanQuorumPolicyCurrentSubjectV2{TenantID: tenant, Domain: domain},
		State:                       ports.HumanQuorumPolicyProjectionActiveV2,
		Current:                     true,
		AcceptThreshold:             2,
		MaximumPanelSize:            3,
		RoleRequirements:            []ports.HumanQuorumRoleRequirementV2{{Role: "security", Minimum: 1}, {Role: "legal", Minimum: 1}},
		RejectVetoRoles:             []string{"security", "legal"},
		DelegationRequired:          true,
		ProductionSelfReviewAllowed: false,
		MaxPanelDurationNanos:       (24 * time.Hour).Nanoseconds(),
		MaxVoteTTLNanos:             time.Hour.Nanoseconds(),
		CheckedUnixNano:             now.UnixNano(),
		ExpiresUnixNano:             now.Add(30 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}
