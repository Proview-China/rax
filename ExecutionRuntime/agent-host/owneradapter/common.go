package owneradapter

import (
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

func unavailableV1(value any, reason string) error {
	if value == nil {
		return contract.NewError(contract.ErrorUnavailable, reason, "required owner reader is unavailable")
	}
	v := reflect.ValueOf(value)
	if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface || v.Kind() == reflect.Func || v.Kind() == reflect.Map || v.Kind() == reflect.Slice) && v.IsNil() {
		return contract.NewError(contract.ErrorUnavailable, reason, "required owner reader is unavailable")
	}
	return nil
}

func nowUnixNanoV1(clock func() time.Time) (int64, error) {
	if err := unavailableV1(clock, "clock_unavailable"); err != nil {
		return 0, err
	}
	now := clock().UnixNano()
	if now <= 0 {
		return 0, contract.NewError(contract.ErrorUnavailable, "clock_invalid", "clock returned a non-positive instant")
	}
	return now, nil
}
