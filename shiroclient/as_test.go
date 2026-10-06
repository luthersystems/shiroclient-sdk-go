package shiroclient_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// as returns v as T and fails the test when v has another type. It reads
// decoded JSON, where a bare type assertion would panic.
func as[T any](t testing.TB, v interface{}) T {
	t.Helper()
	out, ok := v.(T)
	require.Truef(t, ok, "got %T, want %T", v, out)
	return out
}
