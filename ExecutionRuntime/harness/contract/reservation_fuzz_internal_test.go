package contract

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func FuzzModelDispatchReservationRefV2NeverPanics(f *testing.F) {
	f.Add("reservation", "attempt", int64(1), int64(2))
	f.Add("", "", int64(-1), int64(0))
	f.Fuzz(func(t *testing.T, id, attempt string, reserved, expires int64) {
		value := ModelDispatchReservationRefV2{ID: id, Digest: core.DigestBytes([]byte(id)), AttemptID: attempt, IntentDigest: core.DigestBytes([]byte(attempt)), CandidateDigest: core.DigestBytes([]byte(id + attempt)), ReservedUnixNano: reserved, ExpiresUnixNano: expires}
		_ = value.Validate()
	})
}
