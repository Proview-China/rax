package nilcheck

import "reflect"

// IsNil reports both a nil interface and an interface containing a typed-nil
// dependency. Constructors use it to reject capabilities that would panic on
// their first method call.
func IsNil(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}
