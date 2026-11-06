package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
)

// TestHealthCheckTrace checks HealthCheck starts a span under the caller's
// and sends it to the gateway as W3C trace context (#88).
func TestHealthCheckTrace(t *testing.T) {
	traceparent := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceparent <- r.Header.Get("traceparent")
		_, _ = w.Write([]byte(`{"reports": []}`))
	}))
	t.Cleanup(srv.Close)

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	client, ok := NewRPC([]types.Config{types.Opt(func(r *types.RequestOptions) { r.Endpoint = srv.URL })}).(*rpcShiroClient)
	require.True(t, ok)
	client.tracer = tp.Tracer("test")

	ctx, parent := tp.Tracer("test").Start(context.Background(), "caller")
	_, err := client.HealthCheck(ctx, nil)
	parent.End()
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 2)
	span := spans[0]
	assert.Equal(t, "sdk:HealthCheck", span.Name)
	assert.Equal(t, parent.SpanContext().SpanID(), span.Parent.SpanID())

	want := "00-" + span.SpanContext.TraceID().String() + "-" + span.SpanContext.SpanID().String() + "-01"
	assert.Equal(t, want, <-traceparent)
}

// TestHealthCheckWrite checks that WithHealthCheckWrite asks the gateway for
// its heartbeat write and sends the auth token, and that a plain check sends
// neither (luthersystems/substrate#1037).
func TestHealthCheckWrite(t *testing.T) {
	type got struct{ write, auth string }
	reqs := make(chan got, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs <- got{write: r.URL.Query().Get("write"), auth: r.Header.Get("Authorization")}
		_, _ = w.Write([]byte(`{"reports": []}`))
	}))
	t.Cleanup(srv.Close)
	client := NewRPC([]types.Config{types.Opt(func(r *types.RequestOptions) { r.Endpoint = srv.URL })})
	hc, ok := client.(smartHealthCheck)
	require.True(t, ok)

	_, err := hc.HealthCheck(t.Context(), []string{"phylum"})
	require.NoError(t, err)
	assert.Equal(t, got{}, <-reqs, "a plain check asks for no write")

	_, err = hc.HealthCheck(t.Context(), []string{"phylum"},
		types.Opt(func(r *types.RequestOptions) { r.HealthCheckWrite = true }),
		types.Opt(func(r *types.RequestOptions) { r.AuthToken = "tok" }))
	require.NoError(t, err)
	assert.Equal(t, got{write: "true", auth: "Bearer tok"}, <-reqs)
}
