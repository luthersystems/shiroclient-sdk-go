package mock

import (
	"context"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

// newSampledCtx returns a context carrying a sampled W3C span context with
// fixed IDs so traceparent values are deterministic.
func newSampledCtx(t *testing.T) (context.Context, trace.SpanContext) {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("1112131415161718")
	require.NoError(t, err)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	return trace.ContextWithSpanContext(context.Background(), sc), sc
}

func TestFlatten_InjectsTraceContext(t *testing.T) {
	ctx, sc := newSampledCtx(t)
	c := &mockShiroClient{}

	cro, err := c.flatten(ctx)
	require.NoError(t, err)
	require.NotNil(t, cro)

	tp, ok := cro.Transient["traceparent"]
	require.True(t, ok, "expected traceparent key in transient")
	assert.Equal(t,
		"00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01",
		string(tp),
	)

	// Round-trip: the propagator should extract the same span context back.
	extracted := trace.SpanContextFromContext(
		tracePropagator.Extract(context.Background(), traceCarrier(cro.Transient)),
	)
	assert.Equal(t, sc.TraceID(), extracted.TraceID())
	assert.Equal(t, sc.SpanID(), extracted.SpanID())
	assert.True(t, extracted.IsSampled())
}

func TestFlatten_PreservesExistingTransient(t *testing.T) {
	ctx, _ := newSampledCtx(t)
	c := &mockShiroClient{}

	userTransient := types.Opt(func(r *types.RequestOptions) {
		r.Transient["mxf"] = []byte("user-payload")
	})

	cro, err := c.flatten(ctx, userTransient)
	require.NoError(t, err)

	assert.Equal(t, []byte("user-payload"), cro.Transient["mxf"],
		"caller-supplied transient must survive trace injection")
	_, hasTrace := cro.Transient["traceparent"]
	assert.True(t, hasTrace, "trace injection must still happen alongside user transient")
}

func TestFlatten_NoSpanLeavesTransientClean(t *testing.T) {
	c := &mockShiroClient{}

	cro, err := c.flatten(context.Background())
	require.NoError(t, err)

	_, hasTrace := cro.Transient["traceparent"]
	assert.False(t, hasTrace,
		"propagator should be a no-op without a span context on ctx")
	_, hasState := cro.Transient["tracestate"]
	assert.False(t, hasState)
}

func TestFlatten_PerCallConfigOverridesBase(t *testing.T) {
	// Regression guard: flatten applies baseConfig before per-call configs,
	// so per-call writers win on key collision. This is the merge ordering
	// callers rely on for things like header/transient overrides.
	ctx := context.Background()
	c := &mockShiroClient{
		baseConfig: []types.Config{
			types.Opt(func(r *types.RequestOptions) {
				r.Transient["k"] = []byte("base")
			}),
		},
	}

	override := types.Opt(func(r *types.RequestOptions) {
		r.Transient["k"] = []byte("call")
	})

	cro, err := c.flatten(ctx, override)
	require.NoError(t, err)
	assert.Equal(t, []byte("call"), cro.Transient["k"])
}
