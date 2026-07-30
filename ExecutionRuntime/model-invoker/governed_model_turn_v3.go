package modelinvoker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	GovernedModelTurnContractVersionV3        = "praxis.model-invoker.governed-model-turn/v3"
	GovernedModelTurnAttemptContractVersionV3 = "praxis.model-invoker.governed-model-turn-attempt/v3"
)

type GovernedModelTurnStateV3 string

const GovernedModelTurnPreparedV3 GovernedModelTurnStateV3 = "prepared"

// GovernedModelTurnCommandV3 is the exact pre-provider command. This owner-local
// slice deliberately has no Provider transport or routegateway dependency.
type GovernedModelTurnCommandV3 struct {
	PreparedRef            PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	MaterialRef            InvocationMaterialRefV2             `json:"material_ref"`
	AttemptRequestDigest   core.Digest                         `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                         `json:"route_call_digest"`
	DispatchSequence       uint64                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                              `json:"provider_attempt_ordinal"`
}

type GovernedModelTurnAttemptRefV3 struct {
	ContractVersion        string                              `json:"contract_version"`
	ID                     string                              `json:"id"`
	Digest                 core.Digest                         `json:"digest"`
	PreparedRef            PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	MaterialRef            InvocationMaterialRefV2             `json:"material_ref"`
	AttemptRequestDigest   core.Digest                         `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                         `json:"route_call_digest"`
	DispatchSequence       uint64                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                              `json:"provider_attempt_ordinal"`
}

type GovernedModelTurnRefV3 struct {
	ContractVersion        string                              `json:"contract_version"`
	ID                     string                              `json:"id"`
	Revision               core.Revision                       `json:"revision"`
	Digest                 core.Digest                         `json:"digest"`
	PreparedRef            PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	MaterialRef            InvocationMaterialRefV2             `json:"material_ref"`
	AttemptRequestDigest   core.Digest                         `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                         `json:"route_call_digest"`
	DispatchSequence       uint64                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                              `json:"provider_attempt_ordinal"`
}

type GovernedModelTurnOutcomeV3 struct {
	ContractVersion        string                              `json:"contract_version"`
	ID                     string                              `json:"id"`
	Revision               core.Revision                       `json:"revision"`
	Digest                 core.Digest                         `json:"digest"`
	PreparedRef            PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef             PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	MaterialRef            InvocationMaterialRefV2             `json:"material_ref"`
	AttemptRequestDigest   core.Digest                         `json:"attempt_request_digest"`
	RouteCallDigest        core.Digest                         `json:"route_call_digest"`
	DispatchSequence       uint64                              `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                              `json:"provider_attempt_ordinal"`
	State                  GovernedModelTurnStateV3            `json:"state"`
	CreatedUnixNano        int64                               `json:"created_unix_nano"`
	UpdatedUnixNano        int64                               `json:"updated_unix_nano"`
	ExpiresUnixNano        int64                               `json:"expires_unix_nano"`
}

type GovernedModelTurnMutationV3 struct {
	Outcome GovernedModelTurnOutcomeV3 `json:"outcome"`
	Applied bool                       `json:"applied"`
}

type GovernedModelTurnRepositoryV3 interface {
	EnsurePreparedGovernedModelTurnV3(
		context.Context,
		GovernedModelTurnOutcomeV3,
	) (GovernedModelTurnMutationV3, error)
	InspectGovernedModelTurnAttemptV3(
		context.Context,
		GovernedModelTurnAttemptRefV3,
	) (GovernedModelTurnOutcomeV3, error)
	InspectExactGovernedModelTurnV3(
		context.Context,
		GovernedModelTurnRefV3,
	) (GovernedModelTurnOutcomeV3, error)
}

type GovernedModelTurnPortV3 interface {
	StartOrInspectGovernedModelTurnV3(
		context.Context,
		GovernedModelTurnCommandV3,
	) (GovernedModelTurnOutcomeV3, error)
	InspectGovernedModelTurnAttemptV3(
		context.Context,
		GovernedModelTurnAttemptRefV3,
	) (GovernedModelTurnOutcomeV3, error)
	InspectExactGovernedModelTurnV3(
		context.Context,
		GovernedModelTurnRefV3,
	) (GovernedModelTurnOutcomeV3, error)
}

type GovernedModelTurnOwnerPortConfigV3 struct {
	Repository GovernedModelTurnRepositoryV3
	Clock      func() time.Time
}

type GovernedModelTurnOwnerPortV3 struct {
	repository GovernedModelTurnRepositoryV3
	clock      func() time.Time
}

func (c GovernedModelTurnCommandV3) Validate() error {
	if c.PreparedRef.Validate() != nil || c.CurrentRef.Validate() != nil ||
		c.MaterialRef.Validate() != nil ||
		c.CurrentRef.Prepared != c.PreparedRef ||
		c.MaterialRef.PreparedRef != c.PreparedRef ||
		c.MaterialRef.AuthorizationRef.CurrentRef != c.CurrentRef ||
		c.AttemptRequestDigest.Validate() != nil ||
		c.RouteCallDigest.Validate() != nil ||
		c.AttemptRequestDigest != c.PreparedRef.UnifiedRequestDigest ||
		c.RouteCallDigest != c.MaterialRef.RouteCallDigest ||
		c.DispatchSequence == 0 || c.ProviderAttemptOrdinal == 0 {
		return governedInvalidV1("governed model turn V3 command is invalid")
	}
	return nil
}

func (r GovernedModelTurnAttemptRefV3) Validate() error {
	if r.ContractVersion != GovernedModelTurnAttemptContractVersionV3 ||
		strings.TrimSpace(r.ID) == "" || r.Digest.Validate() != nil {
		return governedInvalidV1("governed model turn V3 AttemptRef identity is invalid")
	}
	command := GovernedModelTurnCommandV3{
		PreparedRef:            r.PreparedRef,
		CurrentRef:             r.CurrentRef,
		MaterialRef:            r.MaterialRef,
		AttemptRequestDigest:   r.AttemptRequestDigest,
		RouteCallDigest:        r.RouteCallDigest,
		DispatchSequence:       r.DispatchSequence,
		ProviderAttemptOrdinal: r.ProviderAttemptOrdinal,
	}
	if err := command.Validate(); err != nil {
		return err
	}
	id, err := governedModelTurnIdentityV3(command)
	if err != nil || id != r.ID {
		return governedConflictV1("governed model turn V3 AttemptRef ID drifted")
	}
	expected, err := governedModelTurnAttemptRefDigestV3(r)
	if err != nil || expected != r.Digest {
		return governedConflictV1("governed model turn V3 AttemptRef digest drifted")
	}
	return nil
}

func (r GovernedModelTurnRefV3) Validate() error {
	if r.ContractVersion != GovernedModelTurnContractVersionV3 ||
		r.Revision != 1 || r.Digest.Validate() != nil {
		return governedInvalidV1("governed model turn V3 exact Ref is invalid")
	}
	attempt := r.AttemptRefV3()
	if err := attempt.Validate(); err != nil {
		return err
	}
	return nil
}

func (r GovernedModelTurnRefV3) AttemptRefV3() GovernedModelTurnAttemptRefV3 {
	attempt := GovernedModelTurnAttemptRefV3{
		ContractVersion:        GovernedModelTurnAttemptContractVersionV3,
		ID:                     r.ID,
		PreparedRef:            r.PreparedRef,
		CurrentRef:             r.CurrentRef,
		MaterialRef:            r.MaterialRef,
		AttemptRequestDigest:   r.AttemptRequestDigest,
		RouteCallDigest:        r.RouteCallDigest,
		DispatchSequence:       r.DispatchSequence,
		ProviderAttemptOrdinal: r.ProviderAttemptOrdinal,
	}
	attempt.Digest, _ = governedModelTurnAttemptRefDigestV3(attempt)
	return attempt
}

func (o GovernedModelTurnOutcomeV3) RefV3() GovernedModelTurnRefV3 {
	return GovernedModelTurnRefV3{
		ContractVersion:        o.ContractVersion,
		ID:                     o.ID,
		Revision:               o.Revision,
		Digest:                 o.Digest,
		PreparedRef:            o.PreparedRef,
		CurrentRef:             o.CurrentRef,
		MaterialRef:            o.MaterialRef,
		AttemptRequestDigest:   o.AttemptRequestDigest,
		RouteCallDigest:        o.RouteCallDigest,
		DispatchSequence:       o.DispatchSequence,
		ProviderAttemptOrdinal: o.ProviderAttemptOrdinal,
	}
}

func (o GovernedModelTurnOutcomeV3) AttemptRefV3() GovernedModelTurnAttemptRefV3 {
	return o.RefV3().AttemptRefV3()
}

func (o GovernedModelTurnOutcomeV3) CloneV3() GovernedModelTurnOutcomeV3 {
	payload, _ := json.Marshal(o)
	var clone GovernedModelTurnOutcomeV3
	_ = core.DecodeStrictJSON(payload, &clone)
	return clone
}

func EncodeGovernedModelTurnOutcomeV3(
	outcome GovernedModelTurnOutcomeV3,
) ([]byte, error) {
	if err := outcome.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(outcome)
}

func DecodeGovernedModelTurnOutcomeV3(
	payload []byte,
) (GovernedModelTurnOutcomeV3, error) {
	var outcome GovernedModelTurnOutcomeV3
	if err := core.DecodeStrictJSON(payload, &outcome); err != nil {
		return GovernedModelTurnOutcomeV3{}, governedInvalidV1(
			"governed model turn V3 payload is invalid",
		)
	}
	if err := outcome.Validate(); err != nil {
		return GovernedModelTurnOutcomeV3{}, err
	}
	canonical, err := json.Marshal(outcome)
	if err != nil || !reflect.DeepEqual(canonical, payload) {
		return GovernedModelTurnOutcomeV3{}, governedConflictV1(
			"governed model turn V3 payload is not canonical",
		)
	}
	return outcome.CloneV3(), nil
}

func (o GovernedModelTurnOutcomeV3) Validate() error {
	command := GovernedModelTurnCommandV3{
		PreparedRef:            o.PreparedRef,
		CurrentRef:             o.CurrentRef,
		MaterialRef:            o.MaterialRef,
		AttemptRequestDigest:   o.AttemptRequestDigest,
		RouteCallDigest:        o.RouteCallDigest,
		DispatchSequence:       o.DispatchSequence,
		ProviderAttemptOrdinal: o.ProviderAttemptOrdinal,
	}
	if o.ContractVersion != GovernedModelTurnContractVersionV3 ||
		command.Validate() != nil || o.Revision != 1 ||
		o.State != GovernedModelTurnPreparedV3 ||
		o.CreatedUnixNano <= 0 || o.UpdatedUnixNano != o.CreatedUnixNano ||
		o.ExpiresUnixNano <= o.CreatedUnixNano ||
		o.ExpiresUnixNano > o.CurrentRef.ExpiresUnixNano ||
		o.ExpiresUnixNano > o.CurrentRef.NotAfterUnixNano ||
		o.ExpiresUnixNano > o.MaterialRef.ExpiresUnixNano {
		return governedInvalidV1("governed model turn V3 prepared outcome is invalid")
	}
	id, err := governedModelTurnIdentityV3(command)
	if err != nil || id != o.ID {
		return governedConflictV1("governed model turn V3 outcome ID drifted")
	}
	expected, err := governedModelTurnDigestV3(o)
	if err != nil || expected != o.Digest {
		return governedConflictV1("governed model turn V3 outcome digest drifted")
	}
	return o.RefV3().Validate()
}

func (o GovernedModelTurnOutcomeV3) ValidateAgainstAttemptRefV3(
	ref GovernedModelTurnAttemptRefV3,
) error {
	if o.Validate() != nil || ref.Validate() != nil ||
		o.AttemptRefV3() != ref {
		return governedConflictV1("governed model turn V3 differs from stable AttemptRef")
	}
	return nil
}

func NewPreparedGovernedModelTurnV3(
	command GovernedModelTurnCommandV3,
	now time.Time,
) (GovernedModelTurnOutcomeV3, error) {
	if err := command.Validate(); err != nil || now.IsZero() ||
		now.UnixNano() < command.CurrentRef.CheckedUnixNano ||
		!now.Before(time.Unix(0, command.CurrentRef.ExpiresUnixNano)) ||
		!now.Before(time.Unix(0, command.CurrentRef.NotAfterUnixNano)) ||
		!now.Before(time.Unix(0, command.MaterialRef.ExpiresUnixNano)) {
		return GovernedModelTurnOutcomeV3{}, governedConflictV1(
			"governed model turn V3 command is not current",
		)
	}
	return SealGovernedModelTurnOutcomeV3(GovernedModelTurnOutcomeV3{
		PreparedRef:            command.PreparedRef,
		CurrentRef:             command.CurrentRef,
		MaterialRef:            command.MaterialRef,
		AttemptRequestDigest:   command.AttemptRequestDigest,
		RouteCallDigest:        command.RouteCallDigest,
		DispatchSequence:       command.DispatchSequence,
		ProviderAttemptOrdinal: command.ProviderAttemptOrdinal,
		State:                  GovernedModelTurnPreparedV3,
		CreatedUnixNano:        now.UnixNano(),
		UpdatedUnixNano:        now.UnixNano(),
		ExpiresUnixNano: minTimeUnixNanoMaterialV1(
			command.CurrentRef.ExpiresUnixNano,
			command.CurrentRef.NotAfterUnixNano,
			command.MaterialRef.ExpiresUnixNano,
		),
	})
}

func SealGovernedModelTurnOutcomeV3(
	outcome GovernedModelTurnOutcomeV3,
) (GovernedModelTurnOutcomeV3, error) {
	command := GovernedModelTurnCommandV3{
		PreparedRef:            outcome.PreparedRef,
		CurrentRef:             outcome.CurrentRef,
		MaterialRef:            outcome.MaterialRef,
		AttemptRequestDigest:   outcome.AttemptRequestDigest,
		RouteCallDigest:        outcome.RouteCallDigest,
		DispatchSequence:       outcome.DispatchSequence,
		ProviderAttemptOrdinal: outcome.ProviderAttemptOrdinal,
	}
	if err := command.Validate(); err != nil {
		return GovernedModelTurnOutcomeV3{}, err
	}
	outcome.ContractVersion = GovernedModelTurnContractVersionV3
	outcome.Revision = 1
	id, err := governedModelTurnIdentityV3(command)
	if err != nil {
		return GovernedModelTurnOutcomeV3{}, err
	}
	if outcome.ID != "" && outcome.ID != id {
		return GovernedModelTurnOutcomeV3{}, governedConflictV1(
			"governed model turn V3 supplied ID drifted",
		)
	}
	outcome.ID = id
	provided := outcome.Digest
	outcome.Digest = ""
	outcome.Digest, err = governedModelTurnDigestV3(outcome)
	if err != nil {
		return GovernedModelTurnOutcomeV3{}, err
	}
	if provided != "" && provided != outcome.Digest {
		return GovernedModelTurnOutcomeV3{}, governedConflictV1(
			"governed model turn V3 supplied digest drifted",
		)
	}
	return outcome, outcome.Validate()
}

func DeriveGovernedModelTurnAttemptRefV3(
	command GovernedModelTurnCommandV3,
) (GovernedModelTurnAttemptRefV3, error) {
	if err := command.Validate(); err != nil {
		return GovernedModelTurnAttemptRefV3{}, err
	}
	id, err := governedModelTurnIdentityV3(command)
	if err != nil {
		return GovernedModelTurnAttemptRefV3{}, err
	}
	ref := GovernedModelTurnAttemptRefV3{
		ContractVersion:        GovernedModelTurnAttemptContractVersionV3,
		ID:                     id,
		PreparedRef:            command.PreparedRef,
		CurrentRef:             command.CurrentRef,
		MaterialRef:            command.MaterialRef,
		AttemptRequestDigest:   command.AttemptRequestDigest,
		RouteCallDigest:        command.RouteCallDigest,
		DispatchSequence:       command.DispatchSequence,
		ProviderAttemptOrdinal: command.ProviderAttemptOrdinal,
	}
	ref.Digest, err = governedModelTurnAttemptRefDigestV3(ref)
	if err != nil {
		return GovernedModelTurnAttemptRefV3{}, err
	}
	return ref, ref.Validate()
}

func NewGovernedModelTurnOwnerPortV3(
	config GovernedModelTurnOwnerPortConfigV3,
) (*GovernedModelTurnOwnerPortV3, error) {
	if nilLikeGovernedModelTurnRepositoryV3(config.Repository) ||
		config.Clock == nil {
		return nil, governedInvalidV1("governed model turn V3 owner port config is invalid")
	}
	return &GovernedModelTurnOwnerPortV3{
		repository: config.Repository,
		clock:      config.Clock,
	}, nil
}

func (p *GovernedModelTurnOwnerPortV3) StartOrInspectGovernedModelTurnV3(
	ctx context.Context,
	command GovernedModelTurnCommandV3,
) (GovernedModelTurnOutcomeV3, error) {
	if p == nil || ctx == nil || ctx.Err() != nil ||
		nilLikeGovernedModelTurnRepositoryV3(p.repository) || p.clock == nil {
		return GovernedModelTurnOutcomeV3{}, governedInvalidV1(
			"governed model turn V3 owner port is unavailable",
		)
	}
	attempt, err := DeriveGovernedModelTurnAttemptRefV3(command)
	if err != nil {
		return GovernedModelTurnOutcomeV3{}, err
	}
	existing, inspectErr := p.repository.InspectGovernedModelTurnAttemptV3(ctx, attempt)
	if inspectErr == nil {
		return validateCurrentGovernedModelTurnWinnerV3(existing, attempt, p.clock())
	}
	if GovernedModelInvocationErrorKindOfV1(inspectErr) !=
		GovernedModelInvocationErrorNotFound {
		return GovernedModelTurnOutcomeV3{}, inspectErr
	}
	outcome, err := NewPreparedGovernedModelTurnV3(command, p.clock())
	if err != nil {
		return GovernedModelTurnOutcomeV3{}, err
	}
	mutation, err := p.repository.EnsurePreparedGovernedModelTurnV3(ctx, outcome)
	if err != nil {
		kind := GovernedModelInvocationErrorKindOfV1(err)
		if kind != GovernedModelInvocationErrorIndeterminate &&
			kind != GovernedModelInvocationErrorConflict {
			return GovernedModelTurnOutcomeV3{}, err
		}
		recoveryContext, cancel := governedModelTurnRecoveryContextV3(ctx, attempt)
		defer cancel()
		recovered, inspectErr := p.repository.InspectGovernedModelTurnAttemptV3(
			recoveryContext,
			attempt,
		)
		if inspectErr != nil {
			return GovernedModelTurnOutcomeV3{}, errors.Join(err, inspectErr)
		}
		return validateCurrentGovernedModelTurnWinnerV3(recovered, attempt, p.clock())
	}
	if mutation.Outcome.RefV3() != outcome.RefV3() ||
		!reflect.DeepEqual(mutation.Outcome, outcome) {
		return GovernedModelTurnOutcomeV3{}, governedConflictV1(
			"governed model turn V3 repository returned different content",
		)
	}
	return validateCurrentGovernedModelTurnWinnerV3(
		mutation.Outcome,
		attempt,
		p.clock(),
	)
}

func validateCurrentGovernedModelTurnWinnerV3(
	outcome GovernedModelTurnOutcomeV3,
	attempt GovernedModelTurnAttemptRefV3,
	now time.Time,
) (GovernedModelTurnOutcomeV3, error) {
	if err := outcome.ValidateAgainstAttemptRefV3(attempt); err != nil ||
		now.IsZero() ||
		now.UnixNano() < outcome.CurrentRef.CheckedUnixNano ||
		!now.Before(time.Unix(0, outcome.ExpiresUnixNano)) ||
		!now.Before(time.Unix(0, outcome.CurrentRef.ExpiresUnixNano)) ||
		!now.Before(time.Unix(0, outcome.CurrentRef.NotAfterUnixNano)) ||
		!now.Before(time.Unix(0, outcome.MaterialRef.ExpiresUnixNano)) {
		return GovernedModelTurnOutcomeV3{}, governedConflictV1(
			"governed model turn V3 historical winner is not current",
		)
	}
	return outcome.CloneV3(), nil
}

func governedModelTurnRecoveryContextV3(
	ctx context.Context,
	attempt GovernedModelTurnAttemptRefV3,
) (context.Context, context.CancelFunc) {
	deadline := time.Unix(0, minTimeUnixNanoMaterialV1(
		attempt.CurrentRef.ExpiresUnixNano,
		attempt.CurrentRef.NotAfterUnixNano,
		attempt.MaterialRef.ExpiresUnixNano,
	))
	return context.WithDeadline(context.WithoutCancel(ctx), deadline)
}

func (p *GovernedModelTurnOwnerPortV3) InspectGovernedModelTurnAttemptV3(
	ctx context.Context,
	ref GovernedModelTurnAttemptRefV3,
) (GovernedModelTurnOutcomeV3, error) {
	if p == nil || nilLikeGovernedModelTurnRepositoryV3(p.repository) {
		return GovernedModelTurnOutcomeV3{}, governedInvalidV1(
			"governed model turn V3 owner port is unavailable",
		)
	}
	return p.repository.InspectGovernedModelTurnAttemptV3(ctx, ref)
}

func (p *GovernedModelTurnOwnerPortV3) InspectExactGovernedModelTurnV3(
	ctx context.Context,
	ref GovernedModelTurnRefV3,
) (GovernedModelTurnOutcomeV3, error) {
	if p == nil || nilLikeGovernedModelTurnRepositoryV3(p.repository) {
		return GovernedModelTurnOutcomeV3{}, governedInvalidV1(
			"governed model turn V3 owner port is unavailable",
		)
	}
	return p.repository.InspectExactGovernedModelTurnV3(ctx, ref)
}

func governedModelTurnIdentityV3(
	command GovernedModelTurnCommandV3,
) (string, error) {
	if err := command.PreparedRef.Validate(); err != nil {
		return "", err
	}
	if err := command.MaterialRef.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(
		"praxis.model-invoker.governed-model-turn",
		"v3",
		"GovernedModelTurnIdentityV3",
		struct {
			PreparedRef            PreparedModelInvocationRefV1 `json:"prepared_ref"`
			MaterialRef            InvocationMaterialRefV2      `json:"material_ref"`
			DispatchSequence       uint64                       `json:"dispatch_sequence"`
			ProviderAttemptOrdinal uint32                       `json:"provider_attempt_ordinal"`
		}{
			command.PreparedRef,
			command.MaterialRef,
			command.DispatchSequence,
			command.ProviderAttemptOrdinal,
		},
	)
	if err != nil {
		return "", err
	}
	return "governed-model-turn-v3/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func governedModelTurnAttemptRefDigestV3(
	ref GovernedModelTurnAttemptRefV3,
) (core.Digest, error) {
	ref.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.governed-model-turn-attempt",
		"v3",
		"GovernedModelTurnAttemptRefV3",
		ref,
	)
}

func governedModelTurnDigestV3(
	outcome GovernedModelTurnOutcomeV3,
) (core.Digest, error) {
	outcome.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.governed-model-turn",
		"v3",
		"GovernedModelTurnOutcomeV3",
		outcome,
	)
}

func nilLikeGovernedModelTurnRepositoryV3(
	repository GovernedModelTurnRepositoryV3,
) bool {
	if repository == nil {
		return true
	}
	value := reflect.ValueOf(repository)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

var _ GovernedModelTurnPortV3 = (*GovernedModelTurnOwnerPortV3)(nil)
