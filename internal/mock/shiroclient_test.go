package mock

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

// traceparentRE matches the W3C TraceContext traceparent header format
// (version 00). Independent of OTel's encoder, so it catches a propagator
// swap that still happens to round-trip through Inject/Extract.
// https://www.w3.org/TR/trace-context/#traceparent-header-field-values
var traceparentRE = regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)

// ctxWithSpan returns a context carrying a span context with fixed
// TraceID/SpanID so traceparent strings are deterministic across runs.
func ctxWithSpan(t *testing.T, flags trace.TraceFlags) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("1112131415161718")
	require.NoError(t, err)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags,
		Remote:     true,
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

// TestFlatten_InjectsTraceparent_NoTransientConfigsNeeded covers issue #56
// AC (a) sampled span on ctx → traceparent populated, and AC (b) caller
// passes zero transient configs and trace still lands.
func TestFlatten_InjectsTraceparent_NoTransientConfigsNeeded(t *testing.T) {
	t.Parallel()
	c := &mockShiroClient{}

	cro, err := c.flatten(ctxWithSpan(t, trace.FlagsSampled))
	require.NoError(t, err)
	require.NotNil(t, cro)

	tp, ok := cro.Transient["traceparent"]
	require.True(t, ok, "expected traceparent key in transient")

	// Literal compare — anchored to the W3C wire format, independent of
	// any OTel-internal round-trip.
	assert.Equal(t,
		"00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01",
		string(tp),
	)
	// Structural backstop: if the literal is ever updated for a legitimate
	// reason, the new value must still conform to traceparent v00.
	assert.Regexp(t, traceparentRE, string(tp))
}

func TestFlatten_PreservesExistingTransient(t *testing.T) {
	t.Parallel()
	c := &mockShiroClient{}

	userTransient := types.Opt(func(r *types.RequestOptions) {
		r.Transient["mxf"] = []byte("user-payload")
	})

	cro, err := c.flatten(ctxWithSpan(t, trace.FlagsSampled), userTransient)
	require.NoError(t, err)

	assert.Equal(t, []byte("user-payload"), cro.Transient["mxf"],
		"caller-supplied transient must survive trace injection")
	_, hasTrace := cro.Transient["traceparent"]
	assert.True(t, hasTrace, "trace injection must still happen alongside user transient")
}

// TestFlatten_UnsampledSpan_NotForceSampled locks in that flatten propagates
// the caller's TraceFlags as-is. Substrate's per-request Debug→Info promotion
// gates on the sampled bit, so a future change that hardcoded FlagsSampled
// here would silently change semantics for unsampled callers.
func TestFlatten_UnsampledSpan_NotForceSampled(t *testing.T) {
	t.Parallel()
	c := &mockShiroClient{}

	cro, err := c.flatten(ctxWithSpan(t, 0))
	require.NoError(t, err)

	tp := string(cro.Transient["traceparent"])
	require.NotEmpty(t, tp, "unsampled spans still emit a traceparent")
	assert.True(t, strings.HasSuffix(tp, "-00"),
		"unsampled TraceFlags must round-trip as -00, got %q", tp)
}

// TestFlatten_CtxTraceparentBeatsUserTransient pins the precedence policy
// when a caller has both a sampled span on ctx and a stale "traceparent"
// in their transient data. The real trace wins because Inject runs after
// ApplyConfigs. If the order is ever flipped, this test fails loudly.
func TestFlatten_CtxTraceparentBeatsUserTransient(t *testing.T) {
	t.Parallel()
	c := &mockShiroClient{}

	stale := types.Opt(func(r *types.RequestOptions) {
		r.Transient["traceparent"] = []byte(
			"00-deadbeefdeadbeefdeadbeefdeadbeef-1111111111111111-00")
	})

	cro, err := c.flatten(ctxWithSpan(t, trace.FlagsSampled), stale)
	require.NoError(t, err)
	assert.Equal(t,
		"00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01",
		string(cro.Transient["traceparent"]),
		"ctx-derived traceparent must override user-supplied value")
}

func TestFlatten_NoSpanLeavesTransientClean(t *testing.T) {
	t.Parallel()
	c := &mockShiroClient{}

	cro, err := c.flatten(context.Background())
	require.NoError(t, err)

	_, hasTrace := cro.Transient["traceparent"]
	assert.False(t, hasTrace,
		"propagator should be a no-op without a span context on ctx")
	_, hasState := cro.Transient["tracestate"]
	assert.False(t, hasState)
}

// TestFlatten_PerCallConfigOverridesBase is defense-in-depth for the
// merge ordering enforced by types.ApplyConfigs (internal/types/types.go) —
// not specific to the trace fix. flatten applies baseConfig before per-call
// configs, so per-call writers win on key collision.
func TestFlatten_PerCallConfigOverridesBase(t *testing.T) {
	t.Parallel()
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

	cro, err := c.flatten(context.Background(), override)
	require.NoError(t, err)
	assert.Equal(t, []byte("call"), cro.Transient["k"])
}
