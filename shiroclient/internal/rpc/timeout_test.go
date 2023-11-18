package rpc

import (
	"fmt"
	"testing"
)

func TestIsTimeoutError(t *testing.T) {
	err := &scError{
		message: "timeout",
		code:    1,
	}

	wrappedErr := fmt.Errorf("wrap call error: %w", err)

	if !IsTimeoutError(wrappedErr) {
		t.Errorf("IsTimeoutError failed to identify a wrapped timeout error")
	}
}
