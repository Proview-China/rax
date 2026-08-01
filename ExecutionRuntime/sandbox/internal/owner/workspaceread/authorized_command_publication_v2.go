package workspaceread

import (
	"errors"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

const authorizedCommandPublicationContractVersionV2 = "praxis.sandbox/workspace-read-command-publication-owner/v2"

type CommandPublicationMutationV2 string

const (
	CommandPublicationInitialV2 CommandPublicationMutationV2 = "initial"
	CommandPublicationRefreshV2 CommandPublicationMutationV2 = "refresh"
)

// AuthorizedCommandPublicationV2 is the Sandbox-internal write capability.
// Its fields are private so an external Owner cannot construct a Command,
// Publication, or OwnerCurrent write request. Public consumers only receive
// the read-only Sandbox ports.
type AuthorizedCommandPublicationV2 struct {
	mutation    CommandPublicationMutationV2
	command     contract.WorkspaceReadCommandV1
	publication contract.WorkspaceReadCommandPublicationV2
	expected    contract.WorkspaceReadCommandOwnerCurrentV2
	current     contract.WorkspaceReadCommandOwnerCurrentV2
	source      contract.WorkspaceReadSourceCurrentProjectionV2
	effect      runtimeports.ControlledOperationEffectCurrentProjectionV2
	prepared    runtimeports.ControlledOperationPreparedCurrentProjectionV2
	workspace   contract.WorkspaceView
	checkedNano int64
	seal        runtimecore.Digest
}

func NewInitialCommandPublicationV2(
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	current contract.WorkspaceReadCommandOwnerCurrentV2,
	source contract.WorkspaceReadSourceCurrentProjectionV2,
	effect runtimeports.ControlledOperationEffectCurrentProjectionV2,
	prepared runtimeports.ControlledOperationPreparedCurrentProjectionV2,
	workspace contract.WorkspaceView,
	checked time.Time,
) (AuthorizedCommandPublicationV2, error) {
	if current.Meta.Revision != 1 ||
		command.Meta.CreatedUnixNano != checked.UnixNano() ||
		publication.Meta.CreatedUnixNano != checked.UnixNano() ||
		current.Meta.CreatedUnixNano != checked.UnixNano() {
		return AuthorizedCommandPublicationV2{}, errors.New("workspace read initial owner facts do not share one commit clock")
	}
	return newAuthorizedCommandPublicationV2(
		CommandPublicationInitialV2,
		command,
		publication,
		contract.WorkspaceReadCommandOwnerCurrentV2{},
		current,
		source,
		effect,
		prepared,
		workspace,
		checked,
	)
}

func NewRefreshCommandPublicationV2(
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	expected contract.WorkspaceReadCommandOwnerCurrentV2,
	current contract.WorkspaceReadCommandOwnerCurrentV2,
	source contract.WorkspaceReadSourceCurrentProjectionV2,
	effect runtimeports.ControlledOperationEffectCurrentProjectionV2,
	prepared runtimeports.ControlledOperationPreparedCurrentProjectionV2,
	workspace contract.WorkspaceView,
	checked time.Time,
) (AuthorizedCommandPublicationV2, error) {
	resealed, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(
		current,
		expected,
		checked,
	)
	if err != nil || !reflect.DeepEqual(resealed, current) {
		return AuthorizedCommandPublicationV2{}, errors.New("workspace read refreshed Owner current is not the exact successor")
	}
	return newAuthorizedCommandPublicationV2(
		CommandPublicationRefreshV2,
		command,
		publication,
		expected,
		current,
		source,
		effect,
		prepared,
		workspace,
		checked,
	)
}

func newAuthorizedCommandPublicationV2(
	mutation CommandPublicationMutationV2,
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	expected contract.WorkspaceReadCommandOwnerCurrentV2,
	current contract.WorkspaceReadCommandOwnerCurrentV2,
	source contract.WorkspaceReadSourceCurrentProjectionV2,
	effect runtimeports.ControlledOperationEffectCurrentProjectionV2,
	prepared runtimeports.ControlledOperationPreparedCurrentProjectionV2,
	workspace contract.WorkspaceView,
	checked time.Time,
) (AuthorizedCommandPublicationV2, error) {
	if checked.IsZero() ||
		mutation != CommandPublicationInitialV2 && mutation != CommandPublicationRefreshV2 {
		return AuthorizedCommandPublicationV2{}, errors.New("workspace read owner publication capability is incomplete")
	}
	if err := contract.ValidateWorkspaceReadCommandOwnerClosureV2(command, publication, current); err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	if err := contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
		current,
		source,
		effect,
		prepared,
		workspace,
		checked,
	); err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	commandClone, err := cloneJSON(command)
	if err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	publicationClone, err := cloneJSON(publication)
	if err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	expectedClone, err := cloneJSON(expected)
	if err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	currentClone, err := cloneJSON(current)
	if err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	sourceClone, err := cloneJSON(source)
	if err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	effectClone, err := cloneJSON(effect)
	if err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	preparedClone, err := cloneJSON(prepared)
	if err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	workspaceClone, err := cloneJSON(workspace)
	if err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	value := AuthorizedCommandPublicationV2{
		mutation: mutation, command: commandClone, publication: publicationClone,
		expected: expectedClone, current: currentClone, source: sourceClone,
		effect: effectClone, prepared: preparedClone, workspace: workspaceClone,
		checkedNano: checked.UnixNano(),
	}
	seal, err := value.digest()
	if err != nil {
		return AuthorizedCommandPublicationV2{}, err
	}
	value.seal = seal
	return value, nil
}

func (value AuthorizedCommandPublicationV2) Open(
	now time.Time,
) (
	CommandPublicationMutationV2,
	contract.WorkspaceReadCommandV1,
	contract.WorkspaceReadCommandPublicationV2,
	contract.WorkspaceReadCommandOwnerCurrentV2,
	contract.WorkspaceReadCommandOwnerCurrentV2,
	error,
) {
	emptyCommand := contract.WorkspaceReadCommandV1{}
	emptyPublication := contract.WorkspaceReadCommandPublicationV2{}
	emptyCurrent := contract.WorkspaceReadCommandOwnerCurrentV2{}
	if now.IsZero() || value.checkedNano <= 0 || now.UnixNano() < value.checkedNano {
		return "", emptyCommand, emptyPublication, emptyCurrent, emptyCurrent, errors.New("workspace read owner publication capability clock regressed")
	}
	expectedSeal, err := value.digest()
	if err != nil || expectedSeal != value.seal {
		return "", emptyCommand, emptyPublication, emptyCurrent, emptyCurrent, errors.New("workspace read owner publication capability seal drifted")
	}
	if err = contract.ValidateWorkspaceReadCommandOwnerClosureV2(
		value.command,
		value.publication,
		value.current,
	); err != nil {
		return "", emptyCommand, emptyPublication, emptyCurrent, emptyCurrent, err
	}
	if err = contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
		value.current,
		value.source,
		value.effect,
		value.prepared,
		value.workspace,
		now,
	); err != nil {
		return "", emptyCommand, emptyPublication, emptyCurrent, emptyCurrent, err
	}
	command, err := cloneJSON(value.command)
	if err != nil {
		return "", emptyCommand, emptyPublication, emptyCurrent, emptyCurrent, err
	}
	publication, err := cloneJSON(value.publication)
	if err != nil {
		return "", emptyCommand, emptyPublication, emptyCurrent, emptyCurrent, err
	}
	expected, err := cloneJSON(value.expected)
	if err != nil {
		return "", emptyCommand, emptyPublication, emptyCurrent, emptyCurrent, err
	}
	current, err := cloneJSON(value.current)
	if err != nil {
		return "", emptyCommand, emptyPublication, emptyCurrent, emptyCurrent, err
	}
	return value.mutation, command, publication, expected, current, nil
}

func (value AuthorizedCommandPublicationV2) digest() (runtimecore.Digest, error) {
	return runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read-owner",
		authorizedCommandPublicationContractVersionV2,
		"AuthorizedCommandPublicationV2",
		struct {
			Mutation    CommandPublicationMutationV2                                `json:"mutation"`
			Command     contract.WorkspaceReadCommandV1                             `json:"command"`
			Publication contract.WorkspaceReadCommandPublicationV2                  `json:"publication"`
			Expected    contract.WorkspaceReadCommandOwnerCurrentV2                 `json:"expected"`
			Current     contract.WorkspaceReadCommandOwnerCurrentV2                 `json:"current"`
			Source      contract.WorkspaceReadSourceCurrentProjectionV2             `json:"source"`
			Effect      runtimeports.ControlledOperationEffectCurrentProjectionV2   `json:"effect"`
			Prepared    runtimeports.ControlledOperationPreparedCurrentProjectionV2 `json:"prepared"`
			Workspace   contract.WorkspaceView                                      `json:"workspace"`
			CheckedNano int64                                                       `json:"checked_unix_nano"`
		}{
			Mutation: value.mutation, Command: value.command, Publication: value.publication,
			Expected: value.expected, Current: value.current, Source: value.source,
			Effect: value.effect, Prepared: value.prepared, Workspace: value.workspace,
			CheckedNano: value.checkedNano,
		},
	)
}
