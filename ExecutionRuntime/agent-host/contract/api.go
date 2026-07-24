package contract

import "time"

type ValidateRequestV1 struct {
	Config HostConfigV1 `json:"config"`
}

type ValidateResultV1 struct {
	Definition   DecodedDefinitionV1 `json:"definition"`
	ConfigDigest DigestV1            `json:"config_digest"`
}

type AssembleRequestV1 struct {
	Config HostConfigV1 `json:"config"`
}

type AssembleResultV1 struct {
	Definition DecodedDefinitionV1 `json:"definition"`
	Resolved   ResolvedAssemblyV1  `json:"resolved"`
	Compiled   CompiledAssemblyV1  `json:"compiled"`
}

type StartRequestV1 struct {
	StartID string       `json:"start_id"`
	Config  HostConfigV1 `json:"config"`
}

type StartResultV1 struct {
	Journal ExactRefV1    `json:"journal"`
	Ready   SystemReadyV1 `json:"ready"`
}

type InspectRequestV1 struct {
	HostID  string `json:"host_id"`
	StartID string `json:"start_id"`
}
type InspectResultV1 struct {
	Journal HostJournalV1  `json:"journal"`
	Ready   *SystemReadyV1 `json:"ready,omitempty"`
}
type StopRequestV1 struct {
	HostID  string `json:"host_id"`
	StartID string `json:"start_id"`
}
type StopResultV1 struct {
	Journal ExactRefV1       `json:"journal"`
	Cleanup CleanupSummaryV1 `json:"cleanup"`
}

type ReleaseCurrentV1 struct {
	Domain          string     `json:"domain"`
	ReleaseRef      ExactRefV1 `json:"release_ref"`
	Production      bool       `json:"production"`
	ExpiresUnixNano int64      `json:"expires_unix_nano"`
}

var requiredReleaseDomainsV1 = []string{
	"praxis.continuity", "praxis.tool-mcp", "praxis.memory-knowledge",
	"praxis.sandbox", "praxis.review", "praxis.context-cache", "praxis.harness",
}

func RequiredReleaseDomainsV1() []string { return append([]string(nil), requiredReleaseDomainsV1...) }

type SystemReadyV1 struct {
	ContractVersion string                   `json:"contract_version"`
	HostID          string                   `json:"host_id"`
	StartID         string                   `json:"start_id"`
	DefinitionRef   ExactRefV1               `json:"definition_ref"`
	PlanRef         ExactRefV1               `json:"plan_ref"`
	GenerationRef   ExactRefV1               `json:"generation_ref"`
	HandoffRef      ExactRefV1               `json:"handoff_ref"`
	BindingRef      ExactRefV1               `json:"binding_ref"`
	Components      []ConstructedComponentV1 `json:"components"`
	Releases        []ReleaseCurrentV1       `json:"releases"`
	CheckedUnixNano int64                    `json:"checked_unix_nano"`
	ExpiresUnixNano int64                    `json:"expires_unix_nano"`
	Digest          DigestV1                 `json:"digest"`
}

func (r SystemReadyV1) Validate(now time.Time) error {
	if r.ContractVersion != ContractVersionV1 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "ready contract version is unsupported")
	}
	if err := ValidateIdentifierV1("host id", r.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", r.StartID); err != nil {
		return err
	}
	for _, ref := range []ExactRefV1{r.DefinitionRef, r.PlanRef, r.GenerationRef, r.HandoffRef, r.BindingRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if len(r.Components) == 0 {
		return NewError(ErrorPrecondition, "components_missing", "ready projection has no constructed components")
	}
	seenNodes := map[string]struct{}{}
	for _, component := range r.Components {
		if err := component.Validate(); err != nil {
			return err
		}
		if _, ok := seenNodes[component.NodeID]; ok {
			return NewError(ErrorConflict, "duplicate_component", "ready projection duplicates a component")
		}
		seenNodes[component.NodeID] = struct{}{}
	}
	releases := map[string]ReleaseCurrentV1{}
	for _, release := range r.Releases {
		if err := ValidateIdentifierV1("release domain", release.Domain); err != nil {
			return err
		}
		if err := release.ReleaseRef.Validate(); err != nil {
			return err
		}
		if !release.Production {
			return NewError(ErrorPrecondition, "release_not_production", "required release is not production")
		}
		if release.ExpiresUnixNano <= now.UnixNano() || r.ExpiresUnixNano > release.ExpiresUnixNano {
			return NewError(ErrorPrecondition, "release_not_current", "ready lifetime exceeds a required release")
		}
		if _, ok := releases[release.Domain]; ok {
			return NewError(ErrorConflict, "duplicate_release", "ready projection duplicates a release domain")
		}
		releases[release.Domain] = release
	}
	for _, domain := range requiredReleaseDomainsV1 {
		release, ok := releases[domain]
		if !ok {
			return NewError(ErrorPrecondition, "release_missing", "required production release is missing")
		}
		if release.ExpiresUnixNano <= r.CheckedUnixNano {
			return NewError(ErrorPrecondition, "release_expired", "release expired at readiness check")
		}
	}
	if now.IsZero() || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || now.UnixNano() < r.CheckedUnixNano || now.UnixNano() >= r.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "ready_not_current", "ready projection is not current")
	}
	expected, err := r.digestV1()
	if err != nil {
		return err
	}
	if expected != r.Digest {
		return NewError(ErrorPrecondition, "ready_digest_drift", "ready projection digest drifted")
	}
	return nil
}

func (r SystemReadyV1) digestV1() (DigestV1, error) {
	clone := r
	clone.Digest = ""
	return DigestJSONV1(clone)
}
func SealSystemReadyV1(r SystemReadyV1) (SystemReadyV1, error) {
	digest, err := r.digestV1()
	if err != nil {
		return SystemReadyV1{}, err
	}
	r.Digest = digest
	return r, nil
}
func (r SystemReadyV1) RefV1() (ExactRefV1, error) {
	if err := r.Digest.Validate(); err != nil {
		return ExactRefV1{}, err
	}
	return ExactRefV1{Kind: "praxis.agent-host/system-ready", ID: r.HostID + "/" + r.StartID, Revision: 1, Digest: r.Digest}, nil
}

type CleanupStateV1 string

const (
	CleanupClosedV1        CleanupStateV1 = "closed"
	CleanupResidualV1      CleanupStateV1 = "residual"
	CleanupIndeterminateV1 CleanupStateV1 = "indeterminate"
)

type CleanupItemV1 struct {
	NodeID       string         `json:"node_id"`
	ComponentRef ExactRefV1     `json:"component_ref"`
	State        CleanupStateV1 `json:"state"`
	Reason       string         `json:"reason,omitempty"`
}
type CleanupSummaryV1 struct {
	Items []CleanupItemV1 `json:"items"`
	State CleanupStateV1  `json:"state"`
}
