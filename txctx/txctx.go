package txctx

import (
	"context"

	"github.com/luthersystems/substratecommon/substratewrapper"
)

type key struct{}
type value struct {
	txID string
}

// ContextWithID adds a unique value to a context for storing a dependent
// transaction ID.
func ContextWithID(ctx context.Context) context.Context {
	return substratewrapper.ContextWithTransactionID(ctx)
}

// GetID gets the transaction ID from a context value if present.
func GetID(ctx context.Context) string {
	return substratewrapper.GetContextTransactionID(ctx)
}
