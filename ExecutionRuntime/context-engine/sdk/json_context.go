package sdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// writeJSONContextV1 emits the same compact field-order JSON used by
// encoding/json for the SDK's closed DTO graph, while retaining a cancellation
// checkpoint at least once per 64 KiB written and between every aggregate item.
func writeJSONContextV1(ctx context.Context, dst io.Writer, value any) error {
	if ctx == nil {
		return context.Canceled
	}
	w := &contextJSONWriterV1{ctx: ctx, dst: dst}
	if err := w.value(reflect.ValueOf(value)); err != nil {
		return err
	}
	return ctx.Err()
}

type contextJSONWriterV1 struct {
	ctx context.Context
	dst io.Writer
}

func (w *contextJSONWriterV1) write(value []byte) error {
	for offset := 0; offset < len(value); offset += wireChunkBytesV1 {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		end := offset + wireChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		if _, err := w.dst.Write(value[offset:end]); err != nil {
			return err
		}
	}
	return w.ctx.Err()
}

func (w *contextJSONWriterV1) literal(value string) error { return w.write([]byte(value)) }

func (w *contextJSONWriterV1) value(value reflect.Value) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if !value.IsValid() {
		return w.literal("null")
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return w.literal("null")
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Struct:
		return w.structValue(value)
	case reflect.Slice:
		if value.IsNil() {
			return w.literal("null")
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return w.quoted(base64.StdEncoding.EncodeToString(value.Bytes()))
		}
		fallthrough
	case reflect.Array:
		if err := w.literal("["); err != nil {
			return err
		}
		for index := 0; index < value.Len(); index++ {
			if index > 0 {
				if err := w.literal(","); err != nil {
					return err
				}
			}
			if err := w.value(value.Index(index)); err != nil {
				return err
			}
		}
		return w.literal("]")
	case reflect.Map:
		return w.mapValue(value)
	case reflect.String:
		return w.quoted(value.String())
	case reflect.Bool:
		return w.literal(strconv.FormatBool(value.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return w.literal(strconv.FormatInt(value.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return w.literal(strconv.FormatUint(value.Uint(), 10))
	case reflect.Float32:
		return w.literal(strconv.FormatFloat(value.Float(), 'g', -1, 32))
	case reflect.Float64:
		return w.literal(strconv.FormatFloat(value.Float(), 'g', -1, 64))
	default:
		return fmt.Errorf("unsupported JSON kind %s", value.Kind())
	}
}

func (w *contextJSONWriterV1) quoted(value string) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.write(payload)
}

func (w *contextJSONWriterV1) structValue(value reflect.Value) error {
	if err := w.literal("{"); err != nil {
		return err
	}
	written := false
	typeOf := value.Type()
	for index := 0; index < value.NumField(); index++ {
		fieldType := typeOf.Field(index)
		if fieldType.PkgPath != "" {
			continue
		}
		name, options, _ := strings.Cut(fieldType.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = fieldType.Name
		}
		field := value.Field(index)
		if strings.Contains(options, "omitempty") && field.IsZero() {
			continue
		}
		if written {
			if err := w.literal(","); err != nil {
				return err
			}
		}
		written = true
		if err := w.quoted(name); err != nil {
			return err
		}
		if err := w.literal(":"); err != nil {
			return err
		}
		if err := w.value(field); err != nil {
			return err
		}
	}
	return w.literal("}")
}

func (w *contextJSONWriterV1) mapValue(value reflect.Value) error {
	if value.IsNil() {
		return w.literal("null")
	}
	if value.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("unsupported JSON map key %s", value.Type().Key())
	}
	keys := value.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	if err := w.literal("{"); err != nil {
		return err
	}
	for index, key := range keys {
		if index > 0 {
			if err := w.literal(","); err != nil {
				return err
			}
		}
		if err := w.quoted(key.String()); err != nil {
			return err
		}
		if err := w.literal(":"); err != nil {
			return err
		}
		if err := w.value(value.MapIndex(key)); err != nil {
			return err
		}
	}
	return w.literal("}")
}
