package bypassowner

import "time"

// SetRecoveryTimeoutForTestV1 shortens the bounded detached recovery window in
// black-box fault tests. Production construction keeps the fixed maximum.
func SetRecoveryTimeoutForTestV1(owner *Owner, timeout time.Duration) {
	owner.recoveryTimeout = timeout
}
