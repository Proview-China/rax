package core

type ExecutionPreconditions struct {
	IdentityEpoch  Epoch    `json:"identity_epoch"`
	InstanceEpoch  Epoch    `json:"instance_epoch"`
	LeaseEpoch     *Epoch   `json:"lease_epoch,omitempty"`
	AuthorityEpoch Epoch    `json:"authority_epoch"`
	Revision       Revision `json:"aggregate_revision"`
}

func (p ExecutionPreconditions) Validate() error {
	if p.IdentityEpoch == 0 || p.InstanceEpoch == 0 || p.AuthorityEpoch == 0 || p.Revision == 0 {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "execution preconditions require identity, instance, authority and revision")
	}
	if p.LeaseEpoch != nil && *p.LeaseEpoch == 0 {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "lease epoch must be non-zero when present")
	}
	return nil
}

type CurrentExecutionFacts struct {
	Scope    ExecutionScope
	Revision Revision
}

func CheckExecutionPreconditions(expected ExecutionPreconditions, current CurrentExecutionFacts) error {
	if err := current.Scope.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if expected.IdentityEpoch != current.Scope.Identity.Epoch {
		return NewError(ErrorPreconditionFailed, ReasonStaleIdentityEpoch, "identity epoch does not match current fact")
	}
	if expected.InstanceEpoch != current.Scope.Instance.Epoch {
		return NewError(ErrorPreconditionFailed, ReasonStaleInstanceEpoch, "instance epoch does not match current fact")
	}
	if expected.AuthorityEpoch != current.Scope.AuthorityEpoch {
		return NewError(ErrorPreconditionFailed, ReasonStaleAuthorityEpoch, "authority epoch does not match current fact")
	}
	if expected.Revision != current.Revision {
		return NewError(ErrorConflict, ReasonRevisionConflict, "aggregate revision does not match current fact")
	}
	if current.Scope.SandboxLease != nil {
		if expected.LeaseEpoch == nil || *expected.LeaseEpoch != current.Scope.SandboxLease.Epoch {
			return NewError(ErrorPreconditionFailed, ReasonStaleLeaseEpoch, "lease epoch does not match current fact")
		}
	} else if expected.LeaseEpoch != nil {
		return NewError(ErrorPreconditionFailed, ReasonStaleLeaseEpoch, "target has no active sandbox lease")
	}
	return nil
}
