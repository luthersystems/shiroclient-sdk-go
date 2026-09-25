package types

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequestTransientRefusesEveryOtherField sets each RequestOptions field
// but Transient to a non-zero value and checks RequestTransient refuses it,
// so a field added later is refused per request until it is classified.
func TestRequestTransientRefusesEveryOtherField(t *testing.T) {
	typ := reflect.TypeOf(RequestOptions{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Name == "Transient" {
			continue
		}
		t.Run(f.Name, func(t *testing.T) {
			cfg := Opt(func(r *RequestOptions) {
				v := reflect.ValueOf(r).Elem().Field(i)
				switch v.Kind() {
				case reflect.Map:
					m := reflect.MakeMap(v.Type())
					m.SetMapIndex(reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem())
					v.Set(m)
				case reflect.Func:
					v.Set(reflect.MakeFunc(v.Type(), func([]reflect.Value) []reflect.Value {
						out := make([]reflect.Value, v.Type().NumOut())
						for j := range out {
							out[j] = reflect.New(v.Type().Out(j)).Elem()
						}
						return out
					}))
				case reflect.Ptr:
					v.Set(reflect.New(v.Type().Elem()))
				case reflect.Slice:
					v.Set(reflect.MakeSlice(v.Type(), 1, 1))
				case reflect.Interface:
					v.Set(reflect.ValueOf(1))
				case reflect.String:
					v.SetString("x")
				case reflect.Int:
					v.SetInt(1)
				case reflect.Bool:
					v.SetBool(true)
				default:
					t.Fatalf("field %s: kind %s not covered by this test", f.Name, v.Kind())
				}
			})
			_, _, err := RequestTransient([]Config{cfg})
			require.Error(t, err)
			if name, ok := requestOptionNames[f.Name]; ok {
				assert.Contains(t, err.Error(), name)
			}
		})
	}
}

func TestRequestTransientNamesEveryField(t *testing.T) {
	typ := reflect.TypeOf(RequestOptions{})
	for i := 0; i < typ.NumField(); i++ {
		if name := typ.Field(i).Name; name != "Transient" {
			assert.Contains(t, requestOptionNames, name, "name the config that sets %s", name)
		}
	}
}
