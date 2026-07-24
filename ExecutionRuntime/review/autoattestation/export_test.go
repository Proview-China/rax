package autoattestation

import "time"

// SetRecoveryTimeoutForTestV1 shortens the bounded detached recovery window in
// black-box fault tests. Production constructors always use the fixed maximum.
func SetRecoveryTimeoutForTestV1(owner *OwnerV1, timeout time.Duration) {
	owner.recoveryTimeout = timeout
}
